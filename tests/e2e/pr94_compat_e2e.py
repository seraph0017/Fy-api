#!/usr/bin/env python3
"""
PR #94 compatibility e2e checks.

Validates two regression-prone areas after upstream sync:
1. AWS Bedrock Claude request field pointer compatibility
2. Claude tool call compatibility when function.arguments is empty

Required env:
  FYAPI_E2E_BASE_URL
  FYAPI_E2E_TOKEN
  FYAPI_E2E_BEDROCK_MODEL
  FYAPI_E2E_CLAUDE_MODEL

Optional env:
  FYAPI_E2E_TIMEOUT   default: 90
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from dataclasses import asdict, dataclass
from typing import Any

import httpx


@dataclass
class CaseResult:
    name: str
    ok: bool
    detail: str
    status_code: int


@dataclass
class TargetDefaults:
    target_env: str
    base_url: str
    ssh_target: str
    token: str = ""
    bedrock_model: str = ""
    bedrock_channel_id: int | None = None
    claude_model: str = ""
    claude_channel_id: int | None = None


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


def auth_headers(token: str, pin_channel_id: int | None = None) -> dict[str, str]:
    effective_token = token if pin_channel_id is None else f"{token}-{pin_channel_id}"
    return {"Authorization": f"Bearer {effective_token}", "Content-Type": "application/json"}


def mask_token(token: str) -> str:
    if not token:
        return ""
    if len(token) <= 12:
        return token[:2] + "..." + token[-2:]
    return token[:6] + "..." + token[-4:]


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
    "bedrock_model": "",
    "bedrock_channel_id": None,
    "claude_model": "",
    "claude_channel_id": None,
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
      AND c.type = 33
      AND LOCATE('claude', LOWER(a.model)) > 0
    ORDER BY
      CASE
        WHEN LOCATE('sonnet', LOWER(a.model)) > 0 THEN 0
        WHEN LOCATE('opus', LOWER(a.model)) > 0 THEN 1
        WHEN LOCATE('haiku', LOWER(a.model)) > 0 THEN 2
        ELSE 9
      END,
      c.priority DESC,
      c.id DESC
    LIMIT 1
    '''
)
row = cur.fetchone()
if row:
    result["bedrock_channel_id"] = int(row[0])
    result["bedrock_model"] = row[1] or ""

cur.execute(
    '''
    SELECT c.id, a.model
    FROM abilities a
    JOIN channels c ON c.id = a.channel_id
    WHERE a.enabled = 1
      AND c.status = 1
      AND a.`group` = 'default'
      AND c.type <> 33
      AND LOCATE('claude', LOWER(a.model)) > 0
    ORDER BY
      CASE
        WHEN c.type = 14 THEN 0
        WHEN c.type = 1 THEN 1
        ELSE 9
      END,
      CASE
        WHEN LOCATE('sonnet', LOWER(a.model)) > 0 THEN 0
        WHEN LOCATE('opus', LOWER(a.model)) > 0 THEN 1
        WHEN LOCATE('haiku', LOWER(a.model)) > 0 THEN 2
        ELSE 9
      END,
      c.priority DESC,
      c.id DESC
    LIMIT 1
    '''
)
row = cur.fetchone()
if row:
    result["claude_channel_id"] = int(row[0])
    result["claude_model"] = row[1] or ""

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
        bedrock_model=str(payload.get("bedrock_model") or ""),
        bedrock_channel_id=payload.get("bedrock_channel_id"),
        claude_model=str(payload.get("claude_model") or ""),
        claude_channel_id=payload.get("claude_channel_id"),
    )


def resolve_defaults(args: argparse.Namespace) -> TargetDefaults:
    discovered = discover_target_defaults(args.target_env)
    return TargetDefaults(
        target_env=args.target_env,
        base_url=args.base_url or env("FYAPI_E2E_BASE_URL") or discovered.base_url,
        ssh_target=discovered.ssh_target,
        token=args.token or env("FYAPI_E2E_TOKEN") or discovered.token,
        bedrock_model=args.bedrock_model or env("FYAPI_E2E_BEDROCK_MODEL") or discovered.bedrock_model,
        bedrock_channel_id=args.bedrock_channel_id or discovered.bedrock_channel_id,
        claude_model=args.claude_model or env("FYAPI_E2E_CLAUDE_MODEL") or discovered.claude_model,
        claude_channel_id=args.claude_channel_id or discovered.claude_channel_id,
    )


def short_body(resp: httpx.Response) -> str:
    text = resp.text.strip()
    if len(text) > 500:
        return text[:500] + "..."
    return text


def post_json(
    client: httpx.Client,
    path: str,
    body: dict[str, Any],
    *,
    extra_headers: dict[str, str] | None = None,
) -> httpx.Response:
    headers = extra_headers or {}
    return client.post(path, json=body, headers=headers)


def parse_json(resp: httpx.Response) -> dict[str, Any]:
    try:
        payload = resp.json()
    except json.JSONDecodeError:
        return {}
    return payload if isinstance(payload, dict) else {}


def response_has_gateway_regression(resp: httpx.Response) -> bool:
    text = resp.text.lower()
    bad_markers = (
        "tool call function arguments",
        "convert request failed",
        "cannot unmarshal",
        "invalid character",
        "json:",
    )
    return any(marker in text for marker in bad_markers)


def expect_success_json(resp: httpx.Response) -> tuple[bool, str]:
    payload = parse_json(resp)
    if resp.status_code >= 500:
        return False, f"http {resp.status_code}: {short_body(resp)}"
    if payload.get("error"):
        return False, f"http {resp.status_code}: {payload['error']}"
    if "choices" not in payload and "content" not in payload:
        return False, f"http {resp.status_code}: unexpected payload {short_body(resp)}"
    return True, f"http {resp.status_code}"


def check_bedrock_top_p_only(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/messages", {
        "model": model,
        "max_tokens": 64,
        "top_p": 0.8,
        "messages": [{"role": "user", "content": "Reply with OK only."}],
    })
    ok, detail = expect_success_json(resp)
    return CaseResult("bedrock_top_p_only", ok, detail, resp.status_code)


def check_bedrock_temp_and_top_p(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/messages", {
        "model": model,
        "max_tokens": 64,
        "temperature": 0.7,
        "top_p": 0.8,
        "messages": [{"role": "user", "content": "Reply with OK only."}],
    })
    ok, detail = expect_success_json(resp)
    return CaseResult("bedrock_temp_and_top_p", ok, detail, resp.status_code)


def check_bedrock_temp_clamp(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/messages", {
        "model": model,
        "max_tokens": 64,
        "temperature": 2,
        "messages": [{"role": "user", "content": "Reply with OK only."}],
    })
    ok, detail = expect_success_json(resp)
    return CaseResult("bedrock_temp_clamp", ok, detail, resp.status_code)


def check_bedrock_defaults(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/messages", {
        "model": model,
        "max_tokens": 64,
        "messages": [{"role": "user", "content": "Reply with OK only."}],
    })
    ok, detail = expect_success_json(resp)
    return CaseResult("bedrock_defaults", ok, detail, resp.status_code)


def _claude_tool_body(model: str, arguments: str) -> dict[str, Any]:
    return {
        "model": model,
        "messages": [
            {"role": "user", "content": "call the noop tool"},
            {
                "role": "assistant",
                "tool_calls": [
                    {
                        "id": "call_1",
                        "type": "function",
                        "function": {
                            "name": "noop",
                            "arguments": arguments,
                        },
                    }
                ],
            },
            {
                "role": "tool",
                "tool_call_id": "call_1",
                "content": "ok",
            },
        ],
        "tools": [
            {
                "type": "function",
                "function": {
                    "name": "noop",
                    "description": "do nothing",
                    "parameters": {
                        "type": "object",
                        "properties": {},
                    },
                },
            }
        ],
    }


def check_claude_empty_arguments(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/chat/completions", _claude_tool_body(model, ""))
    if resp.status_code >= 500:
        return CaseResult(
            "claude_tool_empty_arguments",
            False,
            f"http {resp.status_code}: {short_body(resp)}",
            resp.status_code,
        )
    if response_has_gateway_regression(resp):
        return CaseResult(
            "claude_tool_empty_arguments",
            False,
            f"http {resp.status_code}: regression marker in response {short_body(resp)}",
            resp.status_code,
        )
    return CaseResult(
        "claude_tool_empty_arguments",
        True,
        f"http {resp.status_code}",
        resp.status_code,
    )


def check_claude_object_arguments(client: httpx.Client, model: str) -> CaseResult:
    resp = post_json(client, "/v1/chat/completions", _claude_tool_body(model, "{}"))
    if resp.status_code >= 500:
        return CaseResult(
            "claude_tool_object_arguments",
            False,
            f"http {resp.status_code}: {short_body(resp)}",
            resp.status_code,
        )
    if response_has_gateway_regression(resp):
        return CaseResult(
            "claude_tool_object_arguments",
            False,
            f"http {resp.status_code}: regression marker in response {short_body(resp)}",
            resp.status_code,
        )
    return CaseResult(
        "claude_tool_object_arguments",
        True,
        f"http {resp.status_code}",
        resp.status_code,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="")
    parser.add_argument("--token", default="")
    parser.add_argument("--bedrock-model", default="")
    parser.add_argument("--claude-model", default="")
    parser.add_argument("--bedrock-channel-id", type=int, default=None)
    parser.add_argument("--claude-channel-id", type=int, default=None)
    parser.add_argument("--target-env", choices=sorted(TARGET_DEFAULTS.keys()), default="hk-test")
    parser.add_argument("--timeout", type=int, default=int(env("FYAPI_E2E_TIMEOUT", "90")))
    parser.add_argument("--only", choices=["all", "bedrock", "claude"], default="all")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    try:
        defaults = resolve_defaults(args)
    except Exception as exc:
        print(f"failed to discover target defaults: {exc}", file=sys.stderr)
        return 2

    args.base_url = defaults.base_url
    args.token = defaults.token
    args.bedrock_model = defaults.bedrock_model
    args.bedrock_channel_id = defaults.bedrock_channel_id
    args.claude_model = defaults.claude_model
    args.claude_channel_id = defaults.claude_channel_id

    if args.dry_run:
        print("cases:")
        print("  - bedrock_top_p_only")
        print("  - bedrock_temp_and_top_p")
        print("  - bedrock_temp_clamp")
        print("  - bedrock_defaults")
        print("  - claude_tool_empty_arguments")
        print("  - claude_tool_object_arguments")
        print(json.dumps({
            "target_env": defaults.target_env,
            "base_url": args.base_url or "<required>",
            "token": mask_token(args.token),
            "bedrock_model": args.bedrock_model,
            "bedrock_channel_id": args.bedrock_channel_id,
            "claude_model": args.claude_model,
            "claude_channel_id": args.claude_channel_id,
            "only": args.only,
        }, ensure_ascii=False, indent=2))
        return 0

    if not args.base_url or not args.token:
        print("FYAPI_E2E_BASE_URL and FYAPI_E2E_TOKEN are required", file=sys.stderr)
        return 2
    if args.only in {"all", "bedrock"} and (not args.bedrock_model or args.bedrock_channel_id is None):
        print("Bedrock model and channel id are required", file=sys.stderr)
        return 2
    if args.only in {"all", "claude"} and (not args.claude_model or args.claude_channel_id is None):
        print("Claude model and channel id are required", file=sys.stderr)
        return 2

    results: list[CaseResult] = []
    if args.only in {"all", "bedrock"}:
        with httpx.Client(
            base_url=args.base_url.rstrip("/"),
            headers=auth_headers(args.token, args.bedrock_channel_id),
            timeout=args.timeout,
        ) as client:
            results.append(check_bedrock_top_p_only(client, args.bedrock_model))
            results.append(check_bedrock_temp_and_top_p(client, args.bedrock_model))
            results.append(check_bedrock_temp_clamp(client, args.bedrock_model))
            results.append(check_bedrock_defaults(client, args.bedrock_model))
    if args.only in {"all", "claude"}:
        with httpx.Client(
            base_url=args.base_url.rstrip("/"),
            headers=auth_headers(args.token, args.claude_channel_id),
            timeout=args.timeout,
        ) as client:
            results.append(check_claude_empty_arguments(client, args.claude_model))
            results.append(check_claude_object_arguments(client, args.claude_model))

    for result in results:
        status = "PASS" if result.ok else "FAIL"
        print(f"{status} {result.name}: {result.detail}")
    return 0 if all(result.ok for result in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
