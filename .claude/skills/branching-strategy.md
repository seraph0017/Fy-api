---
name: branching-strategy
description: >
  TraceNex/Fy-api 分支策略与开发流程。收到代码修改任务时必须先判断属于哪种分支类型，
  再按对应流程执行。触发词："改代码"、"修 bug"、"加功能"、"修复线上"、"重构"、
  "升级依赖"、"写文档"、任何暗示需要新分支的开发任务。
---

# TraceNex 分支策略

收到任何代码变更任务时，**第一步**是判断属于以下哪种类型，然后严格按对应流程执行。不确定时主动问用户。

## 分支类型总览

| 类型 | 分支命名 | 基于 | 合并目标 | 适用场景 |
|------|---------|------|---------|---------|
| **feature** | `feature/<topic>` | `develop` | → `develop` (PR) | 新功能、新接口、新页面 |
| **bugfix** | `bugfix/<topic>` | `develop` | → `develop` (PR) | 非紧急 bug，可以随下次版本发布 |
| **hotfix** | `hotfix/<topic>` | `origin/main` | → `main` (PR) + cherry-pick → `develop` (PR) | 线上紧急问题，必须立即修复 |
| **docs** | `docs/<topic>` | `develop` | → `develop` (PR) | 文档变更（CLAUDE.md、OVERLAY.md、docs/、注释等） |
| **refactor** | `refactor/<topic>` | `develop` | → `develop` (PR) | 重构（不改外部行为），代码整理、结构调整 |
| **chore** | `chore/<topic>` | `develop` | → `develop` (PR) | 依赖升级、CI/CD 配置、构建脚本、工具链变更 |

## 判断规则

按优先级从高到低：

1. **线上正在报错 / 用户正在受影响 / 明确说"紧急"** → `hotfix`
2. **纯文档、注释、README** → `docs`
3. **依赖升级、CI 配置、Makefile、fabfile** → `chore`
4. **不改行为只改结构、命名、拆分** → `refactor`
5. **修复已知 bug，但不紧急** → `bugfix`
6. **新功能、新能力** → `feature`

> 如果一个任务同时涉及多个类型（例如修 bug 同时重构），以**主要目的**为准。

## 分支命名规范

- `<topic>` 用小写英文，单词间用连字符 `-` 分隔
- 保持简短且有描述性（3-5 个词以内）
- 示例：`feature/billing-expr-v2`、`bugfix/gpt5-strip-stop`、`hotfix/gemini-cache-billing`、`docs/overlay-tnbiz-update`、`refactor/relay-adapter-cleanup`、`chore/bump-go-1.25.2`

## 各类型详细流程

### feature / bugfix / docs / refactor / chore（常规流程）

```
1. git fetch origin
2. git checkout -b <type>/<topic> origin/develop
3. 做修改
4. 针对性测试（go build / go test）
5. git add <files>
6. git commit
7. git push -u origin <type>/<topic>
8. gh pr create --base develop
```

### hotfix（紧急流程）

> 详细步骤见 `hotfix-pr-flow` skill，这里是摘要。

```
1. git fetch origin
2. git checkout -b hotfix/<topic> origin/main
3. 做修改 + 针对性测试
4. git add <files> && git commit
5. git push -u origin hotfix/<topic>
6. gh pr create --base main                          # PR-1: 生产修复
7. git checkout -b hotfix/<topic>-develop origin/develop
8. git cherry-pick <上一步的 commit hash>
9. git push -u origin hotfix/<topic>-develop
10. gh pr create --base develop                       # PR-2: 同步到 develop
11. 报告两个 PR 链接
```

**hotfix 关键约束：**
- 绝不直接推 main，必须走 PR
- develop 侧必须是 cherry-pick，不是重新实现
- release 映射：`main` → `cn`/`sg`（生产），`develop` → `test`（测试）
- 未经用户二次确认，不得将 develop 发布到生产目标

## Commit Message 规范

格式：`<type>(<scope>): <description>`

| 分支类型 | commit type |
|---------|-------------|
| feature | `feat` |
| bugfix | `fix` |
| hotfix | `fix` |
| docs | `docs` |
| refactor | `refactor` |
| chore | `chore` |

示例：
- `feat(relay): add deepseek-r2 adapter`
- `fix(billing): correct cache token ratio for gemini`
- `docs(overlay): update OVERLAY.md with new entries`
- `refactor(service): extract billing pipeline into separate functions`
- `chore(deps): bump go-redis to v9.8`

## PR 规范

- 标题 < 70 字符，格式同 commit message
- Body 包含 `## Summary`（要点）和 `## Test plan`（验证步骤）
- hotfix PR 需在 Summary 中注明影响范围（受影响用户数、错误量、持续时间）

## 与版本发布的关系

- **hotfix** 合并 main 后可立即 `fab release --target=cn|sg`
- **其他类型** 合并 develop 后，随下一个版本发布周期统一从 develop 合并到 main 再 release
- 版本号遵循 CLAUDE.md Rule 8：`x.x.x-tracenex` 格式
