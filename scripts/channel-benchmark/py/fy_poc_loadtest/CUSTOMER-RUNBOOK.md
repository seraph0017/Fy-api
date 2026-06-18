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

- 短文本：客户已确认按 `23 tokens` 执行
- 长文本：`7K tokens`
- 当前入库配置是 `30 RMB` 预算版：短文本覆盖并发 `1/20/80/200`，长文本只跑 `C=1` 基线

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
- 提供或确认 7K 长文本测试样本内容；短文本默认使用当前 23 tokens 样例，如客户有指定问句可替换

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
    - name: 短文本_23tokens
      input_tokens: 23
      concurrency_levels: [1, 20, 80, 200]
    - name: 长文本_7K
      input_tokens: 7000
      concurrency_levels: [1]
```

正式测试前建议替换长文本场景的 `prompt` 为客户确认样本；短文本场景也可按客户指定问句替换。否则报告更适合作为链路验证，而不是客户正式验收结果。

## 成本与时长提醒

当前入库的 `domestic-sota` 配置使用 30 RMB 预算版请求数：

| 并发 | 请求数 |
|---:|---:|
| 1 | 3 |
| 20 | 20 |
| 80 | 80 |
| 200 | 200 |

其中：

- 短文本场景会跑并发 `1/20/80/200`
- 长文本场景只跑并发 `1`

按当前入库配置，4 个模型合计请求数为：

```text
4 models * ((3 + 20 + 80 + 200) + 3) = 1224 requests
```

如果预算批准、需要恢复完整 POC 模板，可把配置改回：

```yaml
requests_by_concurrency:
  1: 50
  20: 200
  80: 300
  200: 500
```

## 对客户回复模板

可以直接回复：

```text
可以支持。我们这边已经按贵方口径准备了压测配置：
模型为 MiniMax-M2.7、kimi-k2.6、deepseek-v4-pro、glm-5.1；
并发为 1、20、80、200；
场景为短文本 23 tokens 和长文本 7K；
报告会包含 TTFT、Latency、TPOT、tokens/s、成功率和错误分布。

执行方式有两种：
1. 贵方提供测试账号/API Key、测试网关地址、如需固定渠道则提供渠道 ID，我们按配置执行后回传 Markdown/CSV/JSON 报告；
2. 我们提供脚本和配置，由贵方在本地或测试环境自行执行。

当前入库配置是 30 RMB 预算版：短文本覆盖 1、20、80、200 并发，长文本先跑 1 并发基线。如果需要恢复长文本 20/80/200 并发和完整模板请求数，我们可以在贵方确认预算后切回完整 POC 配置。长文本 7K 也建议使用贵方确认的测试样本。
```
