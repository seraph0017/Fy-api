# Plan: Fy-api Singapore to Hong Kong Migration

**Generated**: 2026-06-23
**Estimated Complexity**: High

## Overview

目标是把当前新加坡生产环境迁移到香港新机。当前生产部署形态是单机 ECS/服务器上跑 Nginx + Podman 蓝绿容器，数据面依赖 MySQL/RDS、Redis，日志接入 SLS/Logtail，可选 Prometheus 监控栈。

推荐迁移策略是“先并行搭好香港环境，再做数据同步和 DNS 割接”，避免在 SG 机器上原地改配置。若香港数据面还没准备好，可以先用香港 ECS 继续连接 SG 的 MySQL/Redis 做连通性验证，但这只适合冒烟，不适合长期生产，因为跨区域延迟和网络抖动会直接影响每个 API 请求和计费写入。

## Current Production Services

| 服务 | 作用 | 当前部署/端口 | 迁移要求 |
|---|---|---:|---|
| Nginx | HTTPS 终止、反向代理、SSE 流式超时配置 | `80/tcp`, `443/tcp`; 配置反代到 `127.0.0.1:3001/3002` | 必迁 |
| Fy-api 容器 | Go API 网关 + classic 前端静态资源 | 容器内 `3000`; 宿主机 `127.0.0.1:3001` blue, `127.0.0.1:3002` green | 必迁 |
| MySQL/RDS | 主业务数据库、用户、渠道、令牌、日志、账务 | `3306/tcp` | 必迁或临时跨区连接 |
| Redis/Tair | 缓存、限流、同步状态 | `6379/tcp` | 强烈建议迁移 |
| ACR/镜像仓库 | 构建、拉取生产镜像 | SG 当前在 `ap-southeast-1` VPC registry | 建议换香港可访问仓库或确认跨区拉取 |
| Logtail/SLS | 应用日志、Nginx 日志、consume 账务日志 | `ilogtaild` 主动上报，无业务入站端口 | 建议迁移/新建 HK 机器组 |
| Logrotate | 宿主机日志轮转 | 无端口 | 必迁 |
| Certbot/Let's Encrypt | TLS 证书签发和续期 | 依赖公网 `80/tcp` HTTP-01 | 必迁 |
| Prometheus 栈 | 可选监控 | `9090`, `9093`, `3001`, `9100`, `8080`, `9115`, `9104`, `9121` | 可迁移；注意 Grafana `3001` 与 Fy-api blue 端口冲突 |
| SSH | 运维入口 | SG 配置是 `58422/tcp` | HK 建议沿用，但按实际安全组为准 |

## Important Ports

对公网只开放：

| 端口 | 协议 | 用途 |
|---:|---|---|
| `80` | TCP | HTTP 到 HTTPS 跳转、Let's Encrypt 验证 |
| `443` | TCP | Fy-api 生产入口 |
| `58422` 或实际 SSH 端口 | TCP | 运维 SSH，建议只对白名单 IP 开放 |

只绑定本机或内网，不对公网开放：

| 端口 | 协议 | 用途 |
|---:|---|---|
| `3000` | TCP | Fy-api 容器内部监听端口 |
| `127.0.0.1:3001` | TCP | blue 宿主机映射端口 |
| `127.0.0.1:3002` | TCP | green 宿主机映射端口 |
| `3306` | TCP | MySQL/RDS 内网连接 |
| `6379` | TCP | Redis/Tair 内网连接 |
| `9100/8080/9115/9104/9121/9090/9093` | TCP | 可选 Prometheus/exporter/Alertmanager；仅内网或 VPN |

## Files to Modify or Create

香港新机上的关键文件：

