# TraceNex 定制清单（OVERLAY.md）

> 最后更新：2026-07-06（刷新 upstream sync PR 到 1e80ce03）
> 维护人：<你的名字>
> 上游基线：new-api @ `1e80ce03` (2026-07-06)
>
> **重要：上游 v1.0（commit `a42b39760`，2026-04-28）把整个老前端搬到了 `web/classic/`，并行新建了 `web/default/`（React 19 + TypeScript + Rsbuild + Base UI + Tailwind）。TraceNex 选择路径 A：所有前端 overlay 跟随 `web/classic/` 路径，runtime theme 锁死在 `"classic"`，不允许切到 default。详见 `docs/上游v1.0前端重写炸弹-影响分析与对策.md`。**

本文件记录所有 TraceNex 相对于 `upstream/main` (QuantumNous/new-api) 的私有改动。
每次从上游 merge 时对照本清单处理冲突。

---

## 维护约定

- **新增文件优先**：定制能力尽量做成独立文件（`*_export.go`, `UsageLogsExportButton.jsx` 等），零上游冲突
- **必须修改上游文件时**：加 `// Fy-api overlay: ...` 注释，便于 merge 时辨认
- **每次 upstream sync 后**：检查本文件所有条目是否还有效

---

## 后端定制

### B-1 [brand] 系统名
- **文件**：`common/constants.go`
- **修改**：`var SystemName = "TraceNex"`（原 `"New API"`）
- **新增变量**：`var MaxLogExportItems = 50000`
- **冲突风险**：低（上游很少改这两行）
- **Merge 策略**：如果 upstream 又加了变量，手动合并到此文件
- **建议长期改造**：改成 overlay/brand/brand.go 里的 init() 函数覆盖，避免 merge

### B-1.1 [brand] 启动日志品牌化
- **文件**：`main.go` 第 ~52 行
- **修改**：`common.SysLog("New API " + ...)` → `common.SysLog(common.SystemName + " " + ...)`
- **目的**：让启动日志 `TraceNex started` 跟随 SystemName，避免写两处
- **冲突风险**：低（单行，带 `// Fy-api overlay:` 注释）

### B-2 [csv-export] 日志 CSV 导出（新增）
- **新增文件**：
  - `controller/log_export.go`（ExportAllLogs / ExportUserLogs / writeLogsCSV）
  - `model/log_export.go`（GetAllLogsForExport / GetUserLogsForExport / attachChannelNames）
- **修改文件**：`router/api-router.go`（注册 /api/log/export + /api/log/self/export，仅 2 行）
- **导出字段**：CSV 镜像使用日志表格，包含 request_id，并从 `logs.other` 解析 `cache_tokens` / `cache_write_tokens` / `cache_creation_tokens*` 导出“缓存读”“缓存写”token 列
- **ClickHouse 兼容**：导出查询复用 `model/log.go` 的显式文本过滤 helper；ClickHouse 日志库下不使用 `LIKE ... ESCAPE '!'`，排序跟随 `created_at/request_id`，避免 CSV export 和页面查询语义分叉
- **冲突风险**：低（独立文件 + 2 行 router 注册）
- **Merge 策略**：router 两行加在 `logRoute.GET("/self/search", ...)` 之后，若 upstream 也改了 logRoute，手动对齐位置

### B-3 [docs] CLAUDE.md Rule 5 改写
- **文件**：`CLAUDE.md` 第 5 条 Rule
- **修改**：把"禁止修改 new-api 品牌"改为"Apache 2.0 合规 attribution + 允许品牌定制"
- **冲突风险**：高（上游 Rule 5 会持续维护）
- **Merge 策略**：每次 upstream 改 Rule 5 都需要人工对齐，保留 Apache 2.0 合规措辞

### B-4 [gitignore] `.cursor/`、`*.log` 和 Python 缓存
- **文件**：`.gitignore`
- **修改**：新增 `.cursor/`、`*.log` 和 `__pycache__/`
- **冲突风险**：极低

### B-5 [docker] Dockerfile 国内部署适配
- **文件**：`Dockerfile`
- **修改**：
  1. **去掉 `@sha256` 摘要 pin**：三个 base image（`oven/bun:1`、`golang:1.26.1-alpine`、`debian:bookworm-slim`）只留 tag
  2. **添加 Go 模块国内代理**：`ENV GOPROXY=https://goproxy.cn,direct` + `ENV GOSUMDB=sum.golang.google.cn`
- **原因**：
  1. 阿里云 Container Registry mirror 不支持按摘要拉取（返回 `denied: requested access to the resource is denied`）
  2. 国内 build 主机无法直连 `proxy.golang.org`（Google 域被墙），`go mod download` 超时
- **代价**：失去摘要级可重现性（见下）；`direct` fallback 允许仍能从原始 VCS 拉模块
- **兜底**：供应链完整性由 `go.sum` / `bun.lock` 保证，base image 小浮动不影响产物
- **冲突风险**：低（上游偶尔刷 SHA；GOPROXY 注入属于 build-env 配置，不太可能冲突）
- **Merge 策略**：上游 bump SHA 时，把新 SHA 更新到文件顶部注释里；保留 tag-only 的 FROM 行和 GOPROXY ENV

### B-6 [deploy] Fabric 服务端构建发布自动化
- **新增文件**：`fabfile.py`
- **用途**：本地只执行 Fabric；远端 ECS 在 `/root/Fy-api` 拉取 Git ref，用 `git archive` 生成干净临时构建目录后 Podman 构建镜像、推送 ACR，再调用 `scripts/prod/06-deploy-blue-green.sh` 蓝绿发布；也支持新加坡新机 `bootstrap-system`
- **默认连接**：`cn=root@8.136.146.211:58422`（`~/.ssh/tracenex_XN.pem`），`hk=root@47.83.137.1:58422`，`cn-test=root@8.156.88.148:58422`，`hk-test=root@47.86.175.72:58422`；默认源码目录 `/root/Fy-api`；均可用 `FYAPI_*` 环境变量覆盖
- **冲突风险**：极低（新增根目录运维入口，不改 upstream 业务代码）
- **Merge 策略**：保留文件；若部署脚本参数变化，同步更新 `deploy` / `release` 任务

### B-6.1 [deploy/nginx] 初始化时提升 Nginx upstream 连接上限
- **修改文件**：`scripts/prod/03-setup-nginx.sh`
- **背景**：2026-06-26 HK 生产环境出现管理台静态资源与 API 请求被 Nginx 直接打成 `500/502`，错误日志为 `worker_connections are not enough while connecting to upstream`。机器本身 `ulimit -n` 足够，但 `/etc/nginx/nginx.conf` 仍停留在默认 `worker_connections 768`。
- **行为**：在写站点配置前，脚本会把主配置中的 `worker_connections` 提升到 `8192`，并补上 `worker_rlimit_nofile 65535`，避免新机或重装后回落到过低默认值。
- **冲突风险**：低（仅修改运维脚本，不影响业务代码）
- **Merge 策略**：若 upstream 后续也开始管理 nginx 主配置，保留本条“提升连接上限”的语义即可，具体数值按线上压测结果再定。

### B-6.2 [session] 可配置共享 Cookie 域
- **修改文件**：`common/constants.go`、`common/init.go`、`common/session.go`、`main.go`、`config/fy-api.env.example`
- **新增测试**：`common/session_test.go::TestSessionOptionsCookieDomain`
- **背景**：HK 生产同时保留 `api.aitracenex.com` 和 `www.aitracenex.com` 入口。浏览器默认 host-only session cookie 导致用户在一个子域登录后跳到另一个子域时看起来像“自动退出”。
- **行为**：新增 `SESSION_COOKIE_DOMAIN` 环境变量；默认空值保持 upstream host-only 行为。生产可设置为 `.aitracenex.com`，让 `api` / `www` 子域共享同一份 session cookie，迁移期内两个入口都可继续访问；已登录老用户访问原域名时会重新签发共享域 cookie，降低切到主域时掉登录的概率。2026-07-06 同步 upstream `SESSION_COOKIE_SECURE` / `SESSION_COOKIE_TRUSTED_URL` 后，`SessionOptions()` 同时应用 domain 与 secure 设置，两套配置不互斥。
- **冲突风险**：低（`main.go` session options 小范围抽函数；默认行为不变）
- **Merge 策略**：若 upstream 后续支持 session cookie domain，优先采用 upstream 配置名；保留空值 host-only、显式 `.aitracenex.com` 共享的语义。

### B-7 [benchmark] 渠道基准测试工具链（channel-benchmark）
- **新增目录**：`scripts/channel-benchmark/`
  - `README.md` —— 顶层导航，解释 `fy-benchmark` 单入口、目录结构和输出结构
  - `RUNBOOK-channel-benchmark.md` —— 完整渠道/模型测试执行手册，默认只暴露 `benchmark.yaml` + `fy-benchmark`
  - `install-env.sh` —— 一键创建 Python venv、安装 benchmark CLIs、按需安装 heavy extras，并重新生成本地 deterministic fixtures
  - `py/` —— Python 工具链（共享一个 venv / 一个 JSONL schema）
    - `benchmark.yaml` —— 用户入口配置模板；复制成 `benchmark.local.yaml` 后只填 `base_url` / token / `channel_id` / model list / mode
    - `fy_benchmark/` —— 统一编排器（注册 `fy-benchmark`）：读取单配置、按模型严格串行、自动生成底层 YAML、记录模块耗时日志、调用 `fy-score` 输出 `scorecard`
    - `fy_smoke/` —— 基础烟测 + Prometheus exporter（admin 渠道查询、stream/non-stream、E2E/TTFT/ITL/usage、JSON/CSV、`--long-thinking`）
    - `fy_loadtest/` —— 并发阶梯压测（E2E/TTFT/ITL/TPOT 分位、RPS、goodput）
    - `fy_image_loadtest/` —— 图片生成持续压测（固定每渠道 worker 数、打 `/v1/images/generations`、支持多渠道同时 pin 到指定 channel id，持续跑到 Ctrl+C）
    - `fy_quality/` —— 质量评分（7 种 grader + 双裁判 rubric + 磁盘缓存）
      - `perturbation.py` —— 确定性扰动（`whitespace` / `trailing_marker` / `synonym`）防训练集污染
      - `datasets/public/quality.jsonl` —— 起手 15 条烟测样本（带扰动示例）
      - `datasets/private/` —— 用户私有题库目录（整个目录 gitignore）
    - `fy_canary/` —— 模型替换检测（alignment + drift + MMD 三种探针）
      - `baseline.py` —— v2 schema 带 `recorded_at_iso` / `n_probes` / `total_samples` / `fy_canary_version`；v1 文件向后兼容
      - `cli.py` —— `baseline` / `audit` / `verify-baseline` 三个子命令；`audit` 默认拒绝超过 30 天的 baseline
    - `fy_image_conformance/` —— 图片协议一致性 + 质量 + 安全（六阶段：探针→冒烟→API兼容→输出验证→Phase A/B 质量→安全）
    - `fy_image_canary/` —— 图片金丝雀真实性检测（5A vendor 对比 + 5B 指纹/跨渠道/能力边界）
    - `fy_score/` —— 统一评分器（五维度加权，文本/图片不同权重，产出 scorecard）
    - `fy_poc_loadtest/` —— 按 `bugs/POC压测方法.docx` / `bugs/报告模板.docx` 口径输出客户 POC 压测报告（三场景：23/1k/7k tokens；并发 1/10/20/30/40/50/64/80/128/256；指标 TTFT/Latency/TPOT/tokens/s/成功率）
    - `benchmark-runs/` —— 默认运行输出目录；每次运行生成 `manifest.json` / `run-summary.*` / 自动生成的 `configs/` / 模块 `logs/` / `reports/scorecard.*`
    - `fixtures` extra —— fixture 生成脚本所需的轻量 Pillow 依赖；text MMD、image canary、tiktoken 仍保持 opt-in extras
    - `fixtures/` —— 协议/参数兼容测试用的可提交小素材（图片编辑 source/mask、图片/视频参考图、短音频、短视频），全部本地生成，不含客户数据或 provider 输出
    - `unified-benchmark-runner-plan.md` —— 统一 `fy-benchmark` 编排器长期规划：CLI/YAML 分工、CN/HK 路由、协议/参数兼容矩阵、strict 模式、fixture/manifest/resume/compare 设计
    - `seedance_gateway_vs_volcengine.py` —— Seedance 2.0 网关 vs 火山直连对照脚本，用于排查上游内容安全和网关转换差异
    - `tests/` + `tests_quality/` + `tests_canary/` + `tests_image_canary/` —— 102 个 e2e 测试
  - `docs/seedance-real-person-video-support-plan.md` —— 基于 DJLine 可信素材链路整理的 SD2.0 真人视频支持方案
