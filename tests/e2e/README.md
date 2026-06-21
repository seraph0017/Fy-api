# e2e

面向测试环境的真实网关端到端验证脚本。

当前包含：

- `pr94_compat_e2e.py`
  - 验证 `#94` 相关的两类兼容性回归
  - Bedrock Claude 指针字段兼容性
  - Claude tool call 空 `arguments` 兼容性

## 用法

```bash
python3 tests/e2e/pr94_compat_e2e.py --dry-run
python3 tests/e2e/pr94_compat_e2e.py
```

默认会自动发现 `sg-test` 的：

- `FYAPI_E2E_BASE_URL`
- `FYAPI_E2E_TOKEN`
- Bedrock Claude 默认渠道/模型
- 普通 Claude 默认渠道/模型

前提：

- 本机可以 SSH 到测试机
- 测试机上 `/opt/fy-api/config/fy-api.env` 和数据库配置可读

如需切到其他环境：

```bash
python3 tests/e2e/pr94_compat_e2e.py --target-env sg-test --dry-run
python3 tests/e2e/pr94_compat_e2e.py --target-env cn-test --dry-run
```

如需手工覆盖：

```bash
export FYAPI_E2E_BASE_URL="https://api-test.aitracenex.com"
export FYAPI_E2E_TOKEN="<admin-user-token>"
export FYAPI_E2E_BEDROCK_MODEL="claude-sonnet-4-5"
export FYAPI_E2E_CLAUDE_MODEL="<claude-model>"

python3 tests/e2e/pr94_compat_e2e.py \
  --bedrock-channel-id 5 \
  --claude-channel-id 17
```

可选环境变量：

```bash
export FYAPI_E2E_TIMEOUT=90
```

## 通过标准

- Bedrock 4 个用例全部 `PASS`
- Claude 空 `arguments` 两个用例全部 `PASS`
- 不出现 `500`
- 不出现明显的网关转换错误关键字

## 说明

- 默认使用 channel pin，要求发现到的 token 属于 admin 用户
- `sg-test` 当前可自动发现 Bedrock Claude 和普通 Claude 两组默认值
- `cn-test` 当前没有真正的 AWS `type=33` Claude 渠道，所以 Bedrock 用例默认不会自动补全
- `FYAPI_E2E_BEDROCK_MODEL` 必须实际走 AWS Bedrock Claude 渠道
- `FYAPI_E2E_CLAUDE_MODEL` 必须实际走 Claude 兼容链路
- 这是黑盒验证，不依赖数据库或管理端 API