| 路径 | 作用 | 香港迁移动作 |
|---|---|---|
| `/opt/fy-api/config/fy-api.env` | 生产环境变量和密钥 | 必须创建/修改：`FRONTEND_BASE_URL`, `SQL_DSN`, `REDIS_CONN_STRING`, `NODE_NAME`, 性能参数 |
| `/etc/nginx/conf.d/fy-api.conf` | 主域名 HTTPS 反代 | 由 `scripts/prod/03-setup-nginx.sh` 生成；域名和 active port 要正确 |
| `/etc/nginx/conf.d/00-fy-api-log-format.conf` | Nginx 日志格式 | 由 `03-setup-nginx.sh` 从仓库复制 |
| `/etc/logrotate.d/fy-api` | 应用日志轮转 | 跑 `scripts/prod/07-setup-logrotate.sh` |
| `/etc/logrotate.d/fy-api-nginx` | Nginx 日志轮转 | 跑 `scripts/prod/07-setup-logrotate.sh` |
| `/etc/ilogtail/user_defined_id` | SLS 机器组标识 | HK 建议使用新标识，如 `fy-api-hk-prod` |
| `/root/Fy-api` | 服务器源码目录 | 已 clone 的话确认分支/ref 和 SG 一致 |
| `fabfile.py` | 本地 Fabric 目标配置 | 建议新增 `hk` target，避免复用 `sg` |

`/opt/fy-api/config/fy-api.env` 里最需要改的字段：

```dotenv
FRONTEND_BASE_URL=https://api.<your-hk-domain>
NODE_NAME=fy-api-hk-prod-1
SQL_DSN=fy_api_app:<PASS>@tcp(<hk-mysql-proxy-or-host>:3306)/<db>?charset=utf8mb4&parseTime=True&loc=Local
REDIS_CONN_STRING=redis://:<PASS>@<hk-redis-host>:6379/0
SESSION_SECRET=<必须和SG保持一致>
CRYPTO_SECRET=<必须和SG保持一致>
```

`SESSION_SECRET` 和 `CRYPTO_SECRET` 不要重新生成，必须从 SG 原样迁过来，否则登录态、加密字段或既有敏感配置可能失效。

## Sprint 1: Inventory and Decision

**Goal**: 明确迁移范围、停机窗口和可回滚基线。
**Demo/Validation**: 能列出 SG 当前镜像 tag、数据库实例、Redis 实例、域名、证书、日志和监控状态。

### Task 1.1: Confirm Migration Scope
- **Location**: 阿里云控制台、SG 服务器、HK 服务器
- **Description**: 决定是否迁移 MySQL/RDS、Redis、SLS、ACR、监控栈。
- **Dependencies**: None
- **Acceptance Criteria**:
  - 明确采用“全量迁移数据面到 HK”或“HK ECS 临时连 SG 数据面”。
  - 明确割接域名和 TTL。
- **Validation**:
  - 产出一页迁移参数表：SG IP、HK IP、域名、DB 地址、Redis 地址、镜像 tag、目标发版 tag。

### Task 1.2: Capture SG Baseline
- **Location**: SG 服务器
- **Description**: 记录当前运行状态，保留回滚信息。
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - 记录 `podman ps -a`、活跃容器颜色、活跃端口、当前镜像 tag。
  - 记录 `/etc/nginx/conf.d/fy-api.conf` 当前 upstream。
  - 记录 `/opt/fy-api/config/fy-api.env` 的非密码配置项。
- **Validation**:
  - `curl -s https://<current-domain>/api/status` 返回 success。

## Sprint 2: Prepare Hong Kong Host

**Goal**: HK 新机达到可部署 Fy-api 的系统状态。
**Demo/Validation**: HK 机器上 Nginx、Podman、目录、日志轮转都可用。

### Task 2.1: System Bootstrap
- **Location**: HK 服务器、`scripts/prod/01-setup-system.sh`
- **Description**: 安装 Podman、Nginx、certbot、logrotate、基础工具，创建 `/opt/fy-api`。
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - `/opt/fy-api/{logs,data,config,backup,scripts}` 存在。
  - `podman --version`、`nginx -t`、`systemctl is-active nginx` 正常。
- **Validation**:
  - `scripts/prod/01-setup-system.sh` 执行成功。

### Task 2.2: Add Fabric HK Target
- **Location**: `fabfile.py`
- **Description**: 新增 `hk` target，配置 HK host、SSH 端口、key、registry、namespace、repo、`/root/Fy-api`、`/opt/fy-api`。
- **Dependencies**: Task 2.1
- **Acceptance Criteria**:
  - `conda run -n fy-api-deploy fab info --target=hk` 显示 HK 配置。
  - `fab preflight --target=hk` 能连上服务器。