- **用途**：
  1. **完整渠道验收**（fy-benchmark）—— `fy-benchmark -c benchmark.local.yaml` 串行执行 smoke / conformance / integrity / loadtest / quality / canary / image 模块，并生成 `run-summary.md` + `reports/scorecard.md`
  2. **烟测/监控**（fy-smoke）—— 快速看 TTFT / 存活 / usage，并可用 `--prom-listen` 暴露 Prometheus metrics
  3. **压测**（fy-loadtest / fy-image-loadtest）—— 灰度上线前验证单渠道在 N 并发下的分位延迟、吞吐和错误分布
  4. **质量**（fy-quality）—— 默认 deterministic graders；需要主观/语义覆盖时显式启用 judge / embedding
  5. **反替换**（fy-canary / fy-image-canary）—— 有可信 baseline channel 时录 baseline，再审计目标渠道是否静默换模型
  6. **监控接入**（fy-smoke Prometheus mode）—— `fy-smoke -c smoke.yaml --prom-listen :9090 --prom-interval 5m` 常驻，给 Grafana 暴露 `channel_benchmark_ttft_seconds` / `_request_total{outcome=...}` / `_run_age_seconds` 等序列
- **冲突风险**：极低（完整独立子目录，不触碰 upstream 业务代码或构建链）
- **Merge 策略**：整个子树随上游同步走；保持独立工具链，不触碰主仓业务代码

### B-8 [gemini] 原生 pass-through 入口保留客户端 API 版本
- **修改文件**：`relay/channel/gemini/adaptor.go`（`GetRequestURL`，紧接 `GetGeminiVersionSetting` 调用之后追加分支）
- **修改文件**：`setting/model_setting/gemini.go`（默认 `VersionSettings` map 显式钉住 image-preview 模型到 `v1beta`）
- **新增测试**：`relay/channel/gemini/relay_gemini_usage_test.go::TestGeminiAdaptorGetRequestURLPreservesNativeVersion`（4 个 sub-test，覆盖 native v1beta、native v1、imagen native v1beta、OpenAI 兼容入口回落 model_setting 四条路径）
- **背景**：海外 HK 部署反馈 `gemini-3-pro-image-preview` 的 native Gemini 调用报 `is not found for API version v1`。Upstream 原始实现里 `GetRequestURL` 一律读 `model_setting.GetGeminiVersionSetting()`，无视客户端写的 `/v1beta/...` 还是 `/v1/...` 路径；后台 `VersionSettings.default` 为 `v1beta` 但任何管理员改动或显式映射都会把 image-preview 这种只在 `v1beta` 暴露的模型踩坑
- **修复**：在 `RelayMode == constant.RelayModeGemini` 的 native pass-through 路径下，按 `info.RequestURLPath` 前缀（`/v1beta/` 或 `/v1/`）覆盖 `version`；OpenAI / Claude 兼容入口（`RelayModeChatCompletions` 等）继续使用 `model_setting`，行为不变。配合 `gemini.go` 显式钉住的 image-preview 模型，原生入口 + 兼容入口双层兜底
- **冲突风险**：低（adaptor.go 内只新增一段 `if` 分支，带 `// Fy-api overlay:` 注释；gemini.go 默认 map 只新增 key）
- **Merge 策略**：上游若改动 `GetRequestURL` 的版本拼接逻辑，保留这段 native pass-through 覆盖；若上游改 `defaultGeminiSettings.VersionSettings`，按需合并新增的 image-preview 项

### B-9 [claude] 默认过滤 `context_management` beta 字段
- **修改文件**：
  - `dto/channel_settings.go`（`ChannelOtherSettings` 新增 `AllowContextManagement bool`，紧跟 `AllowSpeed` 后）
  - `relay/common/relay_info.go`（`RemoveDisabledFields` 中新增一段 `delete(data, "context_management")`，并在函数顶部 doc 中登记该字段）
- **新增测试**：`relay/common/override_test.go::TestRemoveDisabledFieldsContextManagement`（3 个 sub-test：默认裁剪 / `AllowContextManagement=true` 保留 / channel pass-through 保留），同时把 `TestRemoveDisabledFieldsDefaultFiltering` 的输入扩展为含 `context_management`
- **背景**：海外 HK 反馈 `claude-sonnet-4-6` 调用上游返回 `context_management: Extra inputs are not permitted`。Anthropic 的 `context_management` 是 beta 功能，必须配合 `anthropic-beta: context-management-2025-...` header 才能用。客户端（含部分 SDK）会只在 body 里塞这个字段、不带 header，被 Anthropic schema 校验直接拒
- **upstream 现状**：upstream 在 `30cb3b8b`（2025-09-30 "feat: claude context editing"）只在 `dto.ClaudeRequest` 里加了 `ContextManagement` 字段做识别，没有像 `inference_geo / speed / service_tier` 那样接入 `RemoveDisabledFields` 默认过滤体系。这是 upstream 的遗漏；本仓库选择直接 overlay 修复，**不**向 upstream 提 PR

### B-10 [billing] wan2.6 图像/视频模型定价与展示
- **修改文件**：
  - `setting/ratio_setting/model_ratio.go`（新增 `wan2.6-i2v` / `wan2.6-r2v` / `wan2.6-r2v-flash` 视频基础倍率、`wan2.6-t2i` 单图价格）
  - `relay/channel/task/ali/constants.go`（Ali 视频模型列表新增 `wan2.6-i2v` / `wan2.6-r2v` / `wan2.6-r2v-flash`）
  - `relay/channel/task/ali/adaptor.go`（Ali 计费倍率新增 `wan2.6-r2v` / `wan2.6-r2v-flash`；`wan2.6` 默认分辨率改为 `720P`，其中 `wan2.6-r2v*` 默认 `size=1280*720`；分辨率校验前移到 `ValidateRequestAndSetAction` 并在 metadata 反序列化后再校验最终值；`wan2.6-r2v*` 使用 DashScope `input.reference_urls` + `parameters.size/audio/shot_type/watermark`，向后兼容 `input_reference` + `metadata.last_frame_url` 旧传参，且在没有顶层统一字段时保留 `metadata.input.reference_urls` 原生透传；`wan2.7-r2v*` 保留 `input.media` 数组；新增 `AliMediaItem` 结构体、`AliVideoInput.ReferenceURLs` / `AliVideoInput.Media` 字段、`AliVideoParameters.Ratio` / `ShotType` 字段；任务完成时按阿里 `usage.duration` 做实际秒数差额结算）
  - `relay/channel/task/ali/adaptor_test.go`（覆盖 `wan2.6` 默认分辨率、非法分辨率返回 400、完成态按实际时长结算、`wan2.6-r2v*` reference_urls 转换、`wan2.7-r2v*` media 转换、last_frame_url 向后兼容、Ratio metadata 透传、metadata 原生 reference_urls 保留、JSON 序列化断言）
  - `relay/common/relay_info.go`（新增 `TaskMediaItem` 结构体，以及 `TaskSubmitReq.ReferenceURLs` / `Media` / `Audio` / `ShotType` / `Watermark` 字段，支持用户通过 reference_urls 或 media 传入多个参考素材）
  - `service/task_polling.go`（`// Fy-api overlay:`：任务完成态先执行 adaptor 的实际用量结算，再让 per-call 任务跳过 token 重算，避免按次预扣的视频模型跳过 `usage.duration` 差额结算）
  - `service/task_billing.go`（`// Fy-api overlay:`：任务消费日志把 `OtherRatios` 写入 `other` JSON，便于 `seconds` / `resolution-*` 报表和 e2e 断言）
  - `web/classic/src/components/table/model-pricing/modal/components/ModelPricingTable.jsx`（遇到 `wan2.6` 视频模型时改为“视频计费”展示）
  - `web/classic/src/components/table/model-pricing/modal/components/VideoPricingDisplay.jsx`（新增分辨率/每秒价格表）
  - `scripts/ops/media_billing_e2e.py`（新增 cn-test/staging 媒体计费 e2e，用公开 API 覆盖图片固定价格、i2v/r2v 视频任务计费与日志结构化字段）
  - `docs/reports/2026-06-06-media-billing-audit.md`（生产只读聚合、根因、测试矩阵和 e2e 运行说明）
  - `docs/操作手册-视频模型接入.md`（对客户和运营说明 TraceNex 只暴露统一 OpenAI-like 视频任务协议，阿里 `input` / `parameters` 通过顶层便捷字段或 `metadata` 透传）
- **定价规则**：
  - `wan2.6-t2i`：`0.2` 元/张
  - `wan2.6-i2v` / `wan2.6-r2v` / `wan2.6-r2v-flash`：`720P=0.3` 元/秒，`1080P=0.5` 元/秒
