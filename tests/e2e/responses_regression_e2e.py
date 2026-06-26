#!/usr/bin/env python3
"""
Responses regression e2e checks.

Validates two TraceNex overlay behaviors on /v1/responses:
1. strips client-internal metadata fields before forwarding upstream
2. retries once without stale encrypted_content after the specific upstream 400

Required env:
  FYAPI_E2E_BASE_URL
  FYAPI_E2E_TOKEN
  FYAPI_E2E_RESPONSES_MODEL

Optional env:
  FYAPI_E2E_TIMEOUT      default: 90
  FYAPI_E2E_CHANNEL_ID   default: auto-discovered
"""

from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
import time
from dataclasses import dataclass
from typing import Any

import httpx


@dataclass
class CaseResult:
    name: str
    ok: bool
    detail: str
    status_code: int
    request_id: str = ""


@dataclass
class TargetDefaults:
    target_env: str
    base_url: str
    ssh_target: str
    token: str = ""
    model: str = ""
    channel_id: int | None = None


TARGET_DEFAULTS: dict[str, TargetDefaults] = {
    "hk-test": TargetDefaults(
        target_env="hk-test",
        base_url="https://api-test.aitracenex.com",
        ssh_target="root@47.86.175.72 -p 58422",
    ),
    "cn-test": TargetDefaults(
        target_env="cn-test",
        base_url="https://api-test.tracenex.cn",
        ssh_target="root@8.156.88.148 -p 58422",
    ),
}


def env(name: str, default: str = "") -> str:
    return os.getenv(name, default).strip()


def mask_token(token: str) -> str:
    if not token:
        return ""
    if len(token) <= 12:
        return token[:2] + "..." + token[-2:]
    return token[:6] + "..." + token[-4:]


def auth_headers(token: str, pin_channel_id: int | None = None) -> dict[str, str]:
    effective_token = token if pin_channel_id is None else f"{token}-{pin_channel_id}"
    return {"Authorization": f"Bearer {effective_token}", "Content-Type": "application/json"}


def run_ssh_python(ssh_target: str, script: str) -> dict[str, Any]:
    proc = subprocess.run(
        ["bash", "-lc", f"cat <<'PY' | ssh {ssh_target} python3 -\n{script}\nPY"],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"ssh discovery failed: {proc.stderr.strip() or proc.stdout.strip()}")
    payload = json.loads(proc.stdout.strip())
    return payload if isinstance(payload, dict) else {}


def run_ssh_command(ssh_target: str, command: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", "-lc", f"ssh {ssh_target} {shlex.quote(command)}"],
        capture_output=True,
        text=True,
        check=False,
    )


