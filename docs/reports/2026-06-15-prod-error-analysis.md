# 2026-06-15 CN/SG 生产错误分析报告

> 范围：CN 与 SG 生产环境，时间窗口为 2026-06-08 至 2026-06-15 22:xx CST。  
> 数据来源：两地 `/opt/fy-api/logs/oneapi-*` 应用日志，以及 Nginx error log。  
> 目标：先把所有反复出现的错误族统一编号、归类和分析，再决定是否需要修改代码。

## 一、结论摘要

- CN 应用错误：1,712 条 `[ERR]`，未发现 panic。
- SG 应用错误：5,311 条 `[ERR]`，未发现 panic。
- CN Nginx error：159 条，主体是 TLS 握手噪声。
- SG Nginx error：407 条，主体也是 TLS 握手噪声，夹杂少量 upstream timeout / premature close。
- 大多数错误属于配置、TraceNex 本地用户余额、渠道池、上游容量/账号限制、上游参数校验或客户端行为，不应该直接改核心 relay 逻辑。
- 明确的代码修复候选：阿里错误响应里的 `code` 可能是数字，但当前 DTO 只接受字符串。
- 需要优先深挖但不能盲改的高风险区域：SG 流式请求断连与 usage 缺失/计费边界，上游也有公开 issue 在讨论。

## 二、错误编号总表

> 读表口径：`错误来源/链路位置` 用来判断“是谁的问题”；`是否打到上游` 用来判断它是不是供应商侧错误；`修复/上线状态` 只在“已确认修复 commit 已包含在生产当前版本，且上线后未再出现”时标记为“已修复上线”。

| 编号 | 错误到底是什么 | 错误来源/链路位置 | 是否打到上游 | CN 数量 | SG 数量 | 当前版本上线后复现 | 修复/上线状态 | 处理结论 |
|---|---|---|---|---:|---:|---|---|---|
| E001 | 没有可用渠道可路由 | TraceNex 本地 distributor 选渠道阶段 | 通常否 | 860 | 698 | CN 119，SG 14 | 未修复，仍在出现 | 查模型/分组/渠道配置，不是用户余额，也不是供应商余额 |
| E002 | TraceNex 本地用户余额或额度不足 | TraceNex 本地账户/预扣费阶段 | 通常否 | 434 | 92 | CN 83，SG 1 | 非代码问题，仍在出现 | 用户充值/暂停任务；不是上游渠道余额不足 |
| E003 | Token 无效或 token 没有模型权限 | TraceNex 本地鉴权/权限阶段 | 否 | 140 | 188 | CN 15，SG 14 | 非代码问题，仍在出现 | 查客户端 token、用户分组、模型权限 |
| E004 | 阿里视频任务提交响应里没有任务 ID | 阿里视频任务 adaptor 解析上游响应阶段 | 是 | 198 | 0 | CN 0，SG 0 | 相关 wan2.6 r2v 修复已上线，两地当前版本后未复现；标记为已修复上线/未复现 | 保留观察；如复现再查原始上游响应 |
| E005 | 阿里错误响应里的 `code` 是数字，DTO 只接受字符串 | TraceNex 解析供应商错误响应阶段 | 是 | 8 行 | 0 | CN 0，SG 0 | CN 已修复上线；SG 未包含该修复但未观测到此错误 | CN 标记已修复上线；SG 后续发版时补入 |
| E006 | 请求参数、模型或 endpoint 不符合上游能力 | 上游校验或 provider adapter 兼容层 | 是 | 44 | 1,512 | CN 约 11，SG 185；`stop` 精确项两地 0 | 大类未修复；`stop` 修复分支未上线但当前版本后未复现 | 先按 channel/model 拆分，可能做定向字段过滤 |
| E007 | 供应商限流、高需求、供应商账号或价格层级限制 | 上游供应商容量/账号限制 | 是 | 0 已归类 | 约 568-577 | CN 0，SG 491 | 非代码问题，仍在出现 | 调渠道并发、cooldown、供应商额度；不是 TraceNex 用户余额 |
| E008 | 流式请求中客户端断开或 context 被取消 | 下游客户端/流式链路/计费收尾阶段 | 可能已打到上游 | 3 | 873 | CN 0，SG 777 | 未修复，仍在出现；需账务审计 | 先做 usage/账务审计，不能盲改 |
| E009 | 上游网络失败、连接重置、代理超时或 524 | 请求上游后的网络/代理阶段 | 是 | 2 | 238 | CN 4，SG 120 | 未修复，仍在出现 | 按 channel/domain 查超时和健康，不是用户余额 |
| E010 | 供应商内容安全策略拒绝 | 上游内容安全审核阶段 | 是 | 0 已归类 | 167 | CN 0，SG 11 | 非代码问题，仍在出现 | 透传/分类展示，不应绕过 |
| E011 | TLS 握手失败噪声 | Nginx TLS 握手阶段，未进入应用 | 否 | 159 多数 | 407 多数 | 未按应用版本切分 | 非应用问题 | 公网扫描/异常客户端噪声，不计入应用故障 |
| E012 | Nginx 代理层 upstream 超时、提前关闭或 body 过大 | Nginx 反向代理层 | 可能 | 少量 | 少量 | 未精确切分 | 未修复，需代理层排查 | 查 Nginx timeout/body size 和部署时间 |
| E013 | 供应商返回泛化内部错误 | 上游供应商 5xx/内部错误 | 是 | 尾部样本有 | 尾部样本有 | CN 0，SG 7 | 非代码问题，仍在出现 | 可做供应商 5xx 分类/cooldown，不是本地 panic |
| E014 | 图片生成/编辑 API 能力不匹配 | 图片路由的请求参数/模型/供应商能力层 | 是或本地 adapter | 计入 E006 | 计入 E006 | `response_format` 精确项两地 0；其它图片能力错误 SG 仍有 | Azure `response_format` 修复已上线且未复现；大类未修复 | 按图片路由和 channel 拆分，可能做定向校验 |
| E015 | 请求体、tokens、tools、`max_tokens` 超限制 | 客户端输入超过网关/上游上限 | 可能 | 计入 E006 | 计入 E006 | SG 仍有 tools/max tokens 等 | 未修复，偏前置校验/文档问题 | 可做前置 400 校验和模型能力文档 |