- **背景**：上游现有模型价格表偏 token/quota 展示，不适合阿里万相这类按分辨率、按秒结算的视频模型；2026-06-10 复测浙江算力/ DashScope 示例后确认 `wan2.6-r2v*` 应使用 `input.reference_urls`（`input.media` 是 `wan2.7-r2v` 形态）；旧字段或错用 media 会导致上游不返回 task_id。最初版本还存在默认分辨率偏成 `1080P`、非法分辨率在预扣费后才 500、成功任务未按阿里实际时长结算的问题。2026-06-06 复查生产媒体日志时发现 `PerCallBilling` 会在轮询完成态提前跳过 adaptor 实际用量结算，已改为 adaptor actual quota 优先；同时补结构化 `OtherRatios` 日志字段和 cn-test e2e。
- **冲突风险**：中（`model_ratio.go` 和 `ModelPricingTable.jsx` 都是上游常改文件；`adaptor.go` 未来若继续扩充 Ali metadata 映射也可能冲突；`relay_info.go` 新增字段不影响上游已有字段）
- **Merge 策略**：上游若调整视频计费展示或 Ali adaptor，保留这组 `wan2.6` 固定价格规则；`wan2.6-r2v*` 必须使用 `input.reference_urls`，`wan2.7-r2v*` 才使用 `input.media`；不得回退到 `first_frame_url`/`last_frame_url`

### B-11 [relay] 请求体反序列化失误：500 → 400 + Go 字段名脱敏
- **新增文件**：
  - `common/json_error_sanitizer.go`（`SanitizeJSONUnmarshalError`：把 stdlib `*json.UnmarshalTypeError` / `*json.SyntaxError` / 字符串型 wrapped 错误转成用户安全格式 `invalid type for field "X": expected <json type>, got <json type>`，去掉 Go 结构体路径）
  - `common/json_error_sanitizer_test.go`（typed errors / syntax / wrapped string fallback / nil-safe / 切片包路径泄漏 / 嵌套 struct 字段）
  - `common/gin_unmarshal_test.go`（黑盒验证 `UnmarshalBodyReusable` 不再向调用方泄露 Go 路径）
- **修改文件**：
  - `common/gin.go`（`UnmarshalBodyReusable` 错误返回前过 `SanitizeJSONUnmarshalError`）
  - `controller/relay.go`（顶层 dispatcher 把 `ErrorCodeInvalidRequest` 用 `NewErrorWithStatusCode(... StatusBadRequest)` 显式钉到 400，原 `NewError` 默认 500 是 upstream 遗留 bug）
- **背景**：客户端传错参数类型（如 `max_tokens="abc"`）时，upstream new-api 在 `controller/relay.go:114` 用 `types.NewError(err, ErrorCodeInvalidRequest)` 没指定 statusCode → 默认 500。响应组装路径（`controller/relay.go:91 SetMessage(newAPIError.Error())`）跳过了 `MaskSensitiveError`，把 stdlib 错误 `json: cannot unmarshal string into Go struct field GeneralOpenAIRequest.max_tokens of type uint` 原样吐给客户端，既破坏 HTTP 语义（4xx 客户端错被当 5xx 服务端错，触发客户端误重试 / 误熔断），又泄露 Go 内部结构。客户的 v2.1 测试报告里 17 条 500 错误都是这一个根因
- **upstream 现状**：仓库里 `compatible_handler.go / claude_handler.go / gemini_handler.go / image_handler.go / rerank_handler.go / embedding_handler.go` 6 处都正确用了 `NewErrorWithStatusCode(... StatusBadRequest)`，唯独顶层 dispatcher 漏了一处。stdlib 错误脱敏 upstream 完全没做。**不向 upstream 提 PR**
- **冲突风险**：低（顶层 dispatcher 改 1 行；UnmarshalBodyReusable 错误处加 1 行；新增的 sanitizer 是独立文件）
- **Merge 策略**：若 upstream 修了同一行（用同样的 `NewErrorWithStatusCode`），merge 时 take theirs 即可；sanitizer 独立文件不会冲突

### B-12 [relay] /v1/messages 二级反序列化泄漏 + 图片块 nil-deref panic
- **新增文件**：
  - `dto/claude_parse_content_test.go`（`ClaudeMessage.ParseContent` 防泄漏：scalar content / 错类型 text 块 / 合法快路径）
- **修改文件**：
  - `common/utils.go`（`Any2Type[T]` 第二阶段 `json.Unmarshal` 错误也过 `SanitizeJSONUnmarshalError`，关闭 `dto.ClaudeMessage.ParseContent` / `ParseSystem` / `service.ClaudeToOpenAIRequest` 等所有借道 Any2Type 的二级 unmarshal 泄漏面）
  - `common/json_error_sanitizer.go`（`goTypeToJSONType` 增加切片/map/包路径折叠：`[]dto.ClaudeMediaMessage` → `array`、`map[K]V` → `object`、含点的包路径 → `object`，避免类型名直接外泄）
  - `service/convert.go`（`case "image"` 块前增加 `mediaMsg.Source == nil` 守卫，原代码在 `mediaMsg.Source.MediaType` 直接解引用，遇到 `{"type":"image"}` 缺 `source` 字段时 panic）
  - `relay/claude_handler.go`（两处 `ConvertRequestFailed` 改为 `NewErrorWithStatusCode(... StatusBadRequest)`，与 B-10 在 `/v1/chat/completions` 路径上的处理保持一致）
- **背景**：v1.5（B-10）只覆盖 `/v1/chat/completions` 的顶层 unmarshal。2026-05-09 用 fy-conformance 跑 CN 基线时发现 `/v1/messages` 路径上有 4 个同类 bug：
  1. `content=42` / `content=true` → 500 + `json: cannot unmarshal number into Go value of type []***.ClaudeMediaMessage`（`dto.ClaudeMessage.ParseContent` → `common.Any2Type` 二级 unmarshal 没经过 sanitizer）
  2. `content=[{type:text,text:42}]` → 500 + `Go struct field ***.text of type string`（同链路）
  3. `content=[{type:image}]` 缺 `source` 字段 → 500 + PANIC `runtime error: invalid memory address or nil pointer dereference`（`service/convert.go:161` 直接 `mediaMsg.Source.MediaType`）
- **upstream 现状**：upstream `Any2Type` 是直通函数，`service/convert.go` image 块也是无 nil 检查；同样的 panic 在 upstream 仍然存在。**不向 upstream 提 PR**（与 B-10 同策略，等下次周度同步时观察 upstream 是否补）
- **冲突风险**：中（`Any2Type` 是高频函数，签名不变；image case 加 4 行守卫；`claude_handler.go` 改 2 个 if 分支）
- **Merge 策略**：upstream 若改 `Any2Type` 签名（不太可能），同步时把 sanitizer 调用迁过去；image nil 守卫若 upstream 自行修了就 take theirs

### B-13 [relay] base64 图片缺失 MIME type 自动识别
- **修改文件**：
  - `dto/openai_request.go`（`MessageImageUrl.MimeType` 增加 `json:"mime_type,omitempty"`，`Message.ParseContent` 保留 `image_url.mime_type/mimeType`）
  - `service/file_service.go`（raw base64 文件在 MIME 为空时通过图片解码 / content sniff / HEIF 检测补齐 MIME）
- **新增测试**：
  - `dto/openai_request_mime_test.go::TestMessageParseContentPreservesImageURLMimeType`
  - `service/file_service_test.go::TestGetBase64DataInfersMimeTypeForRawBase64Image`
- **背景**：用例明细 116 的 `content_image_url` 传裸 base64 且不带 MIME type 时，TraceNex/上游适配层可能拒绝或向 Claude/Gemini 发送空 MIME，触发 `base64 mime type can not be empty`。现在能识别的图片会自动补成 `image/png` / `image/jpeg` 等；无法识别的内容仍交给后续 provider 白名单校验拒绝
- **冲突风险**：低（小范围解析/文件服务增强）
- **Merge 策略**：若 upstream 后续支持同类自动 MIME 推断，优先采用 upstream 实现，但保留 OpenAI `image_url.mime_type` 不丢失的测试语义

### B-14 [benchmark/image] gpt-image-2 压测工具与图片质量日志修正
- **新增目录**：`scripts/channel-benchmark/py/fy_image_loadtest/`
  - `cli.py` / `config.py` / `client.py` / `runner.py` / `report.py`
  - 命令：`fy-image-loadtest`
  - 用途：针对 `/v1/images/generations` 的持续压测，支持多渠道 pin、每渠道独立并发、`duration_sec` / `max_requests_per_channel` 结束条件、429 `retry-after` 冷却
- **新增文件**：`scripts/channel-benchmark/py/image-loadtest.yaml`
- **新增文件**：`docs/操作手册-gpt-image-2图片模型接入.md`（运营/客户侧 gpt-image-2 渠道配置、请求参数、示例和排障说明）
- **本地配置约定**：`scripts/channel-benchmark/py/image-loadtest.local.yaml`（gitignore，不入库；需要时从 `image-loadtest.yaml` 复制生成）
- **修改文件**：
  - `scripts/channel-benchmark/py/pyproject.toml`（注册 `fy-image-loadtest` CLI）
  - `scripts/channel-benchmark/README.md` / `scripts/channel-benchmark/py/README.md`（补图片压测说明）
  - `relay/image_handler.go`（`// Fy-api overlay:`：图片消费日志保留实际 `quality` 值，不再把 `high/medium/low` 统记成 `standard`）
  - `relay/channel/openai/adaptor.go`（`// Fy-api overlay:`：Azure `gpt-image-*` / `chatgpt-image-latest` 图片生成请求丢弃 `response_format`，避免 Azure 上游拒绝不支持的参数）
- **新增测试**：
  - `relay/channel/openai/adaptor_image_test.go::TestConvertImageRequestDropsAzureGPTImageResponseFormat`
  - `relay/channel/openai/adaptor_image_test.go::TestConvertImageRequestKeepsDallEResponseFormat`
- **背景**：
  1. 现有 `fy-loadtest` 固定打 `/v1/chat/completions`，不适合 `gpt-image-2`
  2. Azure `gpt-image-2` 链路对 `response_format` 不兼容，工具默认不再发送该字段；网关运行时也会在命中 Azure GPT image 模型时删除该字段，旧 DALL-E 模型继续保留
  3. 2026-05-15 CN 线上排查确认：channel `42` 和 `43` 共享同一个 Azure `base_url + key`，并非独立配额桶；本地图片压测配置已按此降并发标注
- **冲突风险**：低（benchmark 子树独立；`relay/image_handler.go` 仅一小段日志文案逻辑）
- **Merge 策略**：benchmark 子树整体保留；若 upstream 后续自带 image loadtest，可比较后择优；`relay/image_handler.go` 若 upstream 修复同类质量标签记录问题，merge 时优先采用 upstream 实现。2026-06-15 同步上游图片流/图片编辑实现时，保留本条 Azure `response_format` 过滤 overlay。