def discover_target_defaults(target_env: str) -> TargetDefaults:
    if target_env not in TARGET_DEFAULTS:
        raise ValueError(f"unsupported target env: {target_env}")
    defaults = TARGET_DEFAULTS[target_env]
    remote_script = r"""
import json
import pymysql
import re
from pathlib import Path

env_text = Path('/opt/fy-api/config/fy-api.env').read_text()
dsn = re.search(r'SQL_DSN=(.+)', env_text).group(1)
m = re.match(r'([^:]+):(.+)@tcp\(([^:]+):(\d+)\)/([^?]+)', dsn)
if not m:
    raise SystemExit('cannot parse SQL_DSN')
user, pw, host, port, db = m.groups()

conn = pymysql.connect(
    host=host,
    user=user,
    password=pw,
    port=int(port),
    database=db,
    charset='utf8mb4',
)
cur = conn.cursor()

result = {
    "token": "",
    "model": "",
    "channel_id": None,
}

cur.execute(
    '''
    SELECT t.key
    FROM tokens t
    JOIN users u ON u.id = t.user_id
    WHERE t.status = 1
      AND t.deleted_at IS NULL
      AND u.role = 100
      AND (t.`group` = '' OR t.`group` = 'default')
    ORDER BY t.unlimited_quota DESC, t.remain_quota DESC, t.id DESC
    LIMIT 1
    '''
)
row = cur.fetchone()
if row:
    result["token"] = row[0] or ""

cur.execute(
    '''
    SELECT c.id, a.model
    FROM abilities a
    JOIN channels c ON c.id = a.channel_id
    WHERE a.enabled = 1
      AND c.status = 1
      AND a.`group` = 'default'
      AND (
        LOWER(a.model) LIKE 'gpt-5%%'
        OR LOWER(a.model) LIKE 'gpt-4.1%%'
        OR LOWER(a.model) LIKE 'o3%%'
        OR LOWER(a.model) LIKE 'o4%%'
      )
    ORDER BY
      CASE
        WHEN LOWER(a.model) LIKE 'gpt-5%%' THEN 0
        WHEN LOWER(a.model) LIKE 'gpt-4.1%%' THEN 1
        WHEN LOWER(a.model) LIKE 'o3%%' THEN 2
        WHEN LOWER(a.model) LIKE 'o4%%' THEN 3
        ELSE 9
      END,
      c.priority DESC,
      c.id DESC
    LIMIT 1
    '''
)
row = cur.fetchone()
if row:
    result["channel_id"] = int(row[0])
    result["model"] = row[1] or ""

cur.close()
conn.close()
print(json.dumps(result))
"""
    payload = run_ssh_python(defaults.ssh_target, remote_script)
    return TargetDefaults(
        target_env=defaults.target_env,
        base_url=defaults.base_url,
        ssh_target=defaults.ssh_target,
        token=str(payload.get("token") or ""),
        model=str(payload.get("model") or ""),
        channel_id=payload.get("channel_id"),
    )


def resolve_defaults(args: argparse.Namespace) -> TargetDefaults:
    discovered = discover_target_defaults(args.target_env)
    return TargetDefaults(
        target_env=args.target_env,
        base_url=args.base_url or env("FYAPI_E2E_BASE_URL") or discovered.base_url,
        ssh_target=discovered.ssh_target,
        token=args.token or env("FYAPI_E2E_TOKEN") or discovered.token,
        model=args.model or env("FYAPI_E2E_RESPONSES_MODEL") or discovered.model,
        channel_id=args.channel_id or int(env("FYAPI_E2E_CHANNEL_ID", "0") or "0") or discovered.channel_id,
    )


def short_body(resp: httpx.Response) -> str:
    text = resp.text.strip()
    if len(text) > 500:
        return text[:500] + "..."
    return text


def parse_json(resp: httpx.Response) -> dict[str, Any]:
    try:
        payload = resp.json()
    except json.JSONDecodeError:
        return {}
    return payload if isinstance(payload, dict) else {}


def post_json(client: httpx.Client, path: str, body: dict[str, Any]) -> httpx.Response:
    return client.post(path, json=body)


def expect_responses_success(resp: httpx.Response) -> tuple[bool, str]:
    payload = parse_json(resp)
    if resp.status_code >= 500:
        return False, f"http {resp.status_code}: {short_body(resp)}"
    if payload.get("error"):
        return False, f"http {resp.status_code}: {payload['error']}"
    if payload.get("status") != "completed":
        return False, f"http {resp.status_code}: unexpected status {payload.get('status')!r}"
    return True, f"http {resp.status_code}"


def check_metadata_sanitized(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/responses", {
        "model": model,
        "input": [
            {
                "role": "user",
                "metadata": {"conversation_id": "trace-e2e-metadata"},
                "internal_chat_message_metadata_passthrough": {"foo": "bar"},
                "content": [{"type": "input_text", "text": "Reply with OK only."}],
            }
        ],
        "max_output_tokens": 32,
    })
    ok, detail = expect_responses_success(resp)
    if not ok and "Unknown parameter" in resp.text:
        detail = f"{detail}; metadata sanitization failed"
    return CaseResult(
        "responses_metadata_sanitized",
        ok,
        detail,
        resp.status_code,
        resp.headers.get("X-Oneapi-Request-Id", ""),
    )