## 三、当前生产版本与上线后复现口径

| 地区 | 当前运行版本 | 部署时间 | 生产源码 commit | 上线后日志切分起点 |
|---|---|---|---|---|
| CN | `v1.6.1-tracenex` | 2026-06-13 22:03 CST 左右 | `169c6189 Fix Ali numeric error code parsing` | `2026/06/13 - 22:03:22` |
| SG | `sg-3d179de6` | 2026-06-12 11:48 CST 左右 | `3d179de6 chore: rename brand to mobile site` | `2026/06/12 - 11:48:22` |

已核对的修复包含关系：

- `169c6189 Fix Ali numeric error code parsing`：CN 当前版本已包含；SG 当前版本未包含。
- `8e4f6fbd fix(ali): adapt wan2.6 r2v reference urls`、`33ec1d9f fix: migrate wan2.6-r2v to DashScope media array`：CN/SG 当前版本均包含。
- `49642f13 fix: drop Azure GPT image response_format`：CN/SG 当前版本均包含。
- `60a24cf5 fix(relay): strip stop param for gpt-5 models`：CN/SG 当前版本均未包含；虽上线后 `stop` 精确错误未复现，但不能标记为“已修复上线”。

上线后复现统计：

| 地区 | 当前版本上线后 `[ERR]` 总数 | 仍在出现的主要错误 |
|---|---:|---|
| CN | 229 | E001 119、E002 83、E003 15、seedream endpoint/model 错误约 11 |
| SG | 1,628 | E008 777、E007 491、E006 185、E009 120、E003 14、E001 14、E010 11、E013 7 |

## E001 渠道不可用 / 无可路由渠道

### 判定口径

E001 指 TraceNex distributor 已经进入选渠道流程，但根据当前 `model + group + token 权限 + 渠道状态` 没有选出任何可用渠道。它不是用户余额不足，也不是供应商返回错误；请求通常没有真正打到上游。

### 现象与证据

CN 数量：860。

CN Top 模型/分组：

- `DeepSeek-V3.2 | Deepseek3.2`：593
- `claude-opus-4-6 | default`：134
- `claude-haiku-4-5-20251001 | default`：89
- `dall-e | default`：36
- `claude-sonnet-4-6 | default`：6
- `wan2.6-r2v-flash | default`：1

SG 数量：698。

SG Top 模型/分组：

