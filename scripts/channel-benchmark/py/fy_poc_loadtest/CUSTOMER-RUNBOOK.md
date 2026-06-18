# 国产 SOTA 模型简单压测说明

## 测试范围

本次测试面向以下国产 SOTA 模型：

- `MiniMax-M2.7`
- `kimi-k2.6`
- `deepseek-v4-pro`
- `glm-5.1`

测试并发：

- `1`
- `20`
- `80`
- `200`

测试场景：

- 短文本：客户口径为 `23K`，需确认是否为 `23K tokens`，还是原 POC 文档中的 `23 tokens`
- 长文本：`7K tokens`

输出指标：

- 首字延迟 `TTFT`
- 平均延迟 `Latency`
- 每 token 延迟 `TPOT`
- 单请求推理速度 `tokens/s`
- 请求成功率
- 错误分布

## 执行方式选择

### 方案 A：由 TraceNex 执行

请提供以下信息：

- 测试网关地址：例如 `https://api-test.tracenex.cn`
- 测试账号 API Key：`sk-...`
- 如需固定供应商渠道，请提供渠道 ID，并确认该 API Key 属于 admin 用户
- 确认模型名称是否与接口暴露名称完全一致：
  - `MiniMax-M2.7`
  - `kimi-k2.6`
  - `deepseek-v4-pro`
  - `glm-5.1`
- 提供或确认 23K / 7K 测试样本内容

TraceNex 将按配置文件执行测试，并交付：

- Markdown 测试报告
- CSV 明细结果
- JSON 原始聚合结果

### 方案 B：由客户自行执行

进入工具目录：

```bash
cd scripts/channel-benchmark/py/fy_poc_loadtest
```

配置环境变量：

```bash
export FY_API_URL=https://api-test.tracenex.cn
export FY_API_USER_TOKEN=sk-...
```

先校验配置：

```bash
make dry-run CONFIG=poc-loadtest-domestic-sota.yaml
```

执行完整测试：

```bash
make run CONFIG=poc-loadtest-domestic-sota.yaml
```

查看最新 Markdown 报告：

```bash
make latest-report CONFIG=poc-loadtest-domestic-sota.yaml
```

列出所有报告：

```bash
make list-reports CONFIG=poc-loadtest-domestic-sota.yaml
```

## 配置文件

本次测试使用：

```text
scripts/channel-benchmark/py/poc-loadtest-domestic-sota.yaml
```

关键配置如下：

```yaml
poc:
  models:
    - MiniMax-M2.7
    - kimi-k2.6
    - deepseek-v4-pro
    - glm-5.1

  concurrency_levels: [1, 20, 80, 200]

  scenarios:
    - name: 短文本_23K
      input_tokens: 23000
    - name: 长文本_7K
      input_tokens: 7000
```

正式测试前必须替换两个场景的 `prompt` 占位内容，否则报告只能验证脚本链路，不能代表真实 23K / 7K 压测结果。

## 成本与时长提醒

默认请求数沿用 POC 模板：

| 并发 | 请求数 |
|---:|---:|
| 1 | 50 |
| 20 | 200 |
| 80 | 300 |
| 200 | 500 |

每个模型会跑 2 个场景、4 个并发级别。4 个模型合计请求数为：

```text
4 models * 2 scenarios * (50 + 200 + 300 + 500) = 8400 requests
```

如果只是先做链路验证，可以临时打开配置里的小请求数：

```yaml
requests_by_concurrency:
  1: 10
  20: 40
  80: 80
  200: 120
```

## 对客户回复模板

可以直接回复：

```text
可以支持。我们这边已经按贵方口径准备了压测配置：
模型为 MiniMax-M2.7、kimi-k2.6、deepseek-v4-pro、glm-5.1；
并发为 1、20、80、200；
场景为短文本 23K 和长文本 7K；
报告会包含 TTFT、Latency、TPOT、tokens/s、成功率和错误分布。

执行方式有两种：
1. 贵方提供测试账号/API Key、测试网关地址、如需固定渠道则提供渠道 ID，我们按配置执行后回传 Markdown/CSV/JSON 报告；
2. 我们提供脚本和配置，由贵方在本地或测试环境自行执行。

另外需要确认一点：短文本“23K”是否确认为 23K tokens？因为早先 POC 文档里短文本是 23 tokens。如确认为 23K，请提供或确认可用于测试的 23K 样本文本；长文本 7K 也建议使用贵方确认的测试样本。
```
