# e2e

面向测试环境的真实网关端到端验证脚本。

当前包含：

- `pr94_compat_e2e.py`
  - 验证 `#94` 相关的两类兼容性回归
  - Bedrock Claude 指针字段兼容性
  - Claude tool call 空 `arguments` 兼容性

## 用法

```bash
export FYAPI_E2E_BASE_URL="https://api-test.tracenex.cn"
export FYAPI_E2E_TOKEN="<user-token>"
export FYAPI_E2E_BEDROCK_MODEL="<bedrock-claude-model>"
export FYAPI_E2E_CLAUDE_MODEL="<claude-model>"

python3 tests/e2e/pr94_compat_e2e.py --dry-run
python3 tests/e2e/pr94_compat_e2e.py
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

- `FYAPI_E2E_BEDROCK_MODEL` 必须实际走 AWS Bedrock Claude 渠道
- `FYAPI_E2E_CLAUDE_MODEL` 必须实际走 Claude 兼容链路
- 这是黑盒验证，不依赖数据库或管理端 API
