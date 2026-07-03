# Config Sync (Fy-api Overlay)

**Purpose**: Single-direction sync of **config-only** tables from the CN Fy-api instance to the HK Fy-api instance.

**Why not DTS / BinLog sync?** See `docs/deploy/cross-region-sync-analysis.md`. Short version:

- DTS 跨境同步成本高 (~$200-500/mo), trips PIPL compliance
- Bi-directional sync has irreconcilable ID / token / quota conflicts
- Application-level sync of **only non-PII config** is the safest option

---

## What syncs

| Table | What syncs | What's stripped |
|---|---|---|
| `channels` | type, name, group, models, model_mapping, status, priority, ... | **`key`**, `openai_organization`, `used_quota`, `balance` (region-specific) |
| `abilities` | all columns (small table, full replace) | none |
| `options` | only whitelisted keys: `ModelRatio`, `GroupRatio`, `CompletionRatio`, `CacheRatio`, `ModelPrice`, `CompletionPrice` | everything else, **always excludes** `Session*` / `Crypto*` / `*Secret` / `*Key` / `*Token` / `*Password` keys (defense in depth) |

## What does NOT sync

- `users`, `tokens`, `logs`, `quota_records`, `topups`, `redemptions` — all user/billing/usage data stays in its region
- `channels.key` — each region has its own upstream API keys (billed separately)
- Any `options` key starting with a secret prefix

This keeps PII inside the originating region and avoids PIPL / GDPR / PDPA friction.

---

## How to run

### 1. On a trusted host (not the Fy-api container itself)

```bash
pip install pymysql requests

export CN_DB_DSN="mysql://fy_api_app:PASSWORD@rm-bp1u0xaq....mysql.rds.aliyuncs.com:3306/tracenex"
export HK_API_BASE="https://api.tracenex.hk"
export HK_INTERNAL_TOKEN="$(openssl rand -hex 32)"   # match the value on HK side
export STATE_FILE="$HOME/.fy_sync_state.json"

python3 scripts/ops/sync_config.py
```

### 2. Recommended deployment (Aliyun Function Compute)

Cost: ~free (well within monthly free tier for 10-minute interval).

1. Upload `sync_config.py` + `requirements.txt` (`pymysql`, `requests`) to a FC function (Python 3.11 runtime)
2. Configure env vars: `CN_DB_DSN`, `HK_API_BASE`, `HK_INTERNAL_TOKEN`
3. For `STATE_FILE`, use NAS or OSS — FC containers are ephemeral
4. Trigger: time-based, every 10 minutes
5. VPC: must be in CN RDS's VPC to reach the DB; NAT Gateway to reach HK's public API

### 3. Or simplest: cron on the CN ECS

```bash
# /etc/cron.d/fy-sync-config
*/10 * * * * root /usr/bin/python3 /opt/fy-api/scripts/ops/sync_config.py >> /var/log/fy-sync.log 2>&1
```

## HK-side endpoint

The HK Fy-api must implement three new admin endpoints:

- `POST /api/internal/sync/channels`
- `POST /api/internal/sync/abilities`  
- `POST /api/internal/sync/options`

All three require `Authorization: Bearer <HK_INTERNAL_TOKEN>` and **should not be exposed to the public internet**. Either:

- Bind them to localhost only in Nginx (`location /api/internal/ { allow 127.0.0.1; deny all; }`) and use SSH tunneling from CN to push
- **Or** IP-whitelist the CN syncer's public egress IP

Reference implementation is in `controller/internal_sync.go` (Fy-api overlay).

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `403 forbidden` on push | `HK_INTERNAL_TOKEN` mismatch | Check `docker logs fy-api-green` on HK side for token constant |
| `channels_last_sync` stuck | `updated_at` column on channels table doesn't update | Run `UPDATE channels SET updated_at = NOW() WHERE id = X` to force a re-sync |
| `options` not applying | HK Fy-api caches options in memory | HK side must call `options.Refresh()` after sync, or restart container |
| Duplicate channels on HK | HK already has a channel with same ID | One-time: `DELETE FROM channels WHERE id >= X` on HK side before first sync |

## Security

- `HK_INTERNAL_TOKEN` — treat like a DB password. Rotate quarterly
- Never log `CN_DB_DSN`
- `STATE_FILE` contains timestamps only, safe to inspect
