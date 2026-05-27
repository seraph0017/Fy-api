# 管理员操作审计日志 — 设计文档

> 日期：2026-05-27
> 分支：feature/audit-log
> 状态：设计中

## 背景

new-api 没有后台操作审计功能。Option 表只有 key/value 两个字段，无法追溯"谁在什么时候改了什么配置"。上游 GitHub 也没有相关 issue 或计划。

## 目标

记录所有管理员（Admin + Root）的写操作（POST/PUT/DELETE），包括：
- 谁操作的（user_id, username）
- 什么时候（timestamp）
- 操作了什么（resource, resource_id, action）
- 改了什么（Option 变更记录 old/new value，其他资源记录请求体摘要）

## 设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 存储方式 | 应用日志（logger.LogInfo） | 轻量，不加表不加 migration，依赖现有 logrotate 保留 14 天 |
| 拦截方式 | Gin Middleware | 零侵入 controller，不改上游逻辑 |
| 覆盖范围 | 所有 admin + root 写操作 | POST/PUT/DELETE on admin/root route groups |
| Option diff | 记录 old_value / new_value | 设置变更是最核心的审计场景 |
| 其他资源 diff | 请求体摘要（截断） | 渠道/用户字段太多，逐字段 diff 成本高 |

## 实现方案：Gin Middleware 拦截

### 新增文件

| 文件 | 职责 |
|------|------|
| `middleware/audit_log.go` | Gin middleware，拦截写操作并输出审计日志 |

### 对上游文件的改动

| 文件 | 改动 | 冲突风险 |
|------|------|----------|
| `router/api-router.go` | admin/root 路由组加 `.Use(middleware.AuditLog())` | 低（加一行） |

### 日志格式

```
[INFO] 2026/05/27 - 14:30:05 | SYSTEM | [AUDIT] user_id=1 username=admin action=update resource=option resource_id=ModelRatio ip=1.2.3.4 detail={"key":"ModelRatio","old_value":"{}","new_value":"{...}"}
[INFO] 2026/05/27 - 14:31:12 | SYSTEM | [AUDIT] user_id=1 username=admin action=update resource=channel resource_id=5 ip=1.2.3.4 detail={"summary":"PUT /api/channel body_size=1234"}
[INFO] 2026/05/27 - 14:32:00 | SYSTEM | [AUDIT] user_id=1 username=admin action=delete resource=user resource_id=42 ip=1.2.3.4
```

### Middleware 逻辑

```
请求进入
  ├─ 判断 HTTP method 是否为 POST/PUT/DELETE → 否则跳过
  ├─ 从 session/context 获取 user_id, username
  ├─ 如果是 PUT /api/option → 读取当前 Option 旧值
  ├─ 缓存 request body（限制最大 4KB 摘要）
  ├─ c.Next() 执行实际 handler
  └─ handler 返回后（仅 2xx 成功时）→ logger.LogInfo 输出审计行
```

### 字段说明

| 字段 | 来源 | 说明 |
|------|------|------|
| user_id | session / context | 操作人 ID |
| username | session / context | 操作人用户名 |
| action | HTTP method 映射 | POST→create, PUT→update, DELETE→delete |
| resource | URL path 解析 | /api/channel→channel, /api/option→option 等 |
| resource_id | URL path / body | 路径中的 :id 参数或 body 中的 key |
| ip | c.ClientIP() | 操作人 IP |
| detail | 条件记录 | Option: old/new value; 其他: body 摘要(≤4KB) |

### 日志保留

依赖现有 logrotate 配置：
- 落盘日志：14 天保留，每天切或 >500MB 切，gzip 压缩
- 容器 stdout：Podman `max-size=100m max-file=5`（500MB 循环）

### OVERLAY.md 条目（待实现后添加）

```
### B-xx [audit] 管理员操作审计日志（新增）
- **新增文件**：`middleware/audit_log.go`
- **修改文件**：`router/api-router.go`（admin/root 路由组加 .Use()）
- **冲突风险**：极低
```

## 不做的事

- 不建数据库表
- 不加前端页面
- 不做自动清理（依赖 logrotate）
- 不改任何 controller 逻辑
- 不记录 GET 请求（只读操作无需审计）
- 不记录失败的操作（handler 返回非 2xx 时跳过）
