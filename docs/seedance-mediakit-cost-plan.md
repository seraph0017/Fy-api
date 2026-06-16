# Seedance 2.0 + MediaKit 成本与毛利方案

生成日期：2026-06-15
工作区：`/Users/jimmy/go/src/tracenex/fy-api-pr-seedance-mediakit-cost-plan`
目标：完成 Seedance 2.0 + MediaKit 实时预估供应商成本方案，并落地首版实现。

实现状态：已完成首版代码接入。首版覆盖任务私有快照与账务日志 `other` 字段；`/metrics`、自动灰度控制器、最终财务对账报表不在本次实现范围。

## 1. 目标

这次要解决的问题不是“怎么给用户收费”，而是“TraceNex 自己到底花了多少钱、赚了多少钱”。

核心目标：

- 对 Seedance 2.0 生成成本做可复核计算。
- 对 MediaKit 后处理成本做可复核计算，包括当前标准计费，以及闲时 / 低优先级任务的判定与兼容策略。
- 在任务、日志、报表里能同时看到用户收入、预估供应商成本、预估毛利和预估毛利率。
- `/metrics` 只放聚合指标，不放价格表、不放逐任务明细。

现有代码基础：

- `model/task_seedance_enhance.go` 已有 `GenerationCostQuota`、`EnhanceCostQuota`、`PipelineProviderCost`、`UserBilledQuota` 等字段雏形。
- `service/seedance_enhance_pipeline.go` 已经在 MediaKit 完成时写入 `ActualDurationSeconds`、`ActualFPS`、`EnhanceCostQuota`。
- `service/video_pipeline_strategy.go` 已改为按官网价格表和任务参数写入 `GenerationCostQuota`，不再从用户收费 `PriceData` 反推供应商成本。
- `middleware/prometheus_overlay.go` 当前是请求性能指标，不应该直接承担成本核算。

### 1.1 成本口径：实时预估，不作为最终财务毛利

本方案里的 Seedance 2.0 / MediaKit 价格都定义为“实时预估成本”：

- 用途是上线期、灰度期和运营期快速判断当前任务大概花了多少钱。
- 可以用于实时看趋势、预警、灰度放量 / 收缩决策。
- 不作为最终财务毛利、月度结算毛利、对账毛利的唯一依据。
- 最终财务毛利仍以供应商账单、TraceNex 实际收入、退款和人工/自动对账后的报表为准。

原因：

- 官方价格页与最终账单 SKU 可能存在折扣、优惠、四舍五入、赠送额度、账期修正等差异。
- Seedance 即使拿到了 provider `usage.completion_tokens`，它也只能说明“按官方公式和返回 usage 估算出的成本”，不是已经对账的发票成本。
- MediaKit 的 `resolution/fps/tool_version` 能推导出成本，但最终账单仍可能受实际 SKU 名称、活动价或账号折扣影响。
- Prometheus / perf metrics 是聚合时间序列，历史点不适合修账；修账应回到任务日志和账单报表。

因此字段命名和展示建议明确带上 `estimated` / `provider_cost_estimate` 语义。对于已经存在的字段名（例如 `PipelineProviderCost`）可以兼容保留，但日志和报表展示层应标注为“预估供应商成本”。

预估等级建议：

| 等级 | 来源 | 用途 |
| --- | --- | --- |
| `provider_usage_estimated` | provider 返回 usage / 时长 / fps 后，按官方价格计算 | 实时灰度和运营判断的主口径 |
| `param_estimated` | provider 未返回 usage，只能按请求参数和官方公式估算 | 只能看趋势，不能做强结论 |
| `reconciled` | 已和供应商账单或内部财务报表对齐 | 最终财务毛利口径，首版不在本方案内实现 |

## 2. 官方价格口径

### 2.1 Seedance 2.0

来源：

- 火山方舟模型价格页：`https://www.volcengine.com/docs/82379/1544106?lang=zh`
- 官方 PDF：`火山方舟_模型价格_1781059242.pdf`

已确认规则：

