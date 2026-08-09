# Plan: 客户-渠道分层与优先级配置方案

**Generated**: 2026-06-15  
**Estimated Complexity**: High

## Overview

目标是把当前 new-api 的“渠道优先级、分组、倍率、自动兜底”能力，整理成一套适合 100~1000 客户规模的运营方案。

核心原则：

1. 客户差异放在“用户分组”层，不放在“单个渠道优先级”层。
2. 渠道只表达能力与成本，不直接表达客户。
3. 路由分组只表达策略，不承载无限细分客户。
4. 当两个客户对同一组物理渠道的优先级要求相反时，使用“逻辑渠道复制”而不是修改全局顺序。

## Prerequisites

- 现有分组、倍率、渠道、令牌配置可用。
- 已确认 `channel.group`、`priority`、`weight`、`user group`、`auto group` 的现有行为。
- 已确认需要兼容现有日志、计费、审计和 channel affinity。

## Sprint 1: 业务分层定型
**Goal**: 统一命名和边界，避免后续配置失控。  
**Demo/Validation**:
- 输出一张可执行的分层矩阵。
- 能回答“某客户属于哪个组、能用哪些路由组、走什么价格”的问题。

### Task 1.1: 定义客户分层
- **Location**: 设计文档 / 运维规范
- **Description**: 把客户分成 3~5 类，而不是按单客户无限拆分。
- **建议分层**:
  - `enterprise_low_latency`
  - `enterprise_balanced`
  - `smb_premium`
  - `smb_budget`
  - `sandbox/test`
- **Acceptance Criteria**:
  - 每个客户能明确归属一个主分层。
  - 大客户才允许额外例外组。

### Task 1.2: 定义路由分组
- **Location**: 分组命名规范
- **Description**: 将渠道能力按策略分为少量路由组。
- **建议路由组**:
  - `official_fast`
  - `relay_balanced`
  - `pool_cheap`
  - `reverse_backup`
  - `experimental`
- **Acceptance Criteria**:
  - 路由组数量控制在 4~8 个。
  - 每组都有明确定位和退出条件。

### Task 1.3: 定义渠道属性标签
- **Location**: 运维台账 / 渠道备注规范
- **Description**: 给每个渠道登记来源、价格、并发能力、稳定性、适用模型。
- **字段建议**:
  - 来源：官方 / 中转 / 号池 / 逆向
  - 价格：高 / 中 / 低，或实际倍率
  - 并发：高 / 中 / 低
  - 稳定性：高 / 中 / 低
  - 适用场景：主路由 / 备份 / 实验
- **Acceptance Criteria**:
  - 每个渠道的定位一眼可读。

## Sprint 2: 配置模型落地
**Goal**: 把“客户分层 -> 可用路由组 -> 具体渠道”的链路固定下来。  
**Demo/Validation**:
- 任意客户能通过配置表推导出可用渠道集。
- 不依赖人工记忆。

### Task 2.1: 设计用户组到路由组的映射
- **Location**: `setting/ratio_setting/group_ratio.go` 语义层 + 管理台配置规范
- **Description**: 用 user group 表示客户合同层，用 group ratio / special usable group 表示可用路由组。
- **规则**:
  - `group_ratio` 控制价格倍率。
  - `group_group_ratio` 控制某个用户组对某个路由组的特殊价格。
  - `GroupSpecialUsableGroup` 控制某用户组额外可见/不可见的路由组。
- **Acceptance Criteria**:
  - 运营可以只改分组配置，不改代码。

### Task 2.2: 设计渠道到路由组的映射
- **Location**: `channel.group`
- **Description**: 一个渠道可以挂多个路由组，但每个路由组只接收同策略渠道。
- **推荐做法**:
  - 官方高质量渠道放 `official_fast`
  - 低价转发放 `relay_balanced` 或 `pool_cheap`
  - 风险渠道放 `reverse_backup`
- **Acceptance Criteria**:
  - 同一渠道不要同时混进完全不同定位的组。

### Task 2.3: 规范 priority 与 weight
- **Location**: 渠道配置规范
- **Description**:
  - `priority` 表示同一 group 内的先后级。
  - `weight` 只在同 priority 内做流量分摊。
  - 不能用 `weight` 替代“客户偏好”。
- **Acceptance Criteria**:
  - 任何人看到配置，都能判断失败切换顺序。

## Sprint 3: 冲突路由策略
**Goal**: 解决“两个客户对同一批物理渠道优先级相反”的问题。  
**Demo/Validation**:
- 能为两个客户输出不同的最终路由顺序。
- 不需要改全局优先级。

### Task 3.1: 引入逻辑渠道复制策略
- **Location**: 运维配置规范
- **Description**: 当 A 客户希望 `渠道1 > 渠道2`，B 客户希望 `渠道2 > 渠道1` 时，不共用同一组逻辑配置。
- **做法**:
  - 复制同一 upstream 实体成两个逻辑渠道记录。
  - 分别放进不同路由组。
  - 给不同 group 配不同 `priority`。