- `gpt-5.4-mini | default`：330
- `codex-auto-review | default`：287
- `deepseek-r1-0528 | DeepSeek`：24
- `text-embedding-3-small | default`：18
- `gtp5.5 | gpt_陕西景绘`：12
- `deepseek-v3-0324 | DeepSeek`：12

代表日志：

```text
No available channel for model DeepSeek-V3.2 under group Deepseek3.2 (distributor)
```

### 具体情况

这是 distributor 在选渠道时明确表示：当前请求的 `model + group` 没有可用渠道。常见原因包括：

- 该分组下没有配置这个模型。
- 渠道存在但被禁用、不可用、被健康检查下线、额度耗尽或被优先级/标签过滤掉。
- 用户请求模型名和渠道模型名/映射名不一致。
- 用户 token 所属分组和预期分组不一致。

### 上游怎么看

没有找到上游针对这类生产模式的直接 issue。这通常是网关的正常行为：没有可用渠道就拒绝请求。上游一般把它视为运维配置问题，不是 new-api core bug。

### 影响

请求会立即失败，一般不会扣费。频次较高，说明对应模型/分组对用户是可见或被自动任务反复调用的，会影响可用性。

### 是否需要修改

不建议先改核心代码。

建议动作：

1. 按 `model + group + region` 导出渠道可用性。
2. 确认 `DeepSeek-V3.2`、`gpt-5.4-mini`、`codex-auto-review` 是否本来就要对外开放。
3. 增加“用户请求了但没有渠道可路由”的运营报表。
4. 如果前台价格表/模型列表展示了这些模型，后台却没有渠道，建议在管理端加提示。

## E002 TraceNex 本地用户余额/额度不足

### 判定口径

只把 **TraceNex 本地账户体系** 判断出来的余额不足、预扣费不足、用户额度不足放在 E002。判断依据是错误文案来自网关自身的用户余额/预扣费逻辑，例如：

- `预扣费额度失败, 用户剩余额度: ...`
- `用户额度不足, 剩余额度: ...`

不放在 E002 的情况：

- `Billing hard limit has been reached.`：这是上游供应商账号或供应商侧 billing limit，归入 E007。
- Azure `exceeded the call rate limit` / `pricing tier` / `Retry-After`：这是上游限流或服务层级限制，归入 E007。
- `insufficient quota` 如果来自供应商原文，需要看错误上下文；本次日志里没有 CN 此类供应商 quota，SG 的供应商侧限制归 E007。

### 现象与证据

CN 数量：434，全部是 TraceNex 本地预扣费失败，不是上游渠道余额不足。

代表日志：

```text
relay error: 预扣费额度失败, 用户剩余额度: ¥N, 需要预扣费额度: ¥N
```

SG 数量：92，其中：

- `预扣费额度失败`：29 条，本地预扣费不足。
- `用户额度不足`：63 条，本地用户额度不足。

代表日志：

```text
relay error: 用户额度不足, 剩余额度: ＄-0.003238
```

### 具体情况

这是 TraceNex 自己的账户余额或用户额度校验失败。CN 的 434 条全部是本地预扣费失败，像是某个用户或定时任务在余额不足时持续请求。SG 的 92 条也是本地用户余额/额度问题。

这类错误不是“上游渠道余额不足”。上游供应商账号额度、Azure 价格层级限流、供应商 hard limit 已单独归到 E007。

### 上游怎么看

没有发现相关上游 bug。这属于产品/账户体系的正常拒绝。

### 影响

请求失败。预扣费失败通常发生在请求上游之前；本地用户额度不足也不代表渠道或供应商账号没钱。

### 是否需要修改

不需要改代码。

建议动作：

1. 找出重复失败的用户和 token，提醒充值或暂停自动任务。
2. 如果是自动任务持续触发，建议暂停任务或在客户端侧加余额检查。
3. 如有必要优化用户提示文案，但不影响正确性。

## E003 Token 无效或无模型权限

### 判定口径

E003 指请求在鉴权或模型权限检查阶段失败。它发生在选渠道或请求上游之前，责任通常是客户端 token、用户分组、模型权限配置，不代表渠道或供应商故障。

### 现象与证据

CN 数量：140。

代表日志：

```text
Invalid token
无效的令牌
```

SG 数量：188。

代表日志：

```text
Invalid token
This token has no access to model gpt-5.4-mini
该令牌无权访问模型 deepseek-v4-pro
```

### 具体情况