- Seedance 2.0 不是按秒直接计费，而是按视频生成 token 计费。
- 官方公式：`token 用量 = (输入视频时长 + 输出视频时长) * 输出视频宽 * 输出视频高 * 输出视频帧率 / 1024`
- 官方说明：准确 token 用量以 API 返回的 `usage.completion_tokens` 为准。
- 输入包含视频时有最低 token 用量限制。最低 token 与分辨率、宽高比、输出时长有关；如果估算 token 小于最低 token，按最低 token 计费。

官方单价：

| 模型 | 输出分辨率 | 输入不含视频 | 输入包含视频 | 单位 |
| --- | --- | ---: | ---: | --- |
| `doubao-seedance-2.0` | `480p` / `720p` | 46.00 | 28.00 | 元 / 百万 token |
| `doubao-seedance-2.0` | `1080p` | 51.00 | 31.00 | 元 / 百万 token |
| `doubao-seedance-2.0-fast` | 非 1080p | 37.00 | 22.00 | 元 / 百万 token |
| `doubao-seedance-2.0-mini` | 非 1080p | 23.00 | 14.00 | 元 / 百万 token |

注意：

- `doubao-seedance-2.0-fast` / `doubao-seedance-2.0-mini` 不支持 1080p 输出。
- 我们代码里的模型名可能是 `doubao-seedance-2-0-260128` 这种路由名，成本计算需要先归一化到官方价格模型名。
- 如果 provider 返回 usage，就不自己估算 token。
- 如果 provider 没返回 usage，再走官方估算公式，并记录 `usage_source=estimated`。
- 首版已实现上述单价、模型名归一化、provider usage 优先与参数 fallback。输入包含视频的最低 token 表暂不编码，后续如果拿不到 provider usage 且需要更精确 fallback，再补官方 minimum token 表。

### 2.2 MediaKit

来源：

- AI MediaKit 视频工具计费页：`https://www.volcengine.com/api/doc/getDocDetail?DocumentID=2486473&type=page`
- 画质增强提交接口：`https://www.volcengine.com/api/doc/getDocDetail?DocumentID=2279230&type=page`
- 画质增强开发指南：`https://www.volcengine.com/api/doc/getDocDetail?DocumentID=2279961&type=page`

已确认规则：

- 当前链路是 `POST /api/v1/tools/enhance-video`，对应 `tool_version=standard/professional` 的画质增强工具。
- 计费公式是 `输出时长(分钟) × 计费换算系数 × 0.75 元/分钟`。
- 官方明确说明：`scene=aigc` 只在 `tool_version=standard` 时生效。
- 官方明确说明：`resolution` 与 `fps` 会参与计费换算系数，`fps` 未指定时默认保持原始帧率。
- 当前文档没有确认这个接口存在可直接依赖的低优先级 / 闲时字段，首版不要把 off-peak 当成默认能力。

当前 Seedance 增强流水线使用：

- `service/seedance_enhance_pipeline.go`
- `MediaKitSubmitRequest{Scene: "aigc", ToolVersion: "standard", Resolution: "1080p"}`
- 路径：`/api/v1/tools/enhance-video`

这条链路的成本模型应以当前 `enhance-video` 计费页为准，而不是旧的“智能超分闲时”口径。

当前方案建议：

- 以 `tool_version + resolution + fps` 作为 MediaKit 成本主键。
- `tool_version=standard` 和 `tool_version=professional` 分别走官方标准版 / 专业版计费表。
- 提交阶段如果不显式传 `fps`，完成态必须以 provider 返回的 `result.fps` 为准；如果返回也缺失，只能标记 `cost_confidence=estimated`，不能当作最终财务账本。
- `task_type` / 低优先级字段只作为未来兼容项，不进入首版结算逻辑。
- 首版已实现 standard/professional 的官方系数表，并明确不把“画质增强（大模型）”的 2.5 元/分钟基价套到当前 standard/professional 链路。

### 2.3 MediaKit 闲时任务判定

历史 AI MediaKit 计费文档确认：

- `开启闲时任务` 默认关闭。
- 开启后使用服务的闲时资源处理，不保证任务实时性。
- 任务列表里 `任务类型` 支持 `正常任务`、`闲时任务`。
- `闲时任务` 定义为创建任务时开启闲时任务。

但对当前 `enhance-video` 流水线，本方案不把这条历史口径当作主路径：