- **Acceptance Criteria**:
  - 冲突顺序可以在配置层解决。
  - 不污染其他客户。

### Task 3.2: 设计 auto group 仅做兜底
- **Location**: `setting/auto_group.go` / 运营规范
- **Description**: `auto` 只用于跨组容灾，不用于复杂客户个性化。
- **规则**:
  - 优先按主 group 选。
  - 主 group 不可用时，才进入 auto 列表。
  - auto 列表顺序全局一致。
- **Acceptance Criteria**:
  - 任何客户都不会因为 auto 顺序漂移而路由失控。

### Task 3.3: 设计 channel affinity 使用边界
- **Location**: `setting/operation_setting/channel_affinity.go` 语义层
- **Description**: affinity 只做“同客户组内粘住一个好渠道”，不做跨客户共享策略。
- **规则**:
  - 同一客户组内允许 sticky。
  - 不同客户组之间不共享 affinity 结果。
  - 渠道 disabled 时可按现有策略清理或保留。
- **Acceptance Criteria**:
  - affinity 是优化项，不是主路由依赖。

## Sprint 4: 价格与并发治理
**Goal**: 把价格、并发、容量从路由逻辑中剥离出来，形成可运营规则。  
**Demo/Validation**:
- 每类客户能看到明确的价格、并发和容量约束。
- 高并发客户不会挤爆低成本渠道。

### Task 4.1: 分层定价
- **Location**: `group_ratio` / `group_group_ratio`
- **Description**: 用少量价格档位覆盖大多数客户。
- **建议**:
  - 标准价
  - 9 折
  - 8 折
  - 7 折
  - 特殊合同价
- **Acceptance Criteria**:
  - 定价规则不随渠道数线性增长。

### Task 4.2: 并发能力分桶
- **Location**: 渠道运维台账
- **Description**: 先按渠道并发能力做分桶，再把高并发客户导到对应桶。
- **建议**:
  - 高并发桶：官方、高质量中转
  - 中并发桶：混合路由
  - 低并发桶：低价备份
- **Acceptance Criteria**:
  - 高并发客户不会默认落到低并发桶。

### Task 4.3: 熔断与降级策略
- **Location**: 运营规则 + 自动禁用策略
- **Description**: 给每类渠道定义失败阈值和自动禁用策略。
- **规则**:
  - 官方渠道优先保稳定。
  - 低价渠道优先保成本。
  - 逆向渠道优先保备份。
- **Acceptance Criteria**:
  - 单个渠道异常不会拖垮整组客户。

## Sprint 5: 监控、验收与迁移
**Goal**: 上线前确认配置可观察、可回滚、可迁移。  
**Demo/Validation**:
- 按客户组、路由组、渠道维度都能看命中与失败。
- 旧客户迁移后行为可比对。

### Task 5.1: 建立验证矩阵
- **Location**: 测试文档 / 运营检查表
- **Description**: 为每个典型客户类型定义一组请求样本。
- **矩阵建议**:
  - 低价客户
  - 高并发客户
  - 稳定优先客户
  - 备份容灾客户
  - 特殊合同客户
- **Acceptance Criteria**:
  - 每次改配置后都能回归验证。

### Task 5.2: 建立路由审计视图
- **Location**: 日志 / 报表
- **Description**: 按 `user group / route group / channel / model / result` 聚合。
- **Acceptance Criteria**:
  - 能定位“为什么这个客户走了这条渠道”。

### Task 5.3: 迁移旧配置
- **Location**: 配置迁移清单
- **Description**: 将现有客户逐步迁入新分层，不做一次性大切换。
- **步骤**:
  - 先迁移测试客户。
  - 再迁移低风险客户。
  - 最后迁移大客户。
- **Acceptance Criteria**:
  - 每一步都可回滚到旧 group。

## Testing Strategy

- 用 3 类客户样本验证：
  - `same_channels_different_order`
  - `high_concurrency_low_cost`
  - `stable_first_backup_second`
- 用同一模型请求，检查最终命中的 group 和 channel 是否符合预期。
- 验证 `auto` 仅在主组不可用时触发。
- 验证同一物理渠道复制成不同逻辑渠道后，两个客户可以拿到相反优先级。

## Potential Risks & Gotchas

- 组数量膨胀会让运营失控，必须限制路由组数量。
- 如果把客户名直接写进 `channel.group`，后续维护会非常重。
- `weight` 不能解决客户级优先级冲突，只能解决同优先级分摊。
- `auto` 是兜底，不是客户定制主工具。
- 同一 upstream 实体的逻辑复制会增加渠道数，但这是可控的，远比全局规则冲突更稳定。

## Rollback Plan

- 任何新客户先只加到新 group，不迁移旧客户。
- 任何新路由组都保留回退到旧组的映射。
- 如果某组配置异常，直接移除该组在 `GroupSpecialUsableGroup` 的可见性。
- 如果某逻辑渠道策略出问题，禁用对应逻辑渠道，不动物理 upstream。