### B-15 [benchmark/image] 余额不足自动停机
- **修改文件**：
  - `scripts/channel-benchmark/py/fy_image_loadtest/client.py`（识别余额不足类错误）
  - `scripts/channel-benchmark/py/fy_image_loadtest/runner.py`（任一请求命中后触发全局优雅停机）
  - `scripts/channel-benchmark/py/tests/test_image_loadtest.py`（新增停机回归测试）
- **行为**：`fy-image-loadtest` 遇到 `余额不足` / `额度不足` / `insufficient quota` 等错误时，立刻停止继续发新请求，已在飞请求自然收尾后输出最终报告
- **冲突风险**：低（仅 benchmark 子树）

### B-16 [responses] Codex/OpenAI Responses 兼容清洗 + 失效密文单次降级重试
- **新增文件**：
  - `relay/common/openai_responses_sanitizer.go`（预清洗 `input[*].metadata`、`input[*].internal_chat_message_metadata_passthrough`；按需剥离 `encrypted_content`）
  - `relay/responses_retry.go`（识别 `encrypted content ... could not be verified`，触发单次去密文重试）
  - `relay/responses_retry_test.go`
  - `tests/e2e/responses_regression_e2e.py`（`hk-test` 黑盒验证 metadata 清洗与密文单次降级重试）
- **修改文件**：
  - `relay/responses_handler.go`
  - `relay/chat_completions_via_responses.go`
  - `relay/common/override_test.go`
  - `tests/e2e/README.md`
- **背景**：2026-06-26 HK 生产日志显示 `/v1/responses` 高频 400 主要分三类：`encrypted content could not be verified`、`input[*].metadata`、`input[*].internal_chat_message_metadata_passthrough`。上游 `new-api` 已有相关 issue（如 #4662 / #3240），但 `upstream/main` 尚无通用修复。TraceNex overlay 先做兼容兜底。
- **行为**：
  1. 正常转发前仅移除 OpenAI Responses 不接受的客户端内部字段；
  2. 若上游明确返回 `400 + encrypted content could not be verified`，则仅重试一次，并在重试时移除失效 `encrypted_content` 块，让请求退化为明文上下文继续执行；
  3. 原生 `/v1/responses` 与 chat→responses 兼容路径都保留已转换的 outbound JSON 作为重试输入，避免 body reader 消耗后无法降级；
  4. 非该特定错误不触发自动重试。
- **取舍**：这是 availability-first 兜底，不会尝试“解密”或伪造客户端密文状态；代价是重试后可能丢失隐藏推理上下文，但优先避免整次请求直接 400。
- **冲突风险**：中（`responses` 路径仍属上游活跃区域）
- **Merge 策略**：若 upstream 后续合入原生 `encrypted_content` 亲和 / 重试支持，优先采用 upstream；保留本地“只对特定 400 单次降级”的行为语义直到 upstream 证明等价。

### B-15.1 [benchmark/image] 图片协议一致性 + 质量 + 安全测试套件
- **新增目录**：`scripts/channel-benchmark/py/fy_image_conformance/`
  - `cli.py` / `config.py` / `client.py` / `probe.py` / `budget.py` / `report.py`
  - `suites/api_compat.py` / `output_valid.py` / `prompt_follow.py` / `perf.py` / `safety.py`
  - 命令：`fy-image-conformance`

### B-16 [deepseek] DeepSeek V4 `-nothink` / `-nothinking` 别名兼容
- **修改文件**：
  - `setting/reasoning/suffix.go`（`// Fy-api overlay:`：在 DeepSeek V4 既有 `-none` / `-max` 之外，兼容客户端常用的 `-nothink` / `-nothinking`，统一映射到 `thinking.type=disabled`）
  - `relay/channel/deepseek/constants.go`（对外模型列表补充 `deepseek-v4-pro-nothink` / `deepseek-v4-flash-nothink`，保留既有 `-none` 兼容名）
  - `setting/reasoning/suffix_test.go`、`relay/channel/deepseek/adaptor_test.go`（补别名解析与 OpenAI/Claude 适配回归测试）
- **背景**：CN 环境客户通过 CC Switch 使用 DeepSeek V4 时，更自然会写 `deepseek-v4-pro-nothink` / `deepseek-v4-flash-nothink`。当前仓库只识别 `-none`，导致 nothink 请求不会命中 DeepSeek V4 thinking 适配层。这里在网关侧补别名兼容，同时把对外模型名补齐到产品约定。
- **冲突风险**：低（仅 suffix 解析、模型常量与独立测试）
- **Merge 策略**：若 upstream 后续为 DeepSeek V4 增加 no-thinking 官方别名，优先采用 upstream；否则保留 alias 兼容层与 `-nothink` 展示名

### B-16.1 [deepseek] DeepSeek OpenAI-compatible protocol bridge for Codex / Claude Code
- **新增文件**：
  - `relay/channel/deepseek/responses_bridge.go`（`// Fy-api overlay:`：将 DeepSeek 渠道的 `/v1/responses` 请求降级为 OpenAI-compatible `/v1/chat/completions` 出站请求，并把 chat/completions 响应转换回 Responses 响应；支持非流式和基础流式文本、function tools / tool_choice / tool_calls；Codex 的 Responses `namespace` tools 会展开为 chat function tools，并在回包里尽量还原 `namespace`）
- **修改文件**：
  - `relay/channel/deepseek/adaptor.go`（`/v1/messages` 不再默认打 DeepSeek Claude 原生 `/anthropic/v1/messages`，而是先 Claude→OpenAI chat，再走 DeepSeek V4 thinking suffix 归一化；`/v1/responses` 走上面的 Responses→Chat bridge；最终出站格式记录为 `openai`，但客户端入口格式保持原值，便于响应再转回 Claude/Responses；出站前把 DeepSeek 不支持的 `developer` role 归一为 `system`，未知 role 归一为 `user`）
  - `relay/channel/deepseek/adaptor_test.go`（覆盖 Claude Messages→OpenAI chat、Responses→OpenAI chat、DeepSeek V4 `-nothink` thinking 注入、Responses function tools / namespace tools、Codex `developer` role 归一化、function_call history 和 chat tool_calls 响应回转）
- **限制**：当前 bridge 不支持 Responses stateful 能力（`previous_response_id` / `conversation` / `prompt`）和 `max_tool_calls`；OpenAI/Codex 内置工具（如 `web_search_preview` / `file_search` / `tool_search` / `image_generation`）会被静默过滤，不会传给 DeepSeek 上游；Responses history 中的 `reasoning` / summary 类中间项会被丢弃，`function_call` / `function_call_output` 会转成 chat tool_calls / tool messages；`namespace` 仅作为 Codex/MCP 工具容器做 function flatten，不代表 DeepSeek 上游原生支持 Responses namespace。
- **背景**：CN 上游/中转站通常只支持 OpenAI chat/completions 协议和原始模型名（如 `deepseek-v4-pro`），不支持自定义 `deepseek-v4-pro-nothink`、`/v1/messages` 或 `/v1/responses`。Codex 使用 `/v1/responses`，Claude Code 使用 `/v1/messages`，因此 DeepSeek adapter 需要在网关侧桥接协议并继续注入 `thinking` 参数。
- **冲突风险**：中（`relay/channel/deepseek/adaptor.go` 是上游 provider adapter 文件，后续 upstream 若实现 DeepSeek 原生 Responses / Anthropic Messages 兼容时可能冲突；新增 bridge 文件独立，容易删除或替换）
- **Merge 策略**：若 upstream 后续补齐 DeepSeek `/v1/responses` / `/v1/messages` 到 OpenAI chat 的等价转换，优先采用 upstream；否则保留本 bridge，特别是 `FinalRequestRelayFormat=openai` 与保留入口 `RelayFormat` 的双轨语义。
  - 用途：图片渠道六阶段测试（探针 → 冒烟 → API 兼容 → 输出验证 → 内容质量 Phase A/B → 安全抽样），单命令产出结构化 JSON + markdown 报告
- **新增测试**：`scripts/channel-benchmark/py/tests/test_image_conformance_json.py`、`scripts/channel-benchmark/py/tests/test_phase2_phase3.py`
- **修改文件**：`scripts/channel-benchmark/py/pyproject.toml`（注册 `fy-image-conformance` CLI）
- **冲突风险**：极低（benchmark 子树独立新增目录）

### B-15.2 [benchmark/image] 图片金丝雀真实性检测（5A/5B）
- **新增目录**：`scripts/channel-benchmark/py/fy_image_canary/`
  - `cli.py` / `config.py` / `client.py` / `verdict.py` / `runner.py` / `runner_5a.py` / `calibrate.py` / `report.py`
  - `comparators/clip.py` / `histogram.py` / `vlm_judge.py`（5A 三维对比器）
  - `probes/fingerprint.py` / `cross_channel.py` / `capability.py`（5B 探针）
  - 命令：`fy-image-canary`
  - 用途：检测图片渠道是否被静默替换为劣质模型。5A = vendor 直连对比（CLIP + 颜色直方图 + VLM），5B = 无 key 指纹/跨渠道/能力边界探针
- **新增测试**：`scripts/channel-benchmark/py/tests_image_canary/test_verdict_fix.py`、`test_runner.py`、`test_comparators.py`、`test_fingerprint.py`、`test_config.py`
- **修改文件**：`scripts/channel-benchmark/py/pyproject.toml`（注册 `fy-image-canary` CLI + `[image-canary]` extras）
- **冲突风险**：极低（benchmark 子树独立新增目录）

### B-15.3 [benchmark] 统一评分器（文本 + 图片）
- **新增目录**：`scripts/channel-benchmark/py/fy_score/`
  - `cli.py` / `scorer.py` / `loader.py` / `report.py`
  - 命令：`fy-score`
  - 用途：汇总 loadtest / quality / canary / conformance / integrity 各工具输出，按五维度（可用性/性能/质量/真实性/合规性）加权评分，产出 scorecard.json + scorecard.md。文本和图片使用不同权重体系
- **修改文件**：`scripts/channel-benchmark/py/pyproject.toml`（注册 `fy-score` CLI）
- **冲突风险**：极低（benchmark 子树独立新增目录）

### B-16 [aws/bedrock] Claude `anthropic-beta` 兼容过滤
- **修改文件**：
  - `relay/channel/aws/dto.go`（Bedrock AK/SK 路径把 `anthropic-beta` header 转成 body `anthropic_beta` 前，先按 Bedrock 支持列表映射/过滤；无兼容 token 时不发送该字段；同时丢弃客户端 body 自带的 `anthropic_beta` / `output_config`）
  - `relay/channel/aws/relay-aws.go`（AWS Claude pass-through body 路径同样删除 `anthropic_beta` / `output_config`，避免绕过 `formatRequest` 的兜底过滤）
  - `relay/channel/aws/relay_aws_test.go`（覆盖支持 token 保留、`advanced-tool-use-2025-11-20` 映射成 `tool-search-tool-2025-10-19`、不支持 token 被删除、body 自带 beta 删除、pass-through 删除 Bedrock 不兼容字段）