- 不能用本地时间判断是否闲时。
- 不能用“晚上/白天”判断是否闲时。
- 不能默认假设当前接口支持低优先级任务。
- 如果后续官方证实当前接口支持显式低优先级字段，再按提交参数或任务返回字段判定，绝不按时间推断。

实现优先级（仅用于兼容未来字段）：

1. Provider 查询响应里的 `task_type` / 等价字段。
2. 提交请求里显式设置的低优先级字段。
3. 如果都没有，默认 `normal`。

## 3. 成本公式

### 3.1 单位换算

当前系统里：

- `common.QuotaPerUnit = 500000`
- 历史含义约等于 `$1 = 500000 quota`

供应商价格是人民币，所以需要明确一个人民币到 quota 的换算函数。

建议新增：

```text
rmb_to_quota(rmb) = round(rmb / RMBPerUSD * common.QuotaPerUnit)
```

实现口径：

- CN/SG 两个平台可能有不同额度展示方式，内部成本报表主单位统一用 `cost_quota`。
- `cost_rmb` 保留为供应商人民币价格来源和账单抽样对账辅助，不作为跨平台主聚合单位。
- 每条成本明细都记录 `rmb_per_usd` 和 `quota_per_unit`，确保离线同步数据库后仍能复算当时的 quota 成本。

`RMBPerUSD` 先使用现有 `setting/ratio_setting/model_ratio.go` 里的 `USD2RMB = 7.3`，但要把版本和汇率写入成本快照：

- `cost_price_version`
- `rmb_per_usd`

不要把用户分组倍率 `group_ratio` 用到供应商成本里。供应商成本是 TraceNex 成本，不是用户售价。

### 3.2 Seedance 生成成本

优先公式：

```text
billable_tokens = provider_usage.completion_tokens
generation_cost_rmb = unit_price_rmb_per_million_tokens * billable_tokens / 1_000_000
generation_cost_quota = rmb_to_quota(generation_cost_rmb)
```

实际写入明细中的公式文本：

```text
cost_rmb = unit_price_rmb_per_million_tokens * billable_tokens / 1000000;
cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)
```

fallback 公式：

```text
estimated_tokens =
  (input_video_seconds + output_video_seconds)
  * output_width
  * output_height
  * output_fps
  / 1024

billable_tokens = max(estimated_tokens, minimum_tokens_if_video_input)
```

fallback 必须记录：

- `seedance_usage_source=estimated`
- `seedance_estimated_tokens`
- `seedance_minimum_tokens_applied=true/false`

### 3.3 MediaKit 增强成本

```text
enhance_minutes = actual_duration_seconds / 60
enhance_cost_rmb = enhance_base_price_rmb_per_minute
                  * billing_coefficient(tool_version, resolution, fps)
                  * enhance_minutes
enhance_cost_quota = rmb_to_quota(enhance_cost_rmb)
```

实际写入明细中的公式文本：

```text
cost_rmb = base_price_rmb_per_minute * billing_coefficient * duration_minutes;
cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)
```

对于当前 `enhance-video` 流水线：

```text
成本来自官方视频工具计费页，不再使用旧的“智能超分闲时”单价。
```

### 3.4 总成本与毛利

```text
estimated_provider_cost_quota = estimated_generation_cost_quota + estimated_enhance_cost_quota
user_revenue_quota = task.Quota 或日志净额
estimated_gross_profit_quota = user_revenue_quota - estimated_provider_cost_quota
estimated_gross_margin = estimated_gross_profit_quota / user_revenue_quota
```

报表层必须对 consume/refund 做净额处理，不能只 sum consume。

这些字段的默认展示名建议使用：

- `预估供应商成本`
- `预估毛利`
- `预估毛利率`

不要在管理后台或报表里直接叫“最终成本 / 最终毛利”，避免和财务对账口径混淆。

### 3.5 实时灰度判断公式

Seedance 2.0 + MediaKit 灰度期建议看的核心指标：