- **Validation**:
  - 不切换现有 `sg` target，不影响 SG 发版。

### Task 2.3: Configure Security Group
- **Location**: 云厂商控制台
- **Description**: 只开放公网 `80`, `443`, SSH 端口；MySQL/Redis 只允许内网/VPC 或白名单访问。
- **Dependencies**: Task 2.1
- **Acceptance Criteria**:
  - 公网不可访问 `3001`, `3002`, `3306`, `6379`, Prometheus exporter 端口。
- **Validation**:
  - 从外部 `nc -vz <hk-ip> 443` 通，`nc -vz <hk-ip> 3001` 不通。

## Sprint 3: Prepare Data Plane

**Goal**: HK 数据库和 Redis 准备好，并能从 HK 服务器连接。
**Demo/Validation**: HK Fy-api 可连接目标 MySQL/Redis。

### Task 3.1: Provision HK MySQL
- **Location**: HK RDS/MySQL
- **Description**: 创建数据库、应用账号、只读账号、可选 exporter 账号。字符集用 `utf8mb4`。
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - DB 名、账号权限和 SG 生产一致。
  - RDS 白名单包含 HK ECS 内网 IP。
- **Validation**:
  - 从 HK 服务器执行 MySQL client `SELECT VERSION();` 成功。

### Task 3.2: Migrate MySQL Data
- **Location**: SG DB, HK DB
- **Description**: 选择 DTS 或 `mysqldump`/`mysql` 迁移。低停机场景建议先全量导入，再在割接窗口做最终增量/冻结写入。
- **Dependencies**: Task 3.1
- **Acceptance Criteria**:
  - HK DB 表结构和核心数据完整。
  - 用户、渠道、令牌、模型配置、账务日志、options 表迁移完成。
- **Validation**:
  - 对比 SG/HK 的关键表行数和最近日志时间。
  - 在 HK 只读窗口启动 Fy-api 后能登录后台查看渠道和用户。

### Task 3.3: Provision and Warm Redis
- **Location**: HK Redis/Tair
- **Description**: 创建 HK Redis，设置密码和白名单。Redis 可以不做全量数据迁移，通常缓存/限流状态允许冷启动；若业务要求保留限流窗口，可用 `redis-cli --rdb` 或云迁移工具。
- **Dependencies**: Task 1.1
- **Acceptance Criteria**:
  - HK Redis 连接串可用。
  - 明确是否迁移 Redis 数据；默认不迁移缓存。
- **Validation**:
  - 从 HK 服务器 `redis-cli -u "$REDIS_CONN_STRING" PING` 返回 `PONG`。

## Sprint 4: Configure HK Fy-api

**Goal**: HK 服务在不切 DNS 的情况下可通过本机和临时域名健康运行。
**Demo/Validation**: `http://127.0.0.1:3001/api/status` 和临时 HTTPS 域名都成功。

### Task 4.1: Create Production Env File
- **Location**: `/opt/fy-api/config/fy-api.env`
- **Description**: 从 SG 复制密钥，从 HK 数据面填入新 DB/Redis 地址。
- **Dependencies**: Sprint 3
- **Acceptance Criteria**:
  - 文件权限为 `600`。
  - `FRONTEND_BASE_URL` 是最终 HK 生产域名；若先用临时域名验证，要记录割接时改回。
  - `SESSION_SECRET`、`CRYPTO_SECRET` 与 SG 一致。
- **Validation**:
  - `grep -E '^(FRONTEND_BASE_URL|NODE_NAME|SQL_DSN|REDIS_CONN_STRING)=' /opt/fy-api/config/fy-api.env` 检查值，不打印密码到公共渠道。

### Task 4.2: Build or Pull Image
- **Location**: HK 服务器、ACR
- **Description**: 使用与 SG 当前一致的 tag 先部署，避免代码变化和迁移问题混在一起。
- **Dependencies**: Task 2.2
- **Acceptance Criteria**:
  - HK 服务器能 `podman pull` 目标镜像，或能本机构建并打同一 tag。
- **Validation**:
  - `podman image exists <image>:<tag>` 成功。