这类错误来自鉴权或模型权限校验：token 不存在、过期、填错，或者 token 有效但没有请求模型的权限。

### 上游怎么看

没有相关上游 issue。这是预期安全行为。

### 影响

请求失败，不影响系统稳定性。

### 是否需要修改

不需要改代码。

建议动作：

1. 如需要，审计高频失败 token。
2. 检查应该有权限的用户是否分组或模型权限配置错误。
3. 如果是扫描器或误配置客户端导致的噪声，可在告警侧降噪。

## E004 阿里视频任务提交未返回 `output.task_id`

### 判定口径

E004 指请求已经进入阿里视频任务提交链路，网关收到上游响应后没有解析到异步任务 ID。它不是本地余额问题，也不是普通渠道不可用；它代表“任务提交响应形态不符合当前 adaptor 预期”或“上游返回错误但没有被当前结构正确识别”。

### 现象与证据

CN 数量：198。

特征：

- 时间集中在 2026-06-10。
- 日志里是 benchmark 风格 token。
- 模型是 `wan2.6-r2v`。
- 渠道是 #34。
- 同一个请求反复重试，每次都返回 `task_id is empty`。

代表日志：

```text
channel error (channel #34, status code: 500): task_id is empty
record error log: userId=1, channelId=34, modelName=wan2.6-r2v, tokenName=benchmark_image2, content=status_code=500, task_id is empty
```

### 具体情况

当前阿里视频任务 adaptor 期望 DashScope 异步视频任务提交响应里有 `output.task_id`。这和现有 adaptor 设计以及阿里异步任务响应形态一致。

这些错误高度集中在 `wan2.6-r2v` 媒体计费/参数调整时期，可能原因是：

- 当时还在使用旧请求形态。
- 上游返回了错误响应，但没有落在我们当时识别的 `code/message` 结构里。
- channel #34 或模型映射当时指向了不兼容 endpoint。

目前证据不足以扩大 task id 解析逻辑。比如贸然兼容顶层 `id`，如果没有上游成功响应作为证据，可能会掩盖真实的供应商错误。

### 上游怎么看

没有找到上游对 Ali video `task_id is empty` 的直接 issue。上游类似任务 adaptor 通常也是“提交成功必须有 task id，否则是 provider invalid response”。

### 影响

任务提交失败。周边日志显示失败后有返还预扣费。影响看起来集中在 benchmark/测试流量，不像持续线上用户大面积故障。

### 是否需要修改

暂不建议继续修改 task id 解析。`wan2.6-r2v` 相关修复（`8e4f6fbd`、`33ec1d9f` 等）已经包含在 CN/SG 当前生产版本中；按当前版本上线时间切分日志后，`task_id is empty` 两地均未再出现。因此本项可标记为“已修复上线/当前版本后未复现”，后续只需要保留观察。

建议动作：

1. 如果 DB/task 日志里还保留原始响应，拉一条 `request_id` 对应的 provider response。
2. 回看 2026-06-10 channel #34 endpoint 和模型映射。
3. 在 CN test 或当前生产渠道重跑受控 `wan2.6-r2v` e2e。
4. 如果原始响应是带数字 code 的结构化错误，E005 修复可以帮助暴露真实错误。
5. 只有原始响应证明阿里成功响应形态变了，才加 alternate task id 兼容。

## E005 阿里错误 `code` 数字类型导致 JSON 解析失败

### 判定口径

E005 是明确的本地代码兼容问题：上游已经返回了 JSON 错误响应，但 DTO 字段类型过窄，导致网关在解析上游错误时先失败。它不是供应商业务失败本身，而是我们没有正确承接供应商错误结构。

### 现象与证据

CN 归类数量：8 行。实际对应约 4 个请求级失败，因为同一个错误会同时记录 channel error 和 relay error。

代表日志：

```text
json: cannot unmarshal number into Go struct field AliResponse.AliError.code of type string
channel error ... json: cannot unmarshal number into Go struct field AliResponse.AliError.code of type string
```

### 具体情况

阿里错误响应里的 `code` 有时是数字，但当前 DTO 要求字符串。结果是：供应商本来返回了一个可读错误，但网关先在 JSON 反序列化阶段失败，把真实供应商错误吞掉，变成“本地 bad response body”。

### 上游怎么看

没有找到 `AliResponse.AliError.code` 的直接上游 issue。检查过的 upstream/main 相关代码里，阿里错误 code 仍是 string-only。