```text
pipeline_request_count = 命中 seedance2_720p_mediakit_1080p 的任务数
pipeline_success_rate = 成功任务数 / 已结束任务数
pipeline_fallback_rate = enhance 失败后返回 generation 结果的任务数 / 已结束任务数
estimated_avg_provider_cost_quota = sum(estimated_provider_cost_quota) / 成功任务数
estimated_avg_revenue_quota = sum(user_revenue_quota) / 成功任务数
estimated_gross_margin = (sum(user_revenue_quota) - sum(estimated_provider_cost_quota)) / sum(user_revenue_quota)
estimated_cost_per_second = sum(estimated_provider_cost_quota) / sum(actual_duration_seconds)
enhance_attach_rate = 有 MediaKit enhance 的成功任务数 / pipeline 成功任务数
```

灰度调整建议：

- 放量条件：成功率稳定、fallback 率可接受、预估毛利率高于内部阈值、P95 完成时长可接受。
- 收缩条件：预估毛利率低于阈值、MediaKit 增强失败率升高、平均预估成本明显偏离预期、供应商错误率升高。
- 暂停条件：MediaKit 或 Seedance 出现大面积失败、预估毛利转负且持续多个 bucket、供应商账单抽样与预估偏差过大。

注意：这些指标只能辅助调整 `traffic_percent`，不能自动证明最终财务盈利。

## 4. 数据模型设计

扩展 `model.SeedanceEnhancePipeline`：

```go
CostPriceVersion string
RMBPerUSD float64

GenerationUsageSource string
GenerationBillableTokens int64
GenerationEstimatedTokens int64
GenerationMinimumTokensApplied bool
GenerationCostRMB float64
GenerationCostQuota int
EnhanceBillingVersion string
EnhanceToolVersion string
EnhanceScene string
EnhanceOutputResolution string
EnhanceOutputFPS float64
EnhanceBasePriceRMBPerMinute float64
EnhanceBillingCoefficient float64
EnhanceProviderTaskType string
EnhanceTaskClass string
EnhanceTaskClassSource string
EnhanceCostRMB float64
EnhanceCostQuota int

PipelineProviderCost int
UserBilledQuota int
GrossProfitQuota int
GrossMargin float64
```

保留已有字段：

- `ActualDurationSeconds`
- `ActualFPS`
- `GenerationCostQuota`
- `EnhanceCostQuota`
- `PipelineProviderCost`
- `UserBilledQuota`

命名建议：

- Go 结构体可以兼容已有字段名，避免迁移风险。
- JSON 里用 `provider_cost_quota` / `pipeline_provider_cost_quota` 保持清晰。

## 5. 代码改造点

### 5.1 新增成本计算模块

新增文件：

- `service/media_provider_cost.go`
- `service/media_provider_cost_test.go`

职责：

- 存价格表。
- 做模型名归一化。
- 做 RMB -> quota。
- 计算 Seedance 成本。
- 计算 MediaKit `enhance-video` 成本。

建议接口：

```go
type ProviderCostInput struct {
    Component string
    Provider string
    Model string
    ToolVersion string
    Resolution string
    FPS float64
    InputClass string
    TaskClass string
    DurationSeconds float64
    BillableTokens int64
}

type ProviderCostResult struct {
    Version string
    CostRMB float64
    CostQuota int
    UnitPriceRMB float64
    Unit string
    RMBPerUSD float64
}
```

实际首版使用 `MediaProviderCostInput` / `MediaProviderCostResult`，额外记录 `UsageSource`、`EstimatedTokens`、`BillableTokens`、`BillingCoefficient` 和 `EnhanceBasePriceRMBPerMin`。

### 5.2 修改 Seedance 提交快照

文件：

- `service/video_pipeline_strategy.go`

当前问题：

- `estimateVideoPipelineGenerationCostQuota` 从 `info.PriceData` 反推生成成本。
- 这其实是用户收费，不是供应商成本。

改法：

- 提交阶段只记录“预计参数”，不要把它当最终供应商成本。
- 若没有 provider usage，允许记录一个估算成本，但字段必须写 `usage_source=param_estimated`。
- 完成阶段拿到 provider usage 后重算并覆盖。

实现状态：已完成。提交快照总是写入一份估算；有 gin request 时按真实请求参数估算，没有 request 时按 pipeline 默认 5 秒、24fps、720p generation 估算，避免成本为 0。

### 5.3 修改 Seedance 完成态

文件：

- `service/task_polling.go`
- `service/seedance_enhance_pipeline.go`
- 对应 Doubao/Seedance adaptor 的 `ParseTaskResult`

