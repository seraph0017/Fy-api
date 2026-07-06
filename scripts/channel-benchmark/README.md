# channel-benchmark

用于测试 Fy-api 渠道和模型的完整工具链。日常使用只暴露一个入口：

```bash
cd scripts/channel-benchmark
./install-env.sh --with-dev --with-tiktoken
source py/.venv/bin/activate

cd py
cp benchmark.yaml benchmark.local.yaml
fy-benchmark -c benchmark.local.yaml --dry-run
fy-benchmark -c benchmark.local.yaml
```

所有测试都会走真实网关请求并消耗真实 quota。建议测试 token 设置较小额度，作为成本上限。

## 目录结构

```text
channel-benchmark/
├── install-env.sh                 安装 Python venv、依赖、生成小 fixtures
├── README.md                      本文件：用户入口和目录结构
├── RUNBOOK-channel-benchmark.md   执行手册
├── unified-benchmark-runner-plan.md
│                                  统一编排器长期规划
├── fixtures/                      可提交的小素材：图片、mask、音频、视频
├── incidents/                     事故回归索引
└── py/
    ├── benchmark.yaml             单入口配置模板
    ├── fy_benchmark/              `fy-benchmark` 编排器
    ├── fy_smoke/                  连通性、TTFT、E2E、usage、Prometheus
    ├── fy_loadtest/               文本并发压测
    ├── fy_quality/                文本质量评分
    ├── fy_conformance/            协议兼容和错误语义
    ├── fy_integrity/              token 注水、缓存、流式、tool-use 等
    ├── fy_canary/                 模型替换/漂移检测
    ├── fy_image_loadtest/         图片压测
    ├── fy_image_conformance/      图片协议/输出/安全/性能
    ├── fy_image_canary/           图片真实性/指纹检测
    ├── fy_score/                  综合评分和 A/B/C/D/F 报告
    └── benchmark-runs/            默认输出目录
```

底层模块的 YAML 示例仍保留在 `py/` 下，主要用于调试单个模块；完整渠道测试优先使用 `benchmark.yaml`。

## 单入口配置

`py/benchmark.yaml` 是模板，复制成 `benchmark.local.yaml` 后填写：

```yaml
gateway:
  base_url: "https://api-test.tracenex.cn"
  tokens:
    user: "${FY_API_USER_TOKEN}"       # admin 用户 sk key，用于真实请求和 channel pin
    admin: "${FY_API_ADMIN_TOKEN:-}"   # 可选；fy-smoke 查询渠道元数据用

target:
  channel_id: 42
  channel_name: "supplier-a"
  baseline_channel_id: 12              # 可选；strict/deep canary 用
  models:
    - id: "deepseek-r1"
      type: text
      backend: openai
    - id: "gpt-image-1"
      type: image

profile:
  mode: standard                       # quick | standard | strict | deep
  parallel_models: 1                   # 默认严格串行
```

`channel_name` 只是展示名；执行以 `channel_id` 为准。

## 输出

每次运行会生成独立目录：

```text
py/benchmark-runs/<timestamp>-ch<id>/
├── manifest.json
├── run-summary.md
├── run-summary.json
├── configs/       自动生成的底层配置
├── logs/          每个模块 stdout/stderr 和耗时排查入口
├── *-results/     各模块 JSON/CSV/Markdown 输出
└── reports/
    ├── scorecard.json
    └── scorecard.md
```

如果测试慢，先看 `run-summary.md` 和 `logs/`：它能区分慢在 smoke、loadtest、quality、conformance、integrity、canary 还是图片模块。

## 测试范围

文本模型默认覆盖：

- `fy-smoke`: 连通性、stream/non-stream、E2E、TTFT、ITL、usage
- `fy-conformance`: `/v1/chat/completions` 常见参数、错误语义、字段泄漏
- `fy-integrity`: token 注水、缓存、确定性、流式重打包、tool-use、过滤
- `fy-loadtest`: 并发阶梯、延迟分位、吞吐、错误分布
- `fy-quality`: 默认 deterministic graders；可选 judge/embedding
- `fy-canary`: 有 baseline 时检测模型替换/漂移

图片模型默认覆盖：

- `fy-image-loadtest`: 图片生成延迟、成功率、吞吐
- `fy-image-conformance`: `/v1/images/generations` 兼容、输出有效性、安全和轻量性能
- `fy-image-canary`: 有 baseline/vendor 配置时检测真实性/指纹

视频模型目前会在 `fy-benchmark` 中标记为 skipped；完整 `fy-video-*` runner 仍待实现。