### 影响

频率不高，但这是明确正确性问题：

- 供应商真实错误细节丢失。
- 错误分类误导排查。
- 排查 `wan2.6` / 阿里图片视频链路时更困难。

### 是否需要修改

建议修改。

建议修复方向：

1. 使用已有 `dto.StringValue` 接收阿里错误 `code` 字段，让 JSON 字符串和数字都能解析。
2. 构造 OpenAI-compatible 错误时再转回普通 string。
3. 补单测覆盖 top-level、output-level、result-level 数字 code。

当前状态：

- 修复 commit `169c6189 Fix Ali numeric error code parsing` 已包含在 CN 当前生产版本 `v1.6.1-tracenex`。
- CN 当前版本上线后未再出现该错误，CN 可标记为“已修复上线”。
- SG 当前生产版本 `sg-3d179de6` 未包含 `169c6189`，但 SG 本轮日志没有观测到该错误；建议下次 SG 发版补入。
- 定向测试已通过：`GOSUMDB=sum.golang.org go test ./relay/channel/ali ./relay/channel/task/ali`。

## E006 上游参数/模型校验失败

### 判定口径

E006 指请求已经到达上游或 provider adapter，失败原因是参数、模型、endpoint、能力边界不匹配。这里的“上游参数校验失败”不是 TraceNex 用户余额问题，也不是渠道不可用；它说明请求形态和被路由到的供应商能力不一致，或客户端输入本身越界。

### 现象与证据

CN 数量：44。

CN 代表日志：

```text
The parameter `model` specified in the request are not valid: the requested model doubao-seedream-4-0-250828 does not support this api.
The parameter `image` specified in the request is not valid: invalid url specified.
The model or endpoint seedream-4-0-250828 does not exist or you do not have access to it.
```

SG 数量：1,512。

SG 代表日志：

```text
Unsupported parameter: 'stop' is not supported with this model.
max_tokens is too large: 32000. This model supports at most 16384 completion tokens.
Invalid 'tools': array too long. Expected an array with maximum length 128, but got length 153/202.
Input tokens exceed the configured limit of 272000 tokens.
Unrecognized request argument supplied: reasoning_effort
Unknown parameter: 'thinking'
The 'metadata' parameter is only allowed when 'store' is enabled.
image is required
not supported model for image generation, only imagen models are supported
```

### 具体情况

这是混合类错误：

- 一部分是纯客户端错误：图片 URL 非法、tools 太多、tokens 太多、`max_tokens` 超上游限制。
- 一部分是供应商/模型能力差异：某些模型不支持 `stop`、`reasoning_effort`、`thinking`、`metadata`。
- 一部分说明模型路由到了能力不匹配的 provider endpoint。

SG 最大单项是 `Unsupported parameter: 'stop' is not supported with this model`，约 803 次。这可能需要按渠道/模型做定向字段过滤，但要小心：直接丢弃 `stop` 会改变用户请求语义，不能全局静默删除。

### 上游怎么看

没有找到精确的 `stop not supported` 上游 issue。上游通常通过 provider adaptor 或 channel settings 做字段裁剪，但覆盖范围依 provider/model 而异。

### 影响

SG 用户可见失败量较高。此类错误一般不应扣费，但反复重试会浪费延迟和上游容量。

### 是否需要修改

可能需要，但要先拆分细项。

建议动作：

1. 按 channel ID 和 model 拆解 `stop not supported`。
2. 确认对应上游是否真的禁止 `stop`，还是某个兼容层的 bug。
3. 优先使用“渠道/模型能力配置”而不是硬编码全局删除。
4. 如实现过滤，需要在日志里记录 `removed_fields` 或 request conversion，便于审计。
5. 给每个被删除字段补 conformance/e2e 用例。

## E007 上游限流 / 高需求 / 供应商账号限制

### 判定口径

E007 只收供应商侧的容量、限流、价格层级、账号限制类错误。它和 E002 的区别是：

- E002：TraceNex 本地用户余额/额度不足，通常还没打到上游。
- E007：供应商返回限制，例如上游 `429`、`Retry-After`、Azure pricing tier 限制、供应商账号 hard limit。

因此，`Billing hard limit has been reached.` 在本报告里不是“用户余额不足”，而是供应商侧账号/计费限制。

### 现象与证据

SG 数量：约 568-577。细分口径如下：