目标：

- 从上游完成结果里提取 usage / duration / fps / resolution。
- 写入 `GenerationBillableTokens`。
- 调用成本 helper 计算 `GenerationCostRMB` / `GenerationCostQuota`。

如果当前 `TaskInfo` 没有 usage 字段，需要扩展 `relay/common.TaskInfo`，或者在 pipeline 里解析原始 response body。

实现状态：已完成。`relay/common.TaskInfo` 增加 `DurationSeconds`、`FPS`、`Resolution`；Doubao task adaptor 在成功态把上游 `duration`、`framespersecond`、`resolution` 和 usage 带入 `TaskInfo`。

### 5.4 修改 MediaKit client

文件：

- `service/volcengine_mediakit_client.go`

新增：

- `fps` 请求字段，显式记录目标输出帧率。
- `MediaKitTaskResponse` 里的 `tool_version` / `resolution` / `fps` 输出字段。
- 如果后续官方确认当前接口支持低优先级字段，再补对应请求字段；首版不做这个假设。

`MediaKitTaskResponse` 增加兼容字段：

```go
TaskType string `json:"task_type"`
```

实现状态：已完成。`MediaKitTaskResponse` 已含 `TaskType`、`duration`、`fps`、`resolution`、`tool_version`；`MediaKitSubmitRequest` 增加 `fps` 预留字段，但当前 pipeline 不主动设置 fps。

### 5.5 修改 MediaKit 完成态成本

文件：

- `service/seedance_enhance_pipeline.go`

当前问题：

```go
unitPerSecond := 0.025
if fps > 30 {
    unitPerSecond = 0.05
}
```

这个写死逻辑不符合当前 `enhance-video` 官方计费页，也没有把 `tool_version/resolution/fps` 纳入核算。

改法：

- 删除硬编码 `0.025` / `0.05`。
- 读取任务完成结果里的 `tool_version` / `resolution` / `fps`。
- 根据官方计费页计算 `billing_coefficient`。
- `actual_duration_seconds / 60` 计费。
- 写入 `EnhanceBasePriceRMBPerMinute`、`EnhanceBillingCoefficient`、`EnhanceCostRMB`、`EnhanceCostQuota`。

实现状态：已完成。旧的 `0.025` / `0.05` per-second 逻辑已删除，改为 `duration/60 × coefficient × 0.75`。

### 5.6 增强日志

文件：

- `service/task_billing.go`
- 可能还需要 task 完成态日志记录点

日志 `other` 增加：

- `provider_cost_quota`
- `provider_cost_rmb`
- `generation_cost_quota`
- `generation_cost_rmb`
- `enhance_cost_quota`
- `enhance_cost_rmb`
- `enhance_base_price_rmb_per_minute`
- `enhance_billing_coefficient`
- `enhance_tool_version`
- `enhance_output_resolution`
- `enhance_output_fps`
- `enhance_provider_task_type`
- `user_billed_quota`
- `gross_profit_quota`
- `gross_margin`
- `cost_price_version`
- `rmb_per_usd`
- `seedance_usage_source`
- `seedance_billable_tokens`
- `enhance_billing_version`
- `enhance_task_class`
- `enhance_task_class_source`
- `enhance_low_priority`
- `provider_cost_estimate_details`：内部成本明细数组，包含公式 key/version/text、variables、coefficients、usage_source、cost_quota、cost_rmb、confidence。

实现状态：已完成当前轻量版。不新增表结构；任务私有快照 `tasks.private_data.seedance_enhance` 保存 `generation_cost_detail` / `enhance_cost_detail`，task billing `logs.other` 保存 `provider_cost_estimate_details` 和聚合 `*_estimate_*` 字段。用户日志列表和用户导出会脱敏全部成本、公式、变量、系数、毛利字段；管理员日志和 admin export 保留完整数据。若没有差额/退款日志，不额外生成一条最终账务日志，避免改变现有扣费行为。

### 5.7 报表

可以先新增脚本：

- `scripts/ops/media_cost_report.py`

输出列：

- 日期
- 模型
- 渠道 ID
- pipeline
- 任务数
- 用户收入 quota
- 供应商成本 quota
- 毛利 quota
- 毛利率
- 闲时任务数
- 正常任务数