- **背景**：HK 生产 Bedrock 流式调用报 `ValidationException: invalid beta flag`，后续日志还出现 `output_config.format: Extra inputs are not permitted`。直连 Anthropic 支持的 beta/header/body 扩展集合大于 AWS Bedrock 支持集合，原实现把 `anthropic-beta` 原样拆分写入 Bedrock body 的 `anthropic_beta`，且 pass-through 会把原始 body 的不兼容字段直接发给 Bedrock。
- **行为**：后端默认按 AWS Bedrock Claude Messages 当前支持的 beta token 做白名单，并保留已有前端 “AWS Bedrock Claude 兼容模板” 的核心映射语义；渠道级 header override 仍先执行，随后 Bedrock adaptor 做最后兜底过滤。客户端 body 自带的 `anthropic_beta` 不被信任，统一由过滤后的 header 重建；`output_config` 暂按 Bedrock 不兼容字段在 AWS 边界删除。
- **冲突风险**：低（AWS adaptor 局部新增 helper；若 upstream 后续实现 Bedrock beta 白名单或官方支持 `output_config`，合并时对齐官方支持列表即可）

### B-17 [aws/bedrock] 不支持的 tool type 过滤
- **新增文件**：
  - `relay/channel/aws/bedrock_tools_filter.go`（Bedrock 支持的 tool type 白名单 + pass-through / struct 两条路径的过滤函数）
  - `relay/channel/aws/bedrock_tools_filter_test.go`（覆盖：保留支持类型、删除不支持类型、全部不支持时删除 tools 字段、无 type 字段保留）
- **修改文件**：
  - `relay/channel/aws/relay-aws.go`（`sanitizeBedrockClaudeRawFields` +1 行调用 `filterBedrockToolsRaw`）
  - `relay/channel/aws/dto.go`（`sanitizeBedrockClaudeRawFieldsFromStruct` +1 行调用 `filterBedrockToolsFromStruct`）
- **背景**：HK 生产 momo 客户流量出现 400 `ValidationException: tools.N: Input tag 'web_search_20250305' / 'advisor_20260301' found using 'type' does not match any of the expected tags`。Bedrock Claude Messages 仅接受有限的 tool type 集合，Anthropic 直连支持的扩展 tool type 在 Bedrock 侧会被拒绝。

- **行为**：在发送请求到 Bedrock 前，按白名单过滤 tools 数组中不支持的 type，静默丢弃不兼容工具而非返回 400 给客户端。
- **冲突风险**：极低（独立新文件 + 两处各 +1 行；与 B-16 同一函数但不同行，合并时仅需保留两行调用）

### B-27 [ops/report] 毛利报表脚本与 agent skill
- **新增/修改文件**：
  - `scripts/ops/gross_profit_report.py`（CN/HK 多环境毛利 CSV 报表；本地 RDS 直连失败时自动 SSH 到生产机本地 MySQL 聚合查询；`detail.csv` 按运营表格格式输出：`日期 / 环境 / 用户 / 渠道ID / 渠道 / 模型 / 请求数 / 输入Tokens / 输出Tokens / 折扣倍率 / 收入(USD) / 成本(USD) / 毛利(USD) / 毛利率(%)`）
  - `scripts/ops/test_gross_profit_report.py`（锁定明细 CSV 表头、列顺序、日志有效折扣倍率、缺失倍率不伪装为 1）
  - `.agents/skills/gross-profit-report/SKILL.md`（Codex 项目内技能；全局副本需同步到 `~/.codex/skills/gross-profit-report` 和 `~/.claude/skills/gross-profit-report`）
- **行为**：`折扣倍率` 来自日志 `other.group_ratio` 的聚合有效倍率 `SUM(quota) / SUM(quota / group_ratio)`；缺失/非法倍率在 CSV 中显示 `缺失` 并写入 `warnings.csv`，不能临时写死为 `1`。`channel_costs.yaml` 的 `cost_factor` 只用于成本修正，不作为折扣倍率列展示。日期格式为运营表格使用的 `YYYY/M/D`。
- **冲突风险**：低（独立运维脚本和 agent 文档，不改 upstream 业务代码）

### B-18 [aws/bedrock] deprecated temperature 参数过滤
- **新增文件**：
  - `relay/channel/aws/bedrock_temperature_filter.go`（模型黑名单 + struct / pass-through 两条路径的 temperature 剥离函数）
  - `relay/channel/aws/bedrock_temperature_filter_test.go`（覆盖：黑名单模型剥离、非黑名单模型保留、raw map 路径）
- **修改文件**：
  - `relay/channel/aws/relay-aws.go`（`doAwsClientRequest` +1 行 `stripBedrockDeprecatedTemperature`；`buildAwsRequestBody` +1 行 `stripBedrockDeprecatedTemperatureRaw`）
- **背景**：HK 生产 momo 客户流量出现 400 `ValidationException: 'temperature' is deprecated for this model`（claude-opus-4-7 via Bedrock channel #27；2026-06-25 海外测试 `claude-opus-4-8` 也返回同类错误）。Bedrock 对部分新模型不再接受 temperature 参数。
- **行为**：在发送请求到 Bedrock 前，根据模型黑名单剥离 temperature 字段，静默丢弃而非返回 400 给客户端。
- **冲突风险**：极低（独立新文件 + relay-aws.go 两处各 +1 行；与 B-17 同一区域但不同行）

### B-19 [monitoring] Prometheus metrics overlay
- **新增文件**：
  - `middleware/prometheus_overlay.go`（Prometheus histogram/counter 定义 + Gin middleware + TTFT ResponseWriter wrapper）
  - `router/prometheus_overlay.go`（`/metrics` 端点注册 + middleware 挂载，受 `PROMETHEUS_METRICS=1` 环境变量控制）
  - `docs/prometheus-monitoring.md`（部署文档：Prometheus + Grafana + AlertManager 接入指南）
  - `pkg/prommetrics/relay_cache.go`（缓存命中率 + 亲和性命中率 Prometheus 指标定义 + Collector）
- **修改文件**：`router/main.go`（1 行：`SetPrometheusRouter(router)` 调用）、`middleware/distributor.go`（亲和性 lookup 指标记录）、`service/text_quota.go`（缓存 token 指标记录）、`service/channel_affinity.go`（注册 stats provider）
- **指标**：`fy_relay_requests_total`, `fy_relay_errors_total`, `fy_relay_duration_seconds`, `fy_relay_ttft_seconds`, `fy_image_duration_seconds`, `fy_relay_retries_total`, `fy_relay_prompt_tokens_total`, `fy_relay_cached_tokens_total`, `fy_relay_cache_creation_tokens_total`, `fy_affinity_lookups_total`, `fy_affinity_active_entries`
- **冲突风险**：极低（新增文件 + main.go 1 行注册；upstream 不太可能在同一位置加同名函数）
- **Merge 策略**：若 upstream 未来自己加 Prometheus 支持，评估是否迁移到 upstream 实现

### B-18 [tencent/aiart] GPT 图片生成同步伪装
- **新增文件**：
  - `relay/channel/tencent/aiart_image.go`（Tencent AIArt 图片请求转换、TC3 签名、提交/轮询、OpenAI 图片响应转换）
  - `relay/channel/tencent/aiart_image_test.go`（AIArt host 分支、请求转换、签名 scope、响应转换、提交+查询流程测试、ResultImages 多形态兼容、错误码 string 兼容、prompt 引号与安全词回归）
- **修改文件**：`relay/channel/tencent/adaptor.go`（`ConvertImageRequest` / `DoRequest` / `DoResponse` 三处薄分支，均带 `// Fy-api overlay:` 注释）
- **行为**：后台仍使用现有 `腾讯混元 / Tencent` 渠道和 `AppId|SecretId|SecretKey` 密钥格式；当 `RelayModeImagesGenerations` 且渠道 `base_url` host 为 `aiart.tencentcloudapi.com` 时，后端把 OpenAI `/v1/images/generations` 请求转成腾讯 AIArt `SubmitContentToImageGPTJob`，每 5 秒轮询 `DescribeContentToImageGPTJob`，最长同步等待 10 分钟，然后返回 OpenAI 兼容 image response。
- **兼容修复**：
  1. AIArt `Error.Code` 与普通 Tencent chat `Error.Code` 均兼容腾讯返回 string / number 两种形态，避免 `"0"` 触发反序列化失败。
  2. AIArt 查询结果里的 `ResultImage` / `ResultImages` / `ImageUrls` / `Images` 兼容 string、array（含 object array）和 object，object 中优先抽取 `Url` / `ImageUrl` / `B64Json` 等常见字段。
  3. AIArt 请求转换阶段对明确的 sexual / violence prompt 做前置 `moderation_blocked` 兜底，并在图片 handler 中按 OpenAI 图片错误语义返回 400，避免腾讯侧混合词审核漏拦时直接出图。
  4. OpenAI 图片兼容响应默认返回 `b64_json`；只有客户端显式传 `response_format: "url"` 时才透出腾讯 COS URL，保持早期测试通过时的默认行为。
- **配置**：后台不改 UI。渠道类型选 Tencent，Base URL 填 `https://aiart.tencentcloudapi.com`，模型填 `gpt-image-2`，密钥填 `AppId|SecretId|SecretKey`。不要为该渠道启用 pass-through body；图片价格/倍率仍需按模型单独配置。
- **冲突风险**：低（核心逻辑在新增文件；`adaptor.go` 仅三处小分支）
- **Merge 策略**：upstream 若改 Tencent 文本 adaptor，保留 AIArt 分支即可；若 upstream 后续原生支持 AIArt 或图片异步任务，可优先迁移到 upstream 实现，但保留 `AppId|SecretId|SecretKey` 后台兼容配置。

> Note: tnbiz integration code comments still reference the historical B-12..B-18 identifiers from the original overlay branch. The list below is renumbered here to avoid colliding with later main-branch overlay entries.

### B-19 [tnbiz] TraceNex Partner 集成内部 API 路由 + HMAC 鉴权
- **新增文件**：
  - `router/api-internal-router.go`（`/api/internal/*` 独立路由组，**不**继承 `/api` 全局 `GlobalAPIRateLimit`；改用 per-kid quota 占位）
  - `middleware/internal_auth.go` + `middleware/internal_auth_test.go`（HMAC-SHA256 timestamp ±5min + nonce SETNX 24h（go-redis/v8 ctx-first） + body sha256 + endpoint allowlist 精确匹配）