- `Too Many Requests`：467 条。
- `This model is currently experiencing high demand`：46 条。
- Azure `exceeded the call rate limit`：43 条。
- `request is limited`：12 条。
- `Billing hard limit has been reached`：9 条。

少量日志可能同时被 channel error / relay error 记录，所以这里使用区间描述，最终准确数量要按 request id 去重。

代表日志：

```text
Too Many Requests
This model is currently experiencing high demand.
Your requests to gpt-image-2 ... exceeded the call rate limit ... retry after 54 seconds.
Billing hard limit has been reached.
```

### 具体情况

这些是供应商容量、供应商价格层级、供应商账号额度或上游限流问题。在样本里，有些 `Too Many Requests` 出现在一次请求的前几次尝试中，后面又重试成功。

### 上游怎么看

查到上游 issue #2841，内容是 Redis rate limiter 高并发竞态导致本地限流失效，但它不能直接解释这里的 provider `429`。这里更像上游真实限流或供应商账号限制。

### 影响

用户延迟上升，失败率上升。过度重试还可能放大上游压力。

### 是否需要修改

不建议改核心 relay 逻辑。

建议动作：

1. 调整单渠道并发和重试策略。
2. 供应商返回 `Retry-After` 时尽量尊重。
3. 对重复 429 的渠道加 cooldown。
4. 对热点图片/GPT 渠道提高上游额度或拆分渠道。

## E008 流式请求客户端断开 / `context canceled`

### 判定口径

E008 指流式响应过程中下游客户端断开、HTTP context 被取消，或 stream scanner 在收尾阶段遇到连接关闭。它不是单纯的上游失败；是否是 bug 取决于断开后本地是否还正确完成 usage 统计、退款/扣费和日志落账。

### 现象与证据

CN 数量：3。

SG 数量：873。

代表日志：

```text
stream ended: reason=client_gone end_error="context canceled"
send_stream_response_failed: request context done: context canceled
scanner error: http2: response body closed
timeout waiting for goroutines to exit
```

### 具体情况

这表示下游客户端断开，或流式链路没有正常结束。单独看它不一定是 bug；真正的风险是：如果上游已经生成了 token，但 final usage 没收到，本地计费可能出现少扣或多扣。

这次文本日志分类中，SG 的 `zero_usage_billing` 桶被归为 0，是因为分类脚本先匹配了 stream/client-disconnect 行。但早前 Top 样本里确实出现过：

```text
total tokens is 0, cannot consume quota...
```

所以这里不能只靠文本日志，需要专门查消费日志和账务字段。

### 上游怎么看

上游已有多个相关公开 issue：

- #5235：Streaming Responses 请求在客户端断开且 final usage 缺失时，可能退还全部预扣费。
- #4463：客户端断开后 StreamScannerHandler 停止读取上游，导致 usage 丢失 / 计费缺失。
- #4168：流式失败且 `completion_tokens=0` 时仍按 prompt tokens 扣费。
- #5222：改进流式错误分类和日志可读性。

上游还没有形成统一结论。#5235 里维护者质疑：如果已经收到上游 chunk，理论上是否应该出现 `summary.TotalTokens == 0`。这说明这个问题必须用精确复现和账务审计来判断。

### 影响

潜在影响高，因为可能涉及计费正确性。即便不影响账务，也会制造大量错误日志。

### 是否需要修改

不能盲改，必须先做专项账务审计。

建议动作：

1. 查询 SG 消费日志中 `stream_status.end_reason=client_gone`、`quota=0`、prompt/completion tokens、预扣费字段。
2. 如有供应商账单或请求 ID，可和上游计费对齐。
3. 增加 fake streaming upstream e2e：
   - 发送若干 chunk 后不返回 final usage；
   - 客户端收到部分 chunk 后主动断开；
   - 上游 EOF 但没有 usage。
4. 再决定策略：
   - 维持现有退款；
   - 保留预扣费；
   - 最小扣费；
   - 按 provider/model 配置 fallback。

## E009 上游网络失败 / 代理超时 / 524

### 判定口径

E009 指已经尝试请求上游，但出现 TCP reset、上游 524、deadline exceeded、proxy read timeout 等网络/代理/供应商超时问题。它和 E007 的区别是：E007 是上游明确限流/账号限制，E009 是连接或超时失败。

### 现象与证据

CN 应用层数量：2。

代表日志：