实现状态：未实现。报表留到下一阶段；本次先保证任务私有数据和日志字段可被后续报表聚合。

后续再接入 `gross-profit-report` skill 或管理后台页面。

### 5.8 Prometheus metrics

不在第一阶段做为强依赖，但方案上是合理的：metrics 适合做实时聚合观测，方便后续调整灰度百分比。

现有代码里已经有两套相关能力：

- `pkg/perf_metrics`：按 `model + group + bucket` 聚合请求数、成功率、延迟、TPS，用于管理后台性能趋势。
- `middleware/prometheus_overlay.go`：导出 `/metrics` Prometheus 指标，当前主要覆盖 relay 请求、错误、耗时、重试等。
- `service/video_pipeline_config.go`：视频流水线灰度配置支持 `rollout.traffic_percent`，并通过文件 watcher 热加载。

因此建议的控制链路是：

```text
任务完成态写成本快照
  -> consume/task log 写预估成本字段
  -> 低基数 metrics 聚合成本、收入、成功率、fallback
  -> Grafana/后台看到趋势
  -> 人工或独立控制面调整 config/video-pipeline.yaml 的 traffic_percent
  -> watcher 热加载生效
```

不建议的链路是：

```text
metrics 直接存价格表
metrics 直接存逐 task/user 明细
metrics 直接自动修改 traffic_percent
```

原因：

- metrics 是观测系统，不是价格配置系统。
- Prometheus 时间序列历史不可修正，不适合作为最终账本。
- 自动改灰度需要冷却时间、上下限、审批/回滚、异常保护，应该是独立控制面能力，不应该藏在指标采集代码里。

如果后续要做，只做聚合 counter：

- `fy_billing_provider_cost_estimate_quota_total{region,model,provider,component,pipeline}`
- `fy_billing_provider_cost_estimate_rmb_total{region,model,provider,component,pipeline}`
- `fy_billing_user_revenue_quota_total{region,model}`
- `fy_billing_gross_profit_estimate_quota_total{region,model,pipeline}`
- `fy_billing_task_count_total{region,model,pipeline,component,task_class,status}`
- `fy_video_pipeline_fallback_total{region,model,pipeline,reason}`
- `fy_video_pipeline_duration_seconds{region,model,pipeline,stage}` histogram

禁止：

- 不输出价格表 gauge。
- 不输出 `task_id` / `user_id` / `token_id`。
- 不输出 URL、prompt、reference 等高基数字段。

label 控制建议：

- `region`：`cn` / `sg` / `test` 这种小集合。
- `model`：用户请求模型或规范化模型名。
- `pipeline`：例如 `seedance2_720p_mediakit_1080p`。
- `component`：`generation` / `enhance`。
- `task_class`：`normal` / `low_priority` / `unknown`。
- `status`：`success` / `failed` / `fallback`。
- 不加 channel_id，除非明确用于供应商渠道对比；加了也要评估 cardinality。

实现状态：未实现。首版不新增 metrics，不把价格表或灰度配置放进 metrics。后续如接 Prometheus，只从任务完成态快照聚合低基数 `*_estimate_*` 指标。

### 5.9 metrics 是否适合支撑灰度百分比调整

结论：适合，但它应该是“观测和决策输入”，不是“价格源”和“最终账本”。

合理点：

- 灰度调整需要实时看趋势，metrics 的 bucket / counter / histogram 正适合。
- 成本、收入、毛利率都可以低基数聚合，不需要暴露用户或任务明细。
- 现有 `video_pipeline_config.go` 已经支持 `traffic_percent` 热加载，后续可以把 metrics 看板与这个配置形成闭环。

需要补的点：

- 当前 `perf_metrics` 只记录 relay 性能，不记录任务成本；需要新增 billing/pipeline metrics，而不是把价格硬塞进现有性能表。
- 任务是异步完成，成本 metrics 应在任务最终完成 / fallback / failed 时记录，不应该在提交时就记最终成本。
- 需要把 `estimated` 标记带进日志字段；metrics 名字也要带 `estimate`，避免被误认为财务对账成本。
- 若未来要自动调灰度，应新增单独的控制器读取聚合指标，再写 `config/video-pipeline.yaml` 或配置中心；控制器要有最小样本数、冷却时间、最大单次调整幅度、人工开关和回滚。

