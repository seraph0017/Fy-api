# Channel Benchmark Runbook

本手册面向完整渠道/模型测试。默认只使用一个配置文件和一个入口命令。

## 需要准备的信息

- `base_url`: 测试环境地址
  - 国内模型/国内供应商：优先 `cn-test`
  - 海外模型/海外供应商：优先 `hk-test`
- `channel_id`: 目标渠道 ID
- `models`: 明确模型 ID 列表，每个模型声明 `type: text|image|video`
- `FY_API_USER_TOKEN`: admin 用户的 `sk-...`，用于真实请求和 channel pin
- 可选 `FY_API_ADMIN_TOKEN`: 后台 access token，`fy-smoke` 查询渠道名/状态用
- 可选 `baseline_channel_id`: 可信对照渠道；有它才跑 canary baseline/audit
- 可选 judge/embedding token：默认不启用

## 一次完整测试

```bash
cd scripts/channel-benchmark
./install-env.sh --with-dev --with-tiktoken
source py/.venv/bin/activate

cd py
cp benchmark.yaml benchmark.local.yaml
```

编辑 `benchmark.local.yaml`：

```yaml
gateway:
  base_url: "https://api-test.tracenex.cn"
  tokens:
    user: "${FY_API_USER_TOKEN}"
    admin: "${FY_API_ADMIN_TOKEN:-}"

target:
  channel_id: 42
  channel_name: "channel-42"
  models:
    - id: "deepseek-r1"
      type: text
      backend: openai

profile:
  mode: standard
  parallel_models: 1
```

先 dry-run：

```bash
FY_API_USER_TOKEN=sk-xxx fy-benchmark -c benchmark.local.yaml --dry-run
```

确认计划无误后执行：

```bash
FY_API_USER_TOKEN=sk-xxx fy-benchmark -c benchmark.local.yaml
```

看结果：

```bash
open benchmark-runs/*/run-summary.md
open benchmark-runs/*/reports/scorecard.md
```

## 模式选择

- `quick`: 快速排障，少量请求，适合先判断渠道是否明显不可用。
- `standard`: 默认验收，覆盖性能、协议、质量、诚信和评分。
- `strict`: 高标准审计，压测更重、阈值更严，报告更突出不足。
- `deep`: 更完整也更贵；适合 judge、embedding、baseline 都配置好后的终验。

## 默认 quality 行为

默认不启用 LLM judge，只跑 deterministic graders。

差异：

- 优点：成本低、速度快、结果稳定，不会被 judge 延迟干扰。
- 缺点：开放式回答、语义相似度、主观质量覆盖较弱。
- 需要语义/主观质量时，显式配置：

```yaml
gateway:
  tokens:
    judge: "${FY_API_JUDGE_TOKEN}"
    embedding: "${FY_API_EMBEDDING_TOKEN}"
judge:
  enabled: true
  model: "claude-haiku-4-5-20251001"
embedding:
  enabled: true
  model: "text-embedding-v1"
```

## Canary / baseline

没有 baseline 时：

- `fy-benchmark` 默认不跑 canary baseline/audit。
- 仍会通过 conformance、integrity、quality、loadtest 发现大部分协议/性能/注水问题。
- 无法判断“该渠道输出是否偏离可信上游/对照渠道”。

有对照渠道时：

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
```

`fy-benchmark` 会先用 baseline 渠道录 baseline，再审计目标渠道。

## 输出目录

```text
benchmark-runs/<timestamp>-ch<id>/
├── manifest.json
├── run-summary.md
├── run-summary.json
├── configs/
├── logs/
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

排查慢点先看：

- `run-summary.md`: 每个模块耗时和退出码
- `logs/<model>-<module>.log`: 模块原始日志
- `reports/scorecard.md`: 最终 A/B/C/D/F 评分

## 底层模块调试

普通完整测试不需要手写底层 YAML。只有在某个模块失败时，进入 run 目录的 `configs/` 复用自动生成的 YAML：

```bash
fy-loadtest -c benchmark-runs/<run>/configs/<model>-loadtest.yaml
fy-conformance -c benchmark-runs/<run>/configs/<model>-conformance.yaml
fy-integrity run -c benchmark-runs/<run>/configs/<model>-integrity.yaml
fy-quality -c benchmark-runs/<run>/configs/<model>-quality.yaml
```

Prometheus 常驻监控仍用底层 `fy-smoke`：

```bash
fy-smoke -c smoke.yaml --prom-listen :9090 --prom-interval 5m
```