- **修改文件**：`router/main.go`（+1 行 `SetInternalRouter(router)`，紧接 `SetVideoRouter` 之后）
- **冲突风险**：低（router/main.go 1 行 + 独立路由文件）
- **Merge 策略**：upstream 若改 `SetRouter` 签名同步对齐，独立 file 零冲突
- **Feature flag**：`overlay.internal_api_enabled`（OVERLAY_INTERNAL_API），prod 默认 false

### B-20 [tnbiz] 内部 controllers + ChannelLogSetting upsert
- **新增文件**：
  - `controller/tnbiz_internal/health.go`（GET /api/internal/health 自检 + envelope helper）
  - `controller/tnbiz_internal/token.go`（POST /api/internal/token/create — partner 永远不可见 sk-key 明文，仅返回 token_id + masked_key + 5 分钟一次性 delivery_handle）
  - `controller/tnbiz_internal/user.go`（POST topup / quota/adjust / refund / erase，PUT group，GET quota）
  - `controller/tnbiz_internal/settings.go`（POST group_ratio_override/upsert + channel_log_settings/upsert）
  - `controller/tnbiz_internal/context.go`（context helper）
  - `controller/tnbiz_internal/health_test.go`
  - `model/channel_log_settings.go`（schema-only，B-13 partner 维度 channel log 配置）
- **冲突风险**：极低（独立子包 + 新增 model 表）
- **Feature flag**：与 B-12 共用 `overlay.internal_api_enabled`

### B-21 [tnbiz] OVERLAY feature flag 框架（biz_setting + 5-15s polling）
- **新增文件**：
  - `setting/overlay_flag/flag.go`（5 个 atomic-cached flag + ctx-first poller；优先读 `common.OptionMap`，env 兜底）
  - `setting/overlay_flag/flag_test.go`
- **修改文件**：`main.go`（`+overlayCtx := context.Background()` + `overlay_flag.StartPoller(overlayCtx)`，紧接 Redis 初始化之后）
- **冲突风险**：低（main.go 集中 patch + 新建独立子包）
- **Merge 策略**：5 个 flag key 写入 `OptionMap`，prod 默认全 false / shadow，灰度时按 PR 单独切换

### B-22 [tnbiz] User per-customer GroupRatioOverride（hot path 6 调用站 / 4 文件）
- **新增文件**：
  - `model/group_ratio_override.go`（`group_ratio_override` 表 + Upsert + LookupUserOverride DAO）
  - `setting/ratio_setting/effective_group_ratio.go`（`ApplyOverride(override, fallback) float64` —— hot path 唯一入口，atomic flag check + 1 float compare）
  - `setting/ratio_setting/effective_group_ratio_test.go`
  - `relay/common/override_lookup.go`（callback registry 避免 relay/common → model 的潜在循环依赖）
- **修改文件**（每处加 `// Fy-api overlay: B-15 ...` 注释）：
  - `relay/common/relay_info.go`（struct 末尾 +`UserGroupRatioOverride float64`；`GenRelayInfo` best-effort 从 context 拷入；context 缺失时回库一次）
  - `constant/context_key.go`（+`ContextKeyUserGroupRatioOverride`）
  - `service/quota.go`（行 110 / 115 / 121-124 三处 `GetGroupRatio` / `GetGroupGroupRatio` 后串 `ApplyOverride`）
  - `relay/helper/price.go`（行 53-62 user-group / normal-group 两个分支都串 `ApplyOverride`）
  - `service/task_billing.go`（行 276-277 task 路径用 `model.LookupUserOverride(task.UserId, group)` 因为没有 RelayInfo）
  - `service/group.go`（新增 `GetUserGroupRatioWithOverride(userId, userGroup, group)` 给 cold path 调用方）
  - `main.go`（+`relaycommon.SetOverrideLookup(model.LookupUserOverride)` 注入 callback）
- **冲突风险**：HIGH（ratio 系统是上游持续演进区；4 文件都打上 overlay 注释，merge 时 grep `B-15` 即可定位）
- **Feature flag**：`overlay.group_ratio_override`（prod 默认 false，逐 partner 灰度）

### B-23 [tnbiz] consume_log_outbox + RecordConsumeLog 同事务写 outbox
- **新增文件**：
  - `model/consume_log_outbox.go`（落 LOG_DB；status: pending/in_flight/published/failed/dead_letter；`(data_region, status)` + `(status, locked_until)` 双索引；`InsertOutboxInTx` / `LeaseOutboxBatch` / `MarkOutboxPublished` / `MarkOutboxFailed` 含 retry_count++ ≤ 10 → DLQ）
  - `model/log_outbox.go`（`recordConsumeLogWithOutbox` —— flag off 走原 `LOG_DB.Create(log)` 单语句；flag on 用 `LOG_DB.Transaction` 同时 Create(log)+Create(outbox)，任一失败整批回滚）
  - `model/log_outbox_integration_test.go`（in-memory sqlite 验证 flag on / flag off 两条路径）
- **修改文件**：
  - `model/log.go::RecordConsumeLog`（`LOG_DB.Create(log).Error` → `recordConsumeLogWithOutbox(...)`；函数顶部加注释规约「**仅** LogTypeConsume 走 outbox；行 87 / 112 / 139 / 183 / 292 等 5 个非 consume LOG_DB.Create 调用站一律 NOT outbox-eligible」；LogQuotaData fire-and-forget goroutine 仍在 TX 之后异步触发）
  - `model/main.go::migrateLOGDB`（+`LOG_DB.AutoMigrate(&ConsumeLogOutbox{})`）
- **冲突风险**：HIGH（log.go 是上游高活跃区；overlay 集中在 helper 函数，注释打满）
- **Feature flag**：`overlay.outbox_tx_enabled` + `overlay.outbox_mode`（off / shadow / enabled）

### B-23.1 [channel] channels.group 扩容到 varchar(255)
- **修改文件**：
  - `model/channel.go`（`Channel.Group` 从 `varchar(64)` 调整到 `varchar(255)`）
  - `model/main.go`（新增 `migrateChannelGroupToVarchar255()`，启动迁移时显式扩容已有 `channels.group` 列）
- **背景**：渠道编辑页支持多选分组，前端保存时会把分组列表拼成逗号分隔字符串写入 `channels.group`。原 `varchar(64)` 在分组较多或分组名较长时会触发 MySQL `Error 1406 (22001): Data too long for column 'group'`
- **行为**：新部署在 MySQL/PostgreSQL 上启动时会自动把 `channels.group` 扩到 `varchar(255)`；SQLite 无需额外迁移
- **冲突风险**：低（`Channel` 模型字段宽度调整 + 独立迁移函数）
- **Merge 策略**：若 upstream 后续也调整 `channels.group` 类型，优先采用上游实现；否则保留本迁移，避免 HK/CN 老库 schema 偏小

### B-24 [tnbiz] Outbox publisher (Aliyun MNS, shadow + enabled)
- **新增文件**：
  - `service/outbox/runner.go`（Publisher interface + NoopPublisher + Runner with batch/lease/interval；shadow 模式下 publisher inject NoopPublisher，仅 simulate；MNS SDK 接入留 Phase 2A）
  - `service/outbox/runner_test.go`
- **修改文件**：`main.go`（+`outbox.NewRunner(region, topic, nil).Start(overlayCtx)`，紧接 flag poller / OverrideLookup 注入之后）
- **冲突风险**：极低（独立子包 + main.go 一段 patch）
- **Feature flag**：`overlay.outbox_mode`（off / shadow / enabled），region 由 `DATA_REGION` 环境变量注入（cn / hk），强制 region 隔离 invariant

### B-25 [tnbiz] internal_idempotency + internal_api_key (HMAC keystore + idempotency 表)
- **新增文件**：
  - `model/internal_api_key.go` + `model/internal_api_key_test.go`（HMAC keystore：key_id / secret_cipher（AES-GCM with `common.CryptoSecret` 派生 KEK）/ region / status / allowed_endpoints JSON / created_at / rotated_at；明文 secret 永不入库）
  - `model/internal_idempotency.go`（`internal_idempotency` 表，UNIQUE(auth_kid, idempotency_key, endpoint)；`Lookup` / `Save` / `CleanupExpiredIdempotency`；7 天 TTL leader-only cron 兜底）
  - `middleware/internal_idempotency.go`（middleware：命中 (auth_kid, idem_key, endpoint) 三元组直接 replay 200，body hash 不一致则 409）
- **修改文件**：`model/main.go`（+`InternalAPIKey{}` / `InternalIdempotencyRecord{}` / `GroupRatioOverride{}` / `ChannelLogSetting{}` 进 AutoMigrate）
- **冲突风险**：极低（全独立 model + middleware）
- **Feature flag**：`overlay.hmac_keystore_enabled`（B-12 InternalAuth middleware 双 flag 校验：InternalAPI ON + HMACKeystore ON 才放行；任一 OFF 即 503）

### B-26 [aws/bedrock] cache_control.scope 剥离 + 空 text content block 过滤
- **新增文件**：
  - `relay/channel/aws/bedrock_content_filter.go`（cache_control.scope 剥离 + 空 text block 过滤，struct / pass-through 双路径）
  - `relay/channel/aws/bedrock_content_filter_test.go`（7 个 table-driven 测试覆盖：scope 剥离、保留无 scope 的 cache_control、string content 不 panic、空 text 过滤、非 text 保留、raw map 路径）
- **修改文件**：
  - `relay/channel/aws/dto.go`（`sanitizeBedrockClaudeRawFieldsFromStruct` +2 行调用）
  - `relay/channel/aws/relay-aws.go`（`sanitizeBedrockClaudeRawFields` +2 行调用）
- **背景**：HK 生产 momo 客户流量（channel #27 AWS Bedrock）5/21 出现 18 次 400 `cache_control.ephemeral.scope: Extra inputs are not permitted` 和 5 次 `text content blocks must be non-empty`。Bedrock schema 校验比 Anthropic 原生 API 更严格：(1) 不接受 `cache_control` 内的 `scope` 字段；(2) 不允许 `type:"text"` 且 `text:""` 的空块。
- **行为**：请求发往 Bedrock 前，静默移除 system/messages 中所有 `cache_control.scope`，并过滤掉空 text content block。不改变请求语义——`cache_control.type` 保留，非空 text block 保留。
- **冲突风险**：极低（独立新文件 + 两处各 +2 行，与 B-17 同一函数但不同行）

### B-28 [video/pipeline] Seedance 1080p 同模型降分辨率 + 火山增强 pipeline
- **新增文件**：
  - `service/video_pipeline_strategy.go`
  - `service/video_pipeline_analysis.go`
  - `service/video_pipeline_policies.go`
  - `service/video_pipeline_mappers.go`
  - `service/video_pipeline_config.go`
  - `service/seedance_enhance_pipeline.go`
  - `service/volcengine_mediakit_client.go`
  - `model/task_seedance_enhance.go`
  - `config/video-pipeline.example.yaml`
