# Fy-api channel QA

日常入口只需要一个配置文件和一个命令：

```bash
cd scripts/channel-benchmark
./install-env.sh --with-dev --with-tiktoken
source py/.venv/bin/activate

cd py
cp benchmark.yaml benchmark.local.yaml
fy-benchmark -c benchmark.local.yaml --dry-run
fy-benchmark -c benchmark.local.yaml
```

`benchmark.local.yaml` 里通常只改：

- `gateway.base_url`: `cn-test` 或 `hk-test`
- `gateway.tokens.user`: admin 用户的 `sk-...`，用于真实测试流量和 channel pin
- `target.channel_id`: 目标渠道 ID
- `target.models`: 要测的模型列表，每个模型声明 `type: text|image|video`
- `profile.mode`: `quick|standard|strict|deep`
- 可选 `target.baseline_channel_id`: 有可信对照渠道时启用 canary

`fy-benchmark` 会自动为底层模块生成临时配置，严格按模型串行执行，并把结果写入一个 run 目录。

## 目录结构

```text
scripts/channel-benchmark/
├── install-env.sh                 一键安装 Python 环境和轻量 fixtures
├── README.md                      顶层说明：如何跑完整渠道测试
├── RUNBOOK-channel-benchmark.md   运维/测试执行手册
├── fixtures/                      小型固定素材：图片、音频、视频、mask/source
├── incidents/                     已知事故和对应回归用例索引
└── py/
    ├── benchmark.yaml             用户入口配置模板；复制为 benchmark.local.yaml
    ├── fy_benchmark/              统一编排器：生成子配置、串行执行、汇总报告
    ├── fy_smoke/                  连通性、TTFT、E2E、usage、Prometheus exporter
    ├── fy_loadtest/               文本并发压测
    ├── fy_quality/                文本质量评测；默认 deterministic graders
    ├── fy_conformance/            文本协议兼容、4xx/5xx、错误泄漏检查
    ├── fy_integrity/              注水、缓存、确定性、流式、tool-use、过滤检查
    ├── fy_canary/                 文本模型替换/漂移检测
    ├── fy_image_loadtest/         图片生成压测
    ├── fy_image_conformance/      图片协议、输出、安全、性能检查
    ├── fy_image_canary/           图片真实性/指纹检测
    ├── fy_score/                  A/B/C/D/F 统一评分器
    ├── tests*/                    各模块离线测试
    └── benchmark-runs/            默认运行输出目录；每次执行一个时间戳子目录
```

`smoke.yaml`、`loadtest.yaml`、`quality.yaml` 等旧模板仍保留，用于调试单个模块；普通完整测试不要直接维护这些文件。

## 输出结构

一次 `fy-benchmark` 会生成类似：

```text
benchmark-runs/20260705T030627Z-ch42/
├── manifest.json                  本次目标、模型、计划步骤
├── run-summary.md                 每个模块耗时、退出码、日志路径
├── run-summary.json               机器可读执行摘要
├── configs/                       自动生成的底层模块 YAML
├── logs/                          每个模块 stdout/stderr，排查慢点看这里
├── smoke-results/
├── loadtest-results/
├── quality-results/
├── conformance-results/
├── integrity-results/
├── canary-results/
├── image-loadtest-results/
├── image-conformance-results/
├── image-canary-results/
└── reports/
    ├── scorecard.json
    └── scorecard.md
```

`logs/` 记录每个模块耗时和原始输出；如果你要判断慢在 agent、脚本还是渠道，上这里看每个模块的 wall time。

## 示例

文本渠道标准测试：

```yaml
gateway:
  base_url: "https://api-test.tracenex.cn"
  tokens:
    user: "${FY_API_USER_TOKEN}"
target:
  channel_id: 42
  models:
    - id: "deepseek-r1"
      type: text
      backend: openai
profile:
  mode: standard
```

多模型串行测试：

```yaml
target:
  channel_id: 42
  models:
    - {id: "deepseek-r1", type: text, backend: openai}
    - {id: "qwen3-max", type: text, backend: openai}
profile:
  mode: strict
  parallel_models: 1
```

图片模型：

```yaml
target:
  channel_id: 88
  models:
    - {id: "gpt-image-1", type: image}
profile:
  mode: standard
```

有 baseline 的 strict/canary：

```yaml
target:
  channel_id: 42
  baseline_channel_id: 12
  models:
    - {id: "claude-sonnet-4-6", type: text, backend: claude}
profile:
  mode: strict
modules:
  canary: true
embedding:
  enabled: true
  model: "text-embedding-v1"
gateway:
  tokens:
    user: "${FY_API_USER_TOKEN}"
    embedding: "${FY_API_EMBEDDING_TOKEN}"
```

## 模式

- `quick`: 快速排障，少量 smoke/load/conformance/integrity/quality。
- `standard`: 默认验收，覆盖常见性能、质量、协议和诚信问题。
- `strict`: 高标准审计，压测更重、阈值更严，报告会更突出短板。
- `deep`: 昂贵完整检查；适合已配置 judge、embedding、baseline 后做供应商终验。

默认 `quality` 不启用 LLM judge，只跑 deterministic graders。差异是：结果更稳定、成本更低、不会受 judge 延迟影响，但开放式回答、语义相似度和主观质量只能得到较弱覆盖；需要这些能力时再显式开启 `judge.enabled` 或 `embedding.enabled`。

## 底层 CLI

底层工具仍可单独使用：

- `fy-smoke`
- `fy-loadtest`
- `fy-quality`
- `fy-conformance`
- `fy-integrity`
- `fy-canary`
- `fy-image-loadtest`
- `fy-image-conformance`
- `fy-image-canary`
- `fy-score`

这些命令主要用于复现单个失败模块，或做 Prometheus exporter、POC 专项压测等非完整套件场景。
