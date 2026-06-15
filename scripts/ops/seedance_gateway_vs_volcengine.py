#!/usr/bin/env python3
"""
Compare Seedance 2.0 requests through TraceNex and direct VolcEngine.

Gateway mode accepts the public TraceNex /v1/videos request shape.
VolcEngine mode accepts the native /api/v3/contents/generations/tasks shape.

Required env:
  Gateway: FYAPI_E2E_BASE_URL, FYAPI_E2E_TOKEN
  Direct:  VOLCENGINE_ARK_BASE_URL, VOLCENGINE_ARK_API_KEY
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
from pathlib import Path
from typing import Any

import httpx


TERMINAL_GATEWAY = {"completed", "succeeded", "success", "failed", "failure"}
TERMINAL_VOLC = {"succeeded", "success", "completed", "failed", "failure"}


def env(name: str, default: str = "") -> str:
    return os.getenv(name, default).strip()


def load_json(path: str) -> dict[str, Any]:
    payload = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return payload


def summarize(payload: Any) -> str:
    text = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    return text[:2000]


def post_json(client: httpx.Client, path: str, body: dict[str, Any]) -> dict[str, Any]:
    resp = client.post(path, json=body)
    if resp.status_code >= 400:
        raise RuntimeError(f"POST {path} HTTP {resp.status_code}: {resp.text[:2000]}")
    payload = resp.json()
    if isinstance(payload, dict) and payload.get("error"):
        raise RuntimeError(f"POST {path} error: {summarize(payload['error'])}")
    if not isinstance(payload, dict):
        raise RuntimeError(f"POST {path} returned non-object JSON: {summarize(payload)}")
    return payload


def get_json(client: httpx.Client, path: str) -> dict[str, Any]:
    resp = client.get(path)
    if resp.status_code >= 400:
        raise RuntimeError(f"GET {path} HTTP {resp.status_code}: {resp.text[:2000]}")
    payload = resp.json()
    if not isinstance(payload, dict):
        raise RuntimeError(f"GET {path} returned non-object JSON: {summarize(payload)}")
    return payload


def run_gateway(args: argparse.Namespace, body: dict[str, Any]) -> int:
    if not args.base_url or not args.token:
        print("gateway mode requires FYAPI_E2E_BASE_URL and FYAPI_E2E_TOKEN", file=sys.stderr)
        return 2

    headers = {"Authorization": f"Bearer {args.token}", "Content-Type": "application/json"}
    with httpx.Client(base_url=args.base_url.rstrip("/"), headers=headers, timeout=60) as client:
        submitted = post_json(client, "/v1/videos", body)
        task_id = submitted.get("id") or submitted.get("task_id")
        if not isinstance(task_id, str) or not task_id:
            raise RuntimeError(f"gateway submit response missing task id: {summarize(submitted)}")
        print(f"gateway task_id={task_id}")

        if args.no_poll:
            print(summarize(submitted))
            return 0

        final = poll(
            lambda: normalize_gateway(get_json(client, f"/v1/videos/{task_id}")),
            timeout_s=args.timeout,
            interval_s=args.interval,
            terminal=TERMINAL_GATEWAY,
        )
        print(summarize(final))
        return 0 if str(final.get("status") or "").lower() not in {"failed", "failure"} else 1


def normalize_gateway(payload: dict[str, Any]) -> dict[str, Any]:
    if isinstance(payload.get("data"), dict):
        return payload["data"]
    return payload


def run_volcengine(args: argparse.Namespace, body: dict[str, Any]) -> int:
    if not args.volc_base_url or not args.volc_api_key:
        print("volcengine mode requires VOLCENGINE_ARK_BASE_URL and VOLCENGINE_ARK_API_KEY", file=sys.stderr)
        return 2

    headers = {"Authorization": f"Bearer {args.volc_api_key}", "Content-Type": "application/json", "Accept": "application/json"}
    with httpx.Client(base_url=args.volc_base_url.rstrip("/"), headers=headers, timeout=60) as client:
        submitted = post_json(client, "/api/v3/contents/generations/tasks", body)
        task_id = submitted.get("id") or submitted.get("task_id")
        if not isinstance(task_id, str) or not task_id:
            raise RuntimeError(f"volcengine submit response missing task id: {summarize(submitted)}")
        print(f"volcengine task_id={task_id}")

        if args.no_poll:
            print(summarize(submitted))
            return 0

        final = poll(
            lambda: get_json(client, f"/api/v3/contents/generations/tasks/{task_id}"),
            timeout_s=args.timeout,
            interval_s=args.interval,
            terminal=TERMINAL_VOLC,
        )
        print(summarize(final))
        return 0 if str(final.get("status") or "").lower() not in {"failed", "failure"} else 1


def poll(fetch: Any, *, timeout_s: int, interval_s: int, terminal: set[str]) -> dict[str, Any]:
    deadline = time.time() + timeout_s
    last: dict[str, Any] = {}
    while time.time() < deadline:
        last = fetch()
        status = str(last.get("status") or "").lower()
        print(f"status={status or '<missing>'}")
        if status in terminal:
            return last
        time.sleep(interval_s)
    raise TimeoutError(f"task not terminal before timeout; last={summarize(last)}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=["gateway", "volcengine"], required=True)
    parser.add_argument("--payload", required=True, help="JSON request body file for the selected mode")
    parser.add_argument("--base-url", default=env("FYAPI_E2E_BASE_URL"))
    parser.add_argument("--token", default=env("FYAPI_E2E_TOKEN"))
    parser.add_argument("--volc-base-url", default=env("VOLCENGINE_ARK_BASE_URL", "https://ark.cn-beijing.volces.com"))
    parser.add_argument("--volc-api-key", default=env("VOLCENGINE_ARK_API_KEY"))
    parser.add_argument("--timeout", type=int, default=int(env("SEEDANCE_E2E_TIMEOUT", "900")))
    parser.add_argument("--interval", type=int, default=int(env("SEEDANCE_E2E_INTERVAL", "10")))
    parser.add_argument("--no-poll", action="store_true")
    args = parser.parse_args()

    body = load_json(args.payload)
    if args.mode == "gateway":
        return run_gateway(args, body)
    return run_volcengine(args, body)


if __name__ == "__main__":
    raise SystemExit(main())