### Task 4.3: Start First Blue Container
- **Location**: `scripts/prod/04-deploy-fyapi.sh`
- **Description**: 启动 `fy-api-blue`，宿主机映射 `127.0.0.1:3001:3000`。
- **Dependencies**: Task 4.1, Task 4.2
- **Acceptance Criteria**:
  - `fy-api-blue` Up。
  - `/opt/fy-api/logs/oneapi-*.log` 生成。
- **Validation**:
  - `curl -s http://127.0.0.1:3001/api/status | jq .` 返回 `success: true`。

### Task 4.4: Configure Nginx and TLS
- **Location**: `scripts/prod/03-setup-nginx.sh`, `/etc/nginx/conf.d/fy-api.conf`
- **Description**: 用最终域名或临时验证域名生成 Nginx 配置和证书。
- **Dependencies**: Task 4.3
- **Acceptance Criteria**:
  - Nginx upstream 指向 `127.0.0.1:3001`。
  - HTTPS、SSE 关闭缓冲、900s proxy timeout 保持。
- **Validation**:
  - `curl -I https://<hk-domain>/api/status` 返回 200。
  - `nginx -t` 成功。

### Task 4.5: Install Logrotate and Logtail
- **Location**: `scripts/prod/07-setup-logrotate.sh`, `scripts/prod/02-install-logtail.sh`
- **Description**: 配置应用日志和 Nginx 日志轮转；如继续用阿里云 SLS，新建 HK 机器组和采集配置。
- **Dependencies**: Task 4.4
- **Acceptance Criteria**:
  - `/etc/logrotate.d/fy-api` 和 `/etc/logrotate.d/fy-api-nginx` 存在。
  - SLS 中能看到 HK 新机器日志。
- **Validation**:
  - `sudo logrotate -d /etc/logrotate.d/fy-api` 成功。
  - SLS 查询 `NODE_NAME=fy-api-hk-prod-1` 或 HK 机器组有新日志。

## Sprint 5: Pre-Cutover Validation

**Goal**: 在正式 DNS 切换前证明 HK 环境能承载真实业务。
**Demo/Validation**: 管理后台、OpenAI-compatible API、Claude/Gemini/图片/任务路径按核心场景通过。

### Task 5.1: Backend Smoke Tests
- **Location**: HK 域名或临时域名
- **Description**: 验证 `/api/status`、后台登录、渠道列表、令牌、日志页。
- **Dependencies**: Sprint 4
- **Acceptance Criteria**:
  - 管理员可登录。
  - 关键配置和 SG 一致。
- **Validation**:
  - 使用只读检查，避免在割接前产生业务写入混乱。

### Task 5.2: API Compatibility Smoke Tests
- **Location**: HK 域名或临时域名
- **Description**: 用测试 token 验证 `/v1/chat/completions`、流式请求、常用模型、图片/任务模型如有使用。
- **Dependencies**: Task 5.1
- **Acceptance Criteria**:
  - 非流式和流式请求都成功。
  - 消费日志产生并能查询。
- **Validation**:
  - `scripts/channel-benchmark/go` 可跑一轮 smoke。

### Task 5.3: Observability Checks
- **Location**: SLS/Prometheus/Grafana
- **Description**: 验证 Nginx access/error、应用日志、consume 日志、告警路径。
- **Dependencies**: Task 5.2
- **Acceptance Criteria**:
  - 可按 request_id 查询 HK 请求。
  - 5xx/429/panic 告警规则覆盖 HK 日志。
- **Validation**:
  - 人工触发一条测试请求，在 SLS 中查到完整链路。

## Sprint 6: Cutover

**Goal**: 用最短写入冻结窗口把生产流量从 SG 切到 HK。
**Demo/Validation**: 生产域名解析到 HK，真实客户请求成功，SG 保留可回滚。

### Task 6.1: Lower DNS TTL
- **Location**: DNS 控制台
- **Description**: 提前 24 小时把生产域名 TTL 降到 60 秒或供应商允许的最低值。
- **Dependencies**: Sprint 5
- **Acceptance Criteria**:
  - TTL 已生效。
- **Validation**:
  - `dig <domain> +nocmd +noall +answer` 看到低 TTL。

