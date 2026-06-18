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
import sys
from dataclasses import dataclass
from typing import Any

import httpx


@dataclass
class CaseResult:
    name: str
    ok: bool
    detail: str
    status_code: int


def env(name: str, default: str = "") -> str:
    return os.getenv(name, default).strip()


def auth_headers(token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}


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
    parser.add_argument("--base-url", default=env("FYAPI_E2E_BASE_URL"))
    parser.add_argument("--token", default=env("FYAPI_E2E_TOKEN"))
    parser.add_argument("--bedrock-model", default=env("FYAPI_E2E_BEDROCK_MODEL"))
    parser.add_argument("--claude-model", default=env("FYAPI_E2E_CLAUDE_MODEL"))
    parser.add_argument("--timeout", type=int, default=int(env("FYAPI_E2E_TIMEOUT", "90")))
    parser.add_argument("--only", choices=["all", "bedrock", "claude"], default="all")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    if args.dry_run:
        print("cases:")
        print("  - bedrock_top_p_only")
        print("  - bedrock_temp_and_top_p")
        print("  - bedrock_temp_clamp")
        print("  - bedrock_defaults")
        print("  - claude_tool_empty_arguments")
        print("  - claude_tool_object_arguments")
        print(f"base_url={args.base_url or '<required>'}")
        print(f"only={args.only}")
        return 0

    if not args.base_url or not args.token:
        print("FYAPI_E2E_BASE_URL and FYAPI_E2E_TOKEN are required", file=sys.stderr)
        return 2
    if args.only in {"all", "bedrock"} and not args.bedrock_model:
        print("FYAPI_E2E_BEDROCK_MODEL is required", file=sys.stderr)
        return 2
    if args.only in {"all", "claude"} and not args.claude_model:
        print("FYAPI_E2E_CLAUDE_MODEL is required", file=sys.stderr)
        return 2

    results: list[CaseResult] = []
    with httpx.Client(
        base_url=args.base_url.rstrip("/"),
        headers=auth_headers(args.token),
        timeout=args.timeout,
    ) as client:
        if args.only in {"all", "bedrock"}:
            results.append(check_bedrock_top_p_only(client, args.bedrock_model))
            results.append(check_bedrock_temp_and_top_p(client, args.bedrock_model))
            results.append(check_bedrock_temp_clamp(client, args.bedrock_model))
            results.append(check_bedrock_defaults(client, args.bedrock_model))
        if args.only in {"all", "claude"}:
            results.append(check_claude_empty_arguments(client, args.claude_model))
            results.append(check_claude_object_arguments(client, args.claude_model))

    for result in results:
        status = "PASS" if result.ok else "FAIL"
        print(f"{status} {result.name}: {result.detail}")
    return 0 if all(result.ok for result in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