- **修改文件**：
  - `main.go`（启动时读取 `config/video-pipeline.yaml` 或 `VIDEO_PIPELINE_CONFIG_PATH` 并用 fsnotify 监听热加载；配置缺失时默认不启用任何 pipeline）
  - `model/task.go`（`TaskPrivateData` 新增 `SeedanceEnhance` 私有 pipeline snapshot）
  - `controller/relay.go`（任务插入前写入 pipeline snapshot）
  - `relay/common/relay_info.go`（`TaskRelayInfo` 暂存内部 pipeline plan，避免跨 submit 阶段只依赖 gin context）
  - `relay/channel/task/doubao/adaptor.go`（提交前调用 strategy helper 改写 generation payload；按用户请求的 1080p 产品档计费；查询响应隐藏内部 pipeline metadata）
  - `service/task_polling.go`（普通轮询路径前加 `AdvanceVideoPipelineIfNeeded` 薄 hook）
- **新增测试**：
  - `service/video_pipeline_strategy_test.go`
  - `relay/channel/task/doubao/adaptor_pipeline_test.go`
  - `model/task_seedance_enhance_test.go`
- **行为**：pipeline 策略由 `config/video-pipeline.yaml` 驱动，配置结构为 `version/defaults/strategies/lifecycle.match/lifecycle.rollout`；保存后通过 fsnotify 热加载。配置文件缺失、解析失败、`defaults.enabled=false`、单策略 `enabled=false`、未命中 match 或灰度拒绝时，统一回退原始直连生成路径。用户仍按请求模型/1080p 产品价计费；内部仅将 Seedance 2.0 1080p 请求改为同一 Seedance 2.0 模型的 720p generation + 火山 MediaKit 1080p 标准增强，不再按 storyboard、静态 prompt、多参考图等请求特征切换到 1.5-pro 或其它 generation 模型。内部 generation/enhance task id、策略命中、字段映射/丢弃、供应商成本记录在 `PrivateData.SeedanceEnhance`，不直接返回用户。
- **成本估算扩展（2026-06-15）**：
  - 新增 `service/media_provider_cost.go` / `service/video_pipeline_cost.go`，按火山官方价格实时估算 Seedance 2.0 generation 与 AI MediaKit enhance-video provider cost；该成本仅用于实时观测和灰度评估，不作为最终财务毛利。
  - Seedance 2.0 优先使用 provider `usage.completion_tokens`，否则按 `(输入视频时长 + 输出视频时长) × 输出宽 × 输出高 × fps / 1024` 估算；价格按 `doubao-seedance-2.0` 720p/1080p、`doubao-seedance-2.0-fast`、`doubao-seedance-2.0-mini` 官网元/百万 token 档位。
  - MediaKit 当前 pipeline 使用 `tool_version=standard`、`resolution=1080p`、`scene=aigc`；成本按 `输出时长分钟 × 计费换算系数 × 0.75 元/分钟`，标准版/专业版共用 0.75 基价，专业版仅通过更高系数放大。不得把“画质增强（大模型）”的 2.5 元/分钟基价套到 standard/professional。
  - 新增 `cost_price_version`、`rmb_per_usd`、generation/enhance RMB/quota、billable tokens、billing coefficient、gross profit/margin 等字段到 `model.SeedanceEnhancePipeline`，并在现有 JSON 字段内保存 `generation_cost_detail` / `enhance_cost_detail`（公式 key/version/text、变量、系数、usage source、`cost_quota`、`cost_rmb`），不新增成本表；task billing `other` 同步输出内部 `*_estimate_*` 和 `provider_cost_estimate_details` 字段，供数据库同步后做报表。
  - CN/HK 两个平台展示币种可能不同，内部 provider cost 以 `cost_quota` 为主口径，RMB 仅作为供应商价格来源和对账辅助；换算公式固定记录为 `cost_quota = round(cost_rmb / rmb_per_usd * quota_per_unit)`。
  - 用户侧日志列表和 `/api/log/self/export?format=json` 统一脱敏 provider cost、公式、变量、系数、毛利等内部字段；管理员日志和 admin export 保留完整成本明细。用户实际扣费仍由原任务 billing pipeline 决定。
  - MediaKit 闲时/低优先级不按本地时间推断；当前把 provider 返回的 `task_type` 记录为 `enhance_provider_task_type`，任务等级仍默认 `normal`，未来若官方为 `enhance-video` 提供显式低优先级字段，再按提交参数或返回字段扩展。
- **参考文档**：`docs/seedance-mediakit-cost-plan.md`
- **冲突风险**：中（`service/task_polling.go` 和 `relay/channel/task/doubao/adaptor.go` 是 upstream 活跃区域）
- **Merge 策略**：主体逻辑保留在新增 service/model 文件中；upstream 合并时只重放 controller/adaptor/polling 的极薄 `// Fy-api overlay:` hook。不得把策略判断写进 controller 或把火山增强状态机写进 Doubao adaptor。

### B-29 [relay/video] Seedance 2.0 真人参考图可信素材准备
- **新增文件**：
  - `service/seedance_asset_client.go` / `service/seedance_asset_client_test.go`（火山 Ark Asset Service `CreateAsset` / `GetAsset` / `DeleteAsset` 客户端，状态归一为 `creating/processing/active/failed`）
  - `relay/channel/task/doubao/seedance_asset_prepare.go` / `_test.go`（识别 Seedance 2.0 图片参考、保存请求快照、`asset://...` 改写、prepared submit）
  - `docs/seedance-real-person-video-support-plan.md`
  - `scripts/ops/seedance_gateway_vs_volcengine.py`
  - `scripts/ops/seedance_e2e_matrix.py`（Seedance 2.0 `/v1/videos` 端到端矩阵脚本，默认覆盖命中 pipeline 与直连不命中两类 1080p 请求下的横屏无参考图、竖屏无参考图、单图、多图、故事板/metadata content 请求；下载视频后用 `ffprobe` 校验分辨率/时长，并可通过测试环境 SSH 查询 `tasks.private_data` 断言 pipeline 状态、真实上游 task id 和素材清理状态）
- **修改文件**：
  - `dto/channel_settings.go`（`ChannelOtherSettings` 新增 `seedance_asset_*` 渠道级配置字段）
  - `model/task.go`（`TaskPrivateData.SeedanceAssetPrepare` 保存素材准备状态、原始请求快照和最终态素材清理状态）
  - `relay/relay_task.go`（`// Fy-api overlay:`：Seedance 2.0 + HTTP 图片参考命中时，提交阶段返回本地 task，不立即投递上游）
  - `service/task_polling.go`（`// Fy-api overlay:`：轮询阶段先创建/查询 Ark asset，active 后用 `asset://<provider_asset_id>` 二次提交 Seedance）
  - `controller/relay.go`（`// Fy-api overlay:`：把 submit result 中的 staged private data 写入 task）
  - `web/classic/src/components/table/channels/modals/EditChannelModal.jsx`（`// Fy-api overlay:`：type 45 渠道编辑弹窗显示 Seedance 真人素材库配置，写入 `settings` JSON）
  - `web/classic/src/i18n/locales/{zh-CN,zh-TW,zh,en,fr,ja,ru,vi}.json`（新增 Seedance 真人素材库配置文案）
  - `go.mod` / `go.sum`（新增 `github.com/volcengine/volcengine-go-sdk`）
- **配置**：不改 `channels` 表结构。现有 type 45 渠道继续用 `channel.key` 调主视频接口；Ark Asset Service 凭证存入渠道 `settings` JSON：`seedance_asset_access_key`、`seedance_asset_secret_key`、`seedance_asset_group_id`，可选 `seedance_asset_project_name` / `seedance_asset_region` / `seedance_asset_endpoint` / `seedance_asset_timeout_seconds`
- **行为**：客户仍通过 `/v1/videos` 传普通真人图片 URL；fy-api 内部异步准备可信素材，审核通过后改写成 `asset://...` 再提交 Seedance。素材创建/审核/二次提交失败时任务失败并复用现有异步任务退款逻辑。视频任务进入最终态 success/failure 后，轮询器 best-effort 调用 `DeleteAsset` 清理本任务创建的 Ark asset；删除失败只写入 `cleanup_status=failed` 和日志，不影响用户任务结果。
- **冲突风险**：中（`relay/relay_task.go` 和 `service/task_polling.go` 是任务提交/轮询核心；保留 overlay 注释和单测，upstream 同步时不能丢掉 staged task 的 private data）

---

### B-30 [image-edit] /v1/images/edits 超时时间 3x

- **问题**：gpt-image-2 `/v1/images/edits` 图片编辑耗时波动大（21s ~ 2348s，平均 ~296s），全局 `RELAY_TIMEOUT`（HK 环境 = 600s）经常不够用，导致 `context deadline exceeded` 后重试多个渠道全部失败
- **修改文件**：
  - `relay/common/relay_info.go`：`RelayInfo` 新增 `TimeoutMultiplier int` 字段
  - `service/http_client.go`：新增 `CloneHttpClientWithTimeout` 函数，浅拷贝 client 并乘以倍率
  - `relay/channel/api_request.go`：`doRequest` 中当 `TimeoutMultiplier > 0` 且非流式时，用倍率后的 client 发请求；同时通过 `net/http/httptrace` 记录 `WroteRequest` 和 `GotFirstResponseByte` 两个时间点，用于超时排查
  - `relay/image_handler.go`：`ImageHelper` 中判断路径以 `/edits` 结尾时设 `TimeoutMultiplier = 3`
- **行为**：仅对 `/v1/images/edits` 生效，流式请求不受影响。不修改全局 client，只浅拷贝一份临时 client。同时通过 httptrace 记录请求写入耗时和首字节响应耗时，用于超时排查
- **冲突风险**：低（3 个文件新增少量代码，均带 `// Fy-api overlay:` 注释）

### B-31 [gemini] Image edit URL download support for image-preview models

- **问题**：OpenAI `/v1/images/edits` 接口支持在 `image`/`images`/`mask` 字段传 HTTP URL，但 Gemini image-preview 转换路径里 `geminiImagePartFromEncodedString` 只处理 base64 data URI，遇到 URL 会报 decode 错误
- **修改文件**：
  - `relay/channel/gemini/adaptor.go`：`geminiImagePartFromEncodedString` 新增 URL 前缀检测（`http://`/`https://`），匹配时调用新增的 `downloadGeminiImageParts` 下载图片并转为 base64 `InlineData`
