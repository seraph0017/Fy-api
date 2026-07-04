# HK SOCKS5 代理服务操作手册

> 最后更新：2026-07-04

## 当前生产配置

HK 生产 `fy-api` 渠道里，目前有效 SOCKS5 代理配置如下：

| 渠道 ID | 渠道名 | 状态 | 代理 |
| --- | --- | --- | --- |
| 24 | 谷歌官方 | 启用 | `socks5://47.236.133.70:11080` |
| 27 | AWS官方 | 启用 | `socks5://47.236.133.70:11080` |

说明：

- `47.236.133.70` 是当前 SG 代理服务入口。
- 从 HK 生产机 `47.83.137.1` 验证 `socks5://47.236.133.70:11080` 可用，出口 IP 返回 `47.236.133.70`。
- 本地直连该代理会被 reset，推测代理服务或云安全组限制了来源 IP；生产验证应从 HK 生产机发起。
- 当前无法用本地默认 SSH key 登录 `47.236.133.70`，不能直接确认其 systemd 自启状态。后续拿到登录权限后，应执行本文的自启检查命令补验。

## hk-test / sg-test 配置

`hk-test`（旧称 `sg-test`）机器：`47.86.175.72`。

现有服务：

- systemd unit：`gost-proxy.service`
- 二进制：`/usr/local/bin/gost`
- 配置：`/etc/gost/config.yaml`
- 已有 HTTP 代理：`:18080`，带用户名密码，供 CN test 使用。
- 新增 SOCKS5 代理：`:11080`，无用户名密码，服务名 `socks5-for-hk-test`。

当前 `/etc/gost/config.yaml` 结构：

```yaml
services:
  - name: http-proxy-for-cn-test
    addr: :18080
    handler:
      type: http
      auth:
        username: fyapi
        password: <secret>
    listener:
      type: tcp
  - name: socks5-for-hk-test
    addr: :11080
    handler:
      type: socks5
    listener:
      type: tcp
```

验证结果：

- `systemctl status gost-proxy.service`：`enabled` + `active (running)`。
- 本机验证：`curl --socks5-hostname 127.0.0.1:11080 https://api.ipify.org` 返回 `47.86.175.72`。
- `hk-test` 测试库当前只有渠道 `谷歌官方`（ID 7）配置了 SOCKS5，仍指向生产 SG 代理 `socks5://47.236.133.70:11080`；尚未切到本机新增的 `47.86.175.72:11080`。
- UFW 已放行：
  - `47.83.137.1 -> 47.86.175.72:11080/tcp`
  - `8.156.88.148 -> 47.86.175.72:11080/tcp`
- 从 HK/CN test 远程访问 `47.86.175.72:11080` 仍超时，且 gost 日志和 UFW 计数没有命中，说明还需要在云安全组放行 `11080/tcp` 到指定来源 IP。

## 安装步骤

推荐使用 GOST v3，通过 systemd 管理。

### 1. 安装二进制

```bash
install -d -m 0755 /etc/gost
install -m 0755 gost /usr/local/bin/gost
/usr/local/bin/gost -V
```

如果从 GitHub release 下载，选择 linux/amd64 版本，下载后解压出 `gost` 放到 `/usr/local/bin/gost`。

### 2. 写配置

无认证 SOCKS5，只允许通过安全组和本机防火墙限制来源：

```yaml
services:
  - name: socks5-for-hk
    addr: :11080
    handler:
      type: socks5
    listener:
      type: tcp
```

如需与已有 HTTP 代理共存，保留原 service，只追加一个 `handler.type: socks5` 的 service。

### 3. 写 systemd unit

`/etc/systemd/system/gost-proxy.service`：

```ini
[Unit]
Description=Gost SOCKS5 Proxy
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/gost -C /etc/gost/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

启用并启动：

```bash
systemctl daemon-reload
systemctl enable --now gost-proxy.service
systemctl status gost-proxy.service --no-pager
```

### 4. 放行防火墙

只放行明确的 Fy-api 节点，不要全网开放无认证 SOCKS5。

```bash
ufw allow from 47.83.137.1 to any port 11080 proto tcp comment "Fy-api HK -> SOCKS5"
ufw allow from 8.156.88.148 to any port 11080 proto tcp comment "Fy-api cn-test -> SOCKS5"
ufw status verbose
```

还需要在云厂商安全组中放行同样规则：

| 端口 | 协议 | 来源 |
| --- | --- | --- |
| 11080 | TCP | `47.83.137.1/32` |
| 11080 | TCP | `8.156.88.148/32` |

## 验证命令

在代理机本机：

```bash
ss -ltnp | grep 11080
curl --max-time 10 --socks5-hostname 127.0.0.1:11080 https://api.ipify.org
systemctl is-enabled gost-proxy.service
systemctl is-active gost-proxy.service
```

在 HK 生产机：

```bash
curl --max-time 12 --socks5-hostname 47.236.133.70:11080 https://api.ipify.org
curl --max-time 12 --socks5-hostname 47.86.175.72:11080 https://api.ipify.org
```

预期返回代理机公网 IP。

## Fy-api 渠道配置

渠道 `setting` 字段中配置：

```json
{
  "proxy": "socks5://47.236.133.70:11080"
}
```

`hk-test` 云安全组放通后，可用：

```json
{
  "proxy": "socks5://47.86.175.72:11080"
}
```

注意：

- Fy-api 当前 `proxy` 是单个 URL 字符串，不支持在一个字段里配置多个 SOCKS5 IP。
- 如果需要多节点容灾，优先使用“每台代理服务器一条渠道”或在域名后接 L4 负载均衡。
- Go HTTP client 会复用连接，DNS 多 A 记录不能等价于可靠故障切换。
- 修改渠道 proxy 后，Fy-api 会重置 proxy client 缓存；若手动改数据库，需要重启服务或触发渠道更新流程。

## 开机自启检查

```bash
systemctl is-enabled gost-proxy.service
systemctl is-active gost-proxy.service
journalctl -u gost-proxy.service -b --no-pager | tail -n 80
```

`enabled` 表示开机自动启动，`active` 表示当前正在运行。