```text
do request failed: Post "https://api.apipro.ai/v1/chat/completions": read tcp ... connection reset by peer
```

SG 应用层数量：238，另有 Nginx upstream 事件。

代表日志：

```text
upstream error: do request failed
bad response status code 524
Deadline expired before operation could complete.
The origin web server did not return a complete response within the 120-second Proxy Read Timeout window.
```

### 具体情况

这是供应商、网络或代理超时失败。一部分是 Cloudflare 风格 524，一部分是 TCP reset，一部分是上游操作超时。没有看到本地 panic。

### 上游怎么看

没有发现针对这些供应商/错误的直接上游 issue。一般属于网关常规 timeout/error handling。

### 影响

用户请求 transient failure，可能触发重试并增加延迟。

### 是否需要修改

先不改核心代码。

建议动作：

1. 按 channel ID 和 upstream domain 拆解。
2. 按模型类型调 timeout：长上下文、图片、视频不应套普通 chat 超时。
3. 对重复 524/timeout 渠道加健康检查或 cooldown。
4. 如有必要优化 524 和本地 500 的错误分类。

## E010 上游内容安全拒绝

### 判定口径

E010 指供应商内容安全系统拒绝 prompt、输入媒体或输出内容。它不是网关稳定性问题，也不是用户余额问题；网关通常只能透传或规范化提示。

### 现象与证据

SG 数量：167。

代表日志：

```text
The response was filtered due to the prompt triggering Azure OpenAI's content management policy.
Input data may contain inappropriate content.
```

### 具体情况

供应商内容安全系统拒绝了 prompt 或输出。这是上游策略行为。

### 上游怎么看

不是 new-api 问题，是供应商内容安全策略。

### 影响

用户可见失败。通常不能通过网关代码绕过。

### 是否需要修改

不改核心代码。

建议动作：

1. 保留清晰用户提示。
2. 在 metrics 里把安全拒绝单独分类，不要混入 gateway failure。

## E011 Nginx TLS 握手噪声

### 判定口径

E011 指请求在 TLS 握手或 TLS 读阶段已经失败，没有进入 Fy-api 应用。它不是应用错误，不应计入 API 可用性失败率。

### 现象与证据

CN Nginx：159 条，绝大多数是 TLS 噪声。  
SG Nginx：407 条，绝大多数是 TLS 噪声。

代表日志：

```text
SSL_do_handshake() failed (SSL: error:0A00006C:SSL routines::bad key share)
SSL_read() failed (SSL: error:0A0001BB:SSL routines::bad record type)
```

### 具体情况

这类请求通常来自公网扫描器、异常 TLS 客户端或探测流量，基本没有进入应用。

### 上游怎么看

和 new-api 无关。

### 影响

主要是日志噪声。

### 是否需要修改

不改应用代码。

建议动作：

1. 不纳入应用错误告警。
2. 如 SLS 噪声太大，可在日志分析或 Nginx error log 等级上做过滤。

## E012 Nginx upstream timeout / premature close / body too large

### 判定口径

E012 指 Nginx 作为反向代理观察到 upstream 超时、连接提前关闭，或客户端请求体超过 Nginx 限制。它发生在代理层，不等同于 Go 应用 panic；需要结合应用日志和部署时间判断。

### 现象与证据

SG 代表日志：

```text
upstream timed out while reading response header from upstream: POST /v1/chat/completions
upstream prematurely closed connection while reading upstream: POST /v1/messages?beta=true
client intended to send too large body: 17408000 bytes
```

### 具体情况

这些是代理层症状，可能原因：

- 长模型响应超过 Nginx upstream timeout。
- 应用容器重启、部署切流或 stream 中途关闭。
- 客户端上传体超过 Nginx `client_max_body_size`。

### 上游怎么看

只和流式问题有间接关系，不是 new-api 明确上游 bug。

### 影响

数量不大，但用户会看到失败。

### 是否需要修改

先不改代码，可能需要改 Nginx 配置。

建议动作：

1. 对比时间戳和 deploy log。
2. 检查 `/v1/messages`、图片、视频路由的 timeout 和 body size。
3. 如果多模态请求本来就会较大，给对应路由设置更合理的 body size。

## E013 上游内部错误 / 泛化 `openai_error`

### 判定口径

E013 指供应商明确返回 5xx、内部错误或泛化 `openai_error`。它不是参数校验失败，也不是网络连接失败；网关通常已经收到供应商响应并进行了透传/包装。