- **新增测试**：`relay/channel/gemini/relay_gemini_usage_test.go`：3 个测试（单 URL、URL 数组、404 URL 拒绝）
- **行为**：仅对 Gemini image-preview 模型的 `/v1/images/edits` 路径生效，不影响 Imagen 或 chat 路线
- **冲突风险**：极低（adaptor.go 内 `geminiImagePartFromEncodedString` 新增 4 行分支 + 独立新函数 `downloadGeminiImageParts`）

---

## 前端定制

> ⚠️ **路径前缀提示（v1.0 后）**：所有前端 overlay 文件物理路径都在 `web/classic/` 下面。`web/default/` 是上游 v1.0 引入的全新前端，TraceNex 不在那里维护任何 overlay，runtime theme 也不允许切到 default（见 `setting/system_setting/theme.go` + `controller/option.go` 的 `// Fy-api overlay:` 双锁）。

### F-1 [brand] 浏览器 tab + icon
- **文件**：`web/classic/index.html`
- **修改**：`<title>TraceNex</title>` + `<link rel="icon" href="/new_logo.png?v=2" />`
- **冲突风险**：中（上游会改 meta description）
- **Merge 策略**：title 和 icon 两处坚持用 TraceNex；meta description 可接受 upstream

### F-2 [brand] Logo 和 favicon
- **新增**：`web/classic/public/new_logo.png` (3.4 MB)
- **替换**：`web/classic/public/favicon.ico`
- **冲突风险**：低（上游偶尔更新 logo.png，我们用 new_logo.png 独立）
- **注意**：v1.0 merge 时 git 的 directory-rename 启发式会把 public 资源建议到 `web/default/public/`，**必须手动改到 `web/classic/public/`**

### F-3 [i18n] 品牌词替换
- **修改文件**：`web/classic/src/i18n/locales/{zh-CN,zh-TW,zh,en,fr,ja,ru,vi}.json`（v1.0 合并后多了一个 `zh.json`）
- **变化**：所有 value 中 `New API` → `TraceNex`，`TraceNex` → `TraceNex`（历史遗留）
- **冲突风险**：高（上游每月增改几十个翻译 key）
- **Merge 策略**：
  ```bash
  # 每次 merge 上游的 locales 之后：
  for lang in zh-CN zh-TW zh en fr ja ru vi; do
    f="web/classic/src/i18n/locales/${lang}.json"
    jq '(.translation |= with_entries(.value |= gsub("New API"; "TraceNex")))' "$f" > "/tmp/rebrand-${lang}.json"
    cp "/tmp/rebrand-${lang}.json" "$f"
  done
  ```
- **建议长期改造**：上游增加 ESLint 规则禁止未来翻译 value 出现 "New API"

### F-4 [docs] 内嵌产品文档页
- **新增文件**：
  - `web/classic/src/pages/FyApiDocs/index.jsx`（重命名自 TraceNexDocs；当前 `/docs` 入口跳转到统一静态产品文档页）
  - `web/classic/src/components/common/NewMarkdownRender/NewMarkdownRender.jsx`
  - `web/classic/public/product-docs/api-reference.html`（合并页：顶部 3 个路由式标签 `API 接口文档` / `模型接入指南` / `平台功能指南`）
  - `web/classic/public/product-docs/index.html`（兼容入口，跳转到合并页）
  - `web/classic/public/product-docs/model-integration-guide.html`（旧链接兼容，跳转到合并页模型接入标签）
  - `web/classic/public/product-docs/TraceNex.md`（旧 Markdown 素材保留，不再作为产品文档入口）
  - `web/classic/public/product-docs/images/image1.png` ~ `image18.png`
- **修改文件**：`web/classic/src/App.jsx`
  - 第 ~59 行：`const FyApiDocs = lazy(() => import('./pages/FyApiDocs'));`
  - 第 ~365 行：`<Route path='/docs' element={<Suspense>...</Suspense>} />`
- **修改文件**：`web/classic/src/components/layout/PageLayout.jsx`（`/docs` 路径使用 early return 跳过 Header/Footer 包装，作为独立页面渲染）
- **冲突风险**：低（App.jsx 两处小改，Suspense pattern 和 upstream 一致；PageLayout.jsx 只新增 `isDocsRoute` 检查 + early return）
- **注意**：物理目录必须是 `product-docs/` 而不是 `docs/`，否则与 SPA 路由 `/docs` 冲突（static 中间件 301 到尾斜杠，前端路由再 301 去掉斜杠 → 死循环）。产品文档内图片路径全部用绝对路径 `/product-docs/images/...`。

### F-5 [csv-export] 日志页 "导出 CSV" 按钮
- **新增文件**：`web/classic/src/components/table/usage-logs/UsageLogsExportButton.jsx`
- **修改文件**：`web/classic/src/components/table/usage-logs/index.jsx`（把 `statsArea` 的 LogsActions 包一层 flex，加入 ExportButton）
- **冲突风险**：中（upstream 改 index.jsx 布局会冲突）

### F-6 [login] 登录表单定制
- **文件**：`web/classic/src/components/auth/LoginForm.jsx`
- **修改**：
  1. 邮箱/用户名登录按钮移到最前 + 加 Divider（L520-540）
  2. 注册入口总显示（两处 L700 和 L854 的 `!status.self_use_mode_enabled` → `true`）
- **冲突风险**：高（upstream 会持续优化登录表单 UI）
- **Merge 策略**：两个定制都加了 `// Fy-api overlay:` 注释方便辨认

### F-7 [theme-lock] 强制 classic 主题（新增于 2026-05-09）
- **修改文件**：
  - `setting/system_setting/theme.go`（`syncThemeToCommon` 强制把 `themeSettings.Frontend` 写回 `"classic"`）
  - `controller/option.go`（`case "theme.frontend":` 拒绝 `"default"` 写入）
- **目的**：上游 v1.0 后 `theme.frontend` 是一个 admin 后台可改的 option（5/1 之后的 `e0b6eb3a5` 还修了它从 DB 加载的同步问题）。TraceNex 选择路径 A：仅 ship classic，所以做了**双锁**——即使有人手动改 DB option 表也会在启动时被 `syncThemeToCommon` 覆写回 classic
- **冲突风险**：低（两处都是带 `// Fy-api overlay:` 注释的小改动）
- **可逆**：删掉这两处 overlay 即可回到上游"两套前端可切换"的行为，配合在 `web/default/` 重做 F-1~F-6 即为路径 B 的最小迁移路径

### F-8 [model-pricing] 管理端模型价格币种自选
- **修改文件**：
  - `web/classic/src/pages/Setting/Ratio/hooks/modelPricingCurrency.js`（新增 USD/CNY suffix、输入/展示换算、摘要换算 helpers；state 和后端 option 始终保持 USD）
  - `web/classic/src/pages/Setting/Ratio/hooks/useModelPricingEditorState.js`（接入币种换算 helpers 到编辑 state）
  - `web/classic/src/pages/Setting/Ratio/hooks/useModelPricingEditorState.test.js`（新增币种 suffix、汇率 fallback、USD/CNY 换算、摘要显示单元测试）
  - `web/classic/src/pages/Setting/Ratio/components/ModelPricingEditor.jsx`（计费方式下方新增币种按钮组，并为按量/按次价格输入动态 suffix 和展示值）
- **背景**：管理员模型价格编辑页原来硬编码 `$/1M tokens` / `$/次`，只能按美元输入；需求 REQ-20260512-01 要求可切换人民币输入，但后端 option 存储仍保持 USD。
- **行为**：默认 USD；所有计费模式都显示 USD/CNY 按钮；CNY 模式下输入框与左侧价格摘要按 `StatusContext.status.usd_exchange_rate` 展示人民币，输入时转换回 USD 存入编辑 state；表达式/阶梯计费本身仍按表达式原文保存。
- **冲突风险**：中（该编辑器是 upstream 设置页活跃区域）
- **Merge 策略**：若 upstream 重构模型价格编辑器，保留“UI 输入币种可选但 option 始终 USD”的存储语义，换算继续复用 `usd_exchange_rate`。

---

## 不 port 的 TraceNex 改动（技术债 / 已失效 / 上游已取代）

### X-1 ❌ middleware/auth.go 的 debugNDJSON
- 硬编码 Windows 路径 `d:\谷歌浏览器\new-api-main\.cursor\debug.log`
- **原因**：违反跨平台原则 + 安全隐患

### X-2 ❌ .cursor/debug.log 入库
- 开发者调试文件误提交
- **原因**：已 `.gitignore`

### X-3 ❌ web/dist 入库
- 前端构建产物
- **原因**：上游规范不入库，走 CI 构建

### X-4 ❌ 旧 OAuth controller（discord/github/linuxdo/oidc.go）
- **原因**：upstream 已统一到 `oauth/` registry 模式

### X-5 ❌ controller/task_video.go
- **原因**：upstream 已下沉到 `relay/channel/task/taskcommon/` + `relay/channel/task/gemini/`

### X-6 ❌ service/pre_consume_quota.go
- **原因**：upstream 已拆为 `text_quota.go` + `task_billing.go` + `violation_fee.go` + `funding_source.go`

### X-7 ❌ Home/index.jsx 微调（gap/图标）
- **原因**：cosmetic 调整，上游 Home 页已大量演进；需要时作为独立 UX 任务

---

## 待办（Pending port）

### ~~P-1 GroupRatioSettings 双维 port~~（已决议：不做）
- **状态**：**CLOSED**（2026-04-22）
- **原因**：TraceNex 基线的 `GroupRatioSettings.jsx` 已经同时提供「可视化编辑」和「手动 JSON 编辑」两种模式，且可视化模式下使用 `GroupTable` / `AutoGroupList` / `GroupGroupRatioRules` / `GroupSpecialUsableRules` 四个表格化子组件（基于 `CardTable + InputNumber + Checkbox`），是 TraceNex 当年 +976 行表格 UI 的完整**超集**（还多出 AutoGroups / DefaultUseAutoGroup / 内嵌使用说明 SideSheet 等能力）。TraceNex 的改动是在它 fork 时的老 new-api 上自己造的表格；上游官方随后也做了这个能力并做得更全。port 过来只会丢失新能力，价值为零。
- **参考子计划（已作废）**：`docs/Phase2.5-GroupRatioSettings-port-plan.md`

### P-2 存量 OAuth 用户迁移脚本
- **状态**：待写（Phase 3 Runbook §2.1 有 SQL 模板）
- **触发条件**：当 TraceNex 生产库有 discord/github/linuxdo 活跃用户

### P-3 Gemini 计费补偿审计
- **状态**：待做（用户已确认）
- **操作**：扫 TraceNex 2026-01-06 至修复生效前的 Gemini 日志，估算多扣金额，运营侧补偿

---

## 上游同步流程

见 `docs/Weekly-upstream-sync-runbook.md`（周合并 + 按需发版）。