### Task 6.2: Freeze SG Writes
- **Location**: SG 服务器、Nginx/运维公告
- **Description**: 在最终同步窗口短暂停写。可以通过维护页、Nginx deny 写接口、或直接停 SG active 容器实现，具体取决于可接受停机方式。
- **Dependencies**: Task 6.1
- **Acceptance Criteria**:
  - 没有新的业务写入进入 SG DB。
  - SG 当前 DB 做最终备份。
- **Validation**:
  - SG consume/logs 最近时间不再增长。

### Task 6.3: Final Data Sync
- **Location**: SG DB -> HK DB
- **Description**: 执行最终增量或重新导入停写后的最终 dump。
- **Dependencies**: Task 6.2
- **Acceptance Criteria**:
  - HK DB 是最终数据版本。
  - 自增 ID、options、tokens、channels、logs 完整。
- **Validation**:
  - 对比关键表行数、最大 ID、最近更新时间。

### Task 6.4: Switch DNS to HK
- **Location**: DNS 控制台
- **Description**: 把生产 API 域名 A/CNAME 切到 HK IP 或 HK 负载入口。
- **Dependencies**: Task 6.3
- **Acceptance Criteria**:
  - 多地 DNS 查询逐步返回 HK。
- **Validation**:
  - `curl --resolve <domain>:443:<hk-ip> https://<domain>/api/status` 成功。
  - DNS 生效后直接 `curl https://<domain>/api/status` 成功。

### Task 6.5: Post-Cutover Watch
- **Location**: HK 服务器、SLS、客户侧
- **Description**: 观察 30-60 分钟，重点看 5xx、429、DB 连接、Redis 连接、上游渠道错误、流式中断。
- **Dependencies**: Task 6.4
- **Acceptance Criteria**:
  - 错误率和延迟在可接受范围。
  - 新消费日志只进入 HK。
- **Validation**:
  - `podman logs --tail 200 fy-api-blue/green` 无异常。
  - SLS 中 HK 日志持续增长，SG 日志无新增生产请求。

## Rollback Plan

1. 保持 SG 环境不删除、不升级，至少保留 24-72 小时。
2. 如果 HK 服务异常但数据尚未产生不可合并写入，直接把 DNS 切回 SG IP。
3. 如果 HK 已产生写入，需要先决定是否把 HK 新增写入回灌 SG，再切回；否则会丢失割接后的用户、充值、令牌、日志和账务变化。
4. 回滚 DNS 后验证 `https://<domain>/api/status`、后台登录、真实 API token 请求。
5. HK 容器不要立刻删除，先保留日志和 DB 备份用于复盘。

## Testing Strategy

- 系统层：`nginx -t`、`podman ps`、`curl http://127.0.0.1:3001/api/status`。
- 数据层：MySQL 关键表行数、最大 ID、最近更新时间；Redis `PING`。
- API 层：`/api/status`、后台登录、`/v1/chat/completions` 非流式和流式。
- 账务层：发一条测试请求，确认 consume log 和后台日志账务字段正常。
- 观测层：SLS 能查 app、nginx-access、nginx-error、consume；告警规则包含 HK。

## Potential Risks and Gotchas

- `SESSION_SECRET` / `CRYPTO_SECRET` 若重新生成，会导致登录态或加密数据不可用。
- 如果 HK ECS 继续连 SG RDS/Redis，API 请求会跨区访问数据库和缓存，延迟会明显上升，只适合短期验证。
- DNS 割接后仍有客户端缓存旧 IP，所以 SG 至少保留一段时间，不要立即停机。
- Grafana 默认映射宿主机 `3001`，会和 Fy-api blue 端口冲突；生产机上建议不要把 Grafana 直接映射到 `3001`，或迁到单独监控机。
- Certbot HTTP-01 要求域名已解析到 HK 且公网 `80` 可达；如果提前不能改正式域名，需要用临时域名或 DNS-01。
- SLS 机器组标识如果沿用 SG 的 `fy-api-prod`，日志会混在一起；推荐 HK 用单独标识和字段区分。
- ACR VPC 内网地址是 region 相关的；SG 的 `ap-southeast-1` VPC registry 到 HK 可能不可用或走公网，需单独验证。
- Redis 默认可以冷启动，但限流窗口和缓存会丢；这是可接受行为时才不迁 Redis 数据。