### 现象与证据

CN 尾部样本：

```text
The service encountered an unexpected internal error. Request id: ...
openai_error
```

SG 尾部样本：

```text
openai_error
The server had an error while processing your request.
```

### 具体情况

供应商返回了泛化 5xx 或内部错误，网关只是透传/包装。

### 上游怎么看

没有直接上游 issue。属于供应商失败处理。

### 影响

用户短暂失败。

### 是否需要修改

不改，除非要增强分类、重试或 channel cooldown。

建议动作：

1. 按 channel/provider 追踪。
2. 对重复 5xx 的渠道考虑 cooldown。
3. 保留供应商 request id，方便找供应商支持。

## E014 图片 API 能力不匹配

### 判定口径

E014 是 E006 的图片子类，专门指图片生成/编辑路由里请求字段、模型、endpoint 或供应商能力不匹配。它可能是客户端少传图片，也可能是模型被路由到不支持的图片供应商能力。

### 现象与证据

主要在 SG，已计入 E006。

代表日志：

```text
image is required
not supported model for image generation, only imagen models are supported
Unknown parameter: 'response_format'
You uploaded an unsupported image.
gpt-image-2 ... exceeded the call rate limit
```

### 具体情况

图片供应商之间参数能力差异很大。一部分是用户输入错误，例如缺图片、上传格式不支持；另一部分是网关/供应商兼容问题，例如 `response_format` 或模型被路由到了不支持的图片 endpoint。

### 上游怎么看

没有找到这些精确错误的上游 issue。

### 影响

图片生成/编辑请求失败。

### 是否需要修改

可能需要定向校验或过滤，但要先按 channel/model 拆开。

建议动作：

1. 按 `/v1/images/generations` 和 edits 路由拆分。
2. 按 model/channel 拆分。
3. 确认当前 Azure GPT image 丢弃 unsupported `response_format` 的 overlay 是否已经部署到 SG。
4. 对热点图片渠道跑 image conformance e2e。

## E015 请求过大 / token 或工具数量超上游限制

### 判定口径

E015 是 E006 的大小/数量限制子类，专门指 tokens、tools 数量、`max_tokens`、HTTP body size 等超过客户端、网关或供应商上限。它不是渠道没钱，也不是供应商随机故障。

### 现象与证据

主要在 SG，已计入 E006。

代表日志：

```text
Input tokens exceed the configured limit of 272000 tokens. Your messages resulted in 625231 tokens.
Invalid 'tools': array too long. Expected maximum length 128.
max_tokens is too large: 32000. This model supports at most 16384 completion tokens.
client intended to send too large body: 17408000 bytes
```

### 具体情况

客户端请求超过供应商或代理限制。不是网关 panic。

### 上游怎么看

没有直接上游 issue，是预期校验。

### 影响

用户失败，并浪费入口带宽/解析/上游尝试成本。

### 是否需要修改

可选做前置校验和体验优化。

建议动作：

1. 对已知模型限制，在请求上游前直接返回清晰 400。
2. 增加模型能力元数据，告诉客户端最大输出、tools 数量、body 大小等限制。
3. 不建议静默截断用户 payload。

## 四、建议的下一步排查顺序

1. E008：先做 SG 流式断连计费审计，因为当前版本上线后仍有 777 条相关错误，且可能影响钱。
2. E007/E009：处理 SG 供应商限流、容量和 timeout，因为当前版本上线后仍大量出现。
3. E006：拆解 SG 参数/模型能力错误；`stop` 精确错误当前版本后未复现，但其它参数/图片能力错误仍在出现。
4. E001：清理渠道可用性配置，这是高频但偏运营。
5. E005：CN 已修复上线；SG 当前未包含该修复但未观测到此错误，建议下次 SG 发版补入。
6. E004：当前版本后未复现，保留观察；如果复现再查原始上游响应。

## 五、修改准入标准

同一错误族至少满足下面任一条件，再进入代码修改：

- 能用确定性输入在本地或测试环境复现。
- 原始供应商响应证明存在 schema 兼容缺口。
- 账务审计证明存在少扣、多扣或异常退款。
- 当前行为把客户端/供应商校验错误错误地返回成 5xx，或隐藏了关键上游错误。
- 修复可以按 channel/model 控制，并且有 conformance/e2e 测试覆盖。