def check_encrypted_content_retry(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/responses", {
        "model": model,
        "input": [
            {
                "role": "user",
                "content": [{"type": "input_text", "text": "Reply with OK only."}],
            },
            {
                "type": "reasoning",
                "summary": [],
                "encrypted_content": "gAAA.invalid.trace.e2e",
            },
            {
                "role": "assistant",
                "content": [{"type": "output_text", "text": "context"}],
            },
        ],
        "max_output_tokens": 32,
    })
    ok, detail = expect_responses_success(resp)
    if not ok and "could not be verified" in resp.text:
        detail = f"{detail}; encrypted_content retry did not recover"
    return CaseResult(
        "responses_encrypted_content_retry",
        ok,
        detail,
        resp.status_code,
        resp.headers.get("X-Oneapi-Request-Id", ""),
    )


def verify_retry_log(ssh_target: str, request_id: str) -> tuple[bool, str]:
    if not request_id:
        return False, "missing X-Oneapi-Request-Id header"

    for _ in range(10):
        proc = run_ssh_command(
            ssh_target,
            "container=$(podman ps --format '{{.Names}}' | grep -E '^fy-api-(blue|green)$' | head -1) && "
            "test -n \"$container\" && "
            "podman logs --since 15m \"$container\" 2>&1 | "
            f"grep -F \"{request_id}\"",
        )
        output = (proc.stdout or "") + (proc.stderr or "")
        if proc.returncode == 0 and (
            "retrying responses request without encrypted_content after upstream verification failure" in output
        ):
            return True, "retry log found"
        time.sleep(2)
    return False, "retry log not found"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="")
    parser.add_argument("--token", default="")
    parser.add_argument("--model", default="")
    parser.add_argument("--channel-id", type=int, default=None)
    parser.add_argument("--target-env", choices=sorted(TARGET_DEFAULTS.keys()), default="hk-test")
    parser.add_argument("--timeout", type=int, default=int(env("FYAPI_E2E_TIMEOUT", "90")))
    parser.add_argument("--skip-log-check", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    try:
        defaults = resolve_defaults(args)
    except Exception as exc:
        print(f"failed to discover target defaults: {exc}", file=sys.stderr)
        return 2

    args.base_url = defaults.base_url
    args.token = defaults.token
    args.model = defaults.model
    args.channel_id = defaults.channel_id

    if args.dry_run:
        print("cases:")
        print("  - responses_metadata_sanitized")
        print("  - responses_encrypted_content_retry")
        print(json.dumps({
            "target_env": defaults.target_env,
            "base_url": args.base_url or "<required>",
            "token": mask_token(args.token),
            "model": args.model,
            "channel_id": args.channel_id,
            "skip_log_check": args.skip_log_check,
        }, ensure_ascii=False, indent=2))
        return 0

    if not args.base_url or not args.token or not args.model:
        print("FYAPI_E2E_BASE_URL, FYAPI_E2E_TOKEN and FYAPI_E2E_RESPONSES_MODEL are required", file=sys.stderr)
        return 2

    results: list[CaseResult] = []
    with httpx.Client(
        base_url=args.base_url.rstrip("/"),
        headers=auth_headers(args.token, args.channel_id),
        timeout=args.timeout,
    ) as client:
        results.append(check_metadata_sanitized(client, args.model))
        encrypted_retry = check_encrypted_content_retry(client, args.model)
        if encrypted_retry.ok and not args.skip_log_check:
            log_ok, log_detail = verify_retry_log(defaults.ssh_target, encrypted_retry.request_id)
            encrypted_retry.ok = encrypted_retry.ok and log_ok
            encrypted_retry.detail = f"{encrypted_retry.detail}; {log_detail}"
        results.append(encrypted_retry)

    for result in results:
        status = "PASS" if result.ok else "FAIL"
        request_id_suffix = f" request_id={result.request_id}" if result.request_id else ""
        print(f"{status} {result.name}: {result.detail}{request_id_suffix}")
    return 0 if all(result.ok for result in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