## 6. 实施步骤

### Sprint 1：价格契约固化

产出：

- `docs/reports/seedance-mediakit-pricing-2026-06-15.md`

内容：

- Seedance 官方单价。
- Seedance token 公式。
- Seedance minimum token 表或“待补全”的明确缺口。
- MediaKit `enhance-video` 标准版 / 专业版计费规则。
- MediaKit `tool_version + resolution + fps` 对应的计费换算系数。
- 当前 endpoint 是否存在可用的低优先级 / 闲时字段。

验收：

- 每个价格都能追溯到官方文档 URL。
- 每个价格都有单位。
- 每个价格都有版本日期。

### Sprint 2：纯成本计算

产出：

- `service/media_provider_cost.go`
- `service/media_provider_cost_test.go`

验收：

- Seedance 2.0：720p、1080p、含视频、不含视频都有单测。
- MediaKit：标准版 / 专业版、1080p、不同 fps 都有单测。
- RMB 到 quota 换算有单测。

### Sprint 3：流水线接入

产出：

- Seedance 完成态写入真实 usage/token 成本。
- MediaKit 完成态写入真实时长、版本、分辨率、帧率和成本。
- 删除旧硬编码 `0.025` / `0.05` 逻辑。

验收：

- `go test ./service/...`
- 一条 5 秒 720p 生成任务能看到 generation cost。
- 一条 720p -> 1080p 增强任务能看到 enhance cost。

### Sprint 4：日志与报表

产出：

- consume log `other` 带成本字段。
- `scripts/ops/media_cost_report.py`

验收：

- 报表能输出指定日期 Seedance 2.0 的收入、成本、毛利。
- refund 能抵扣收入。
- 成本不受用户 group ratio 影响。

### Sprint 5：可选 metrics

前置条件：

- 日志和报表已经验证正确。

产出：

- 低基数 Prometheus counter。

验收：

- `/metrics` 无 task/user/token 维度。
- 指标只用于趋势和报警，不作为财务账本。

## 7. 风险与处理

### 风险 1：Seedance 2.0 minimum token 表没完整编码

处理：

- 第一阶段优先使用 provider 返回的 usage。
- fallback 估算时标记 `estimated`。
- minimum token 表未确认前，不用 fallback 结果做强财务判断。

### 风险 2：MediaKit 计费页与实际账单 SKU 不一致

处理：

- 首版按 `enhance-video` 当前官方计费页实现。
- 上线前用一条真实 MediaKit 账单校验 `tool_version/resolution/fps` 是否落到同一计费单元。
- 如果账单 SKU 名称不同，只调整成本表映射，不改流水线结构。

### 风险 3：闲时 / 低优先级字段不确定

处理：

- 首版不主动提交低优先级字段。
- 如果后续官方确认当前接口支持，再补显式字段和对应成本单测。
- 仍然不能用本地时间推断是否闲时。

### 风险 4：用用户收入反推供应商成本

处理：

- 明确禁止。
- 成本只来自 provider price table + provider usage。

### 风险 5：Prometheus 历史不可修正

处理：

- Prometheus 放最后。
- 财务口径以日志和报表为准。

## 8. 回滚方案

- 成本字段都是 additive metadata，可以忽略或停止写入。
- 不改用户实际扣费逻辑，先只增加成本观测。
- 如果报表口径有误，修报表重新聚合日志。
- 如果 metrics 出问题，只关闭 billing counters，不影响业务请求。

## 9. 当前实现结论

首版实现已经完成，且保持以下边界：

- 不改变用户实际扣费。
- 不把用户售价、分组倍率、灰度比例混入供应商成本。
- 不按本地时间判断 MediaKit 闲时。
- 不把 metrics 当价格配置或灰度配置存储。
- 所有价格字段都是实时预估成本，最终财务毛利仍需供应商账单和收入报表对账。

本次未做的后续项：

1. Seedance 输入包含视频时的 minimum token 表 fallback。
2. 媒体成本日报 / 管理后台报表。
3. 低基数 Prometheus 成本指标。
4. 独立的灰度控制器或配置中心联动。
