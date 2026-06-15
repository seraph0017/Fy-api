#!/usr/bin/env python3
"""
Run a Seedance 2.0 gateway e2e matrix against /v1/videos.

The default matrix covers:
  - pipeline hits: 1080p no-reference landscape, no-reference portrait,
    single-image, multi-image, storyboard/content-reference requests
  - direct misses: the same 1080p requests with an explicit strategy bypass

Each case writes payload/submit/poll/final/download artifacts under /tmp by
default. Downloaded videos are probed with ffprobe to verify dimensions and
duration. When --db-check-ssh is provided, the script also verifies task
private_data so hit/direct assertions are not based on public response shape.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any


TERMINAL = {"completed", "succeeded", "success", "failed", "failure"}
PIPELINE_NAME = "seedance2_720p_mediakit_1080p"
DEFAULT_IMAGE_URLS = [
    "https://raw.githubusercontent.com/opencv/opencv/master/samples/data/lena.jpg",
    "https://raw.githubusercontent.com/opencv/opencv/master/samples/data/baboon.jpg",
]


@dataclass(frozen=True)
class Case:
    name: str
    expected_pipeline: bool
    expected_refs: int
    expected_width: int
    expected_height: int
    expected_duration: float
    payload: dict[str, Any]


def env(name: str, default: str = "") -> str:
    return os.getenv(name, default).strip()


def normalize_token(token: str) -> str:
    token = token.strip()
    if not token:
        return ""
    if token.startswith("sk-"):
        return token
    return "sk-" + token


def build_cases(image_urls: list[str]) -> list[Case]:
    prompt = (
        "A calm cinematic portrait video, subtle natural movement, gentle camera "
        "push in, soft daylight, realistic texture, no text, no watermark."
    )

    def metadata(*, hit: bool, extra: dict[str, Any] | None = None) -> dict[str, Any]:
        data: dict[str, Any] = {"fy_enhance_force": True} if hit else {"fy_enhance_bypass": True}
        if extra:
            data.update(extra)
        return data

    def payload(
        *,
        hit: bool,
        size: str = "1920x1080",
        refs: int = 0,
        prompt_text: str = prompt,
        extra_metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {
            "model": "doubao-seedance-2-0-260128",
            "prompt": prompt_text,
            "size": size,
            "seconds": "5",
            "watermark": False,
            "metadata": metadata(hit=hit, extra=extra_metadata),
        }
        if refs == 1:
            body["input_reference"] = image_urls[0]
        elif refs == 2:
            body["images"] = image_urls[:2]
        return body

    storyboard_prompt = (
        "Storyboard with three shots: shot 1 wide establishing view, shot 2 slow "
        "camera push toward the person, shot 3 close-up with gentle expression. "
        "Keep continuity, cinematic daylight, no text, no watermark."
    )
    storyboard_metadata = {
        "storyboard": True,
        "shot_count": 3,
        "content": [
            {"type": "image_url", "image_url": {"url": image_urls[0]}},
            {"type": "image_url", "image_url": {"url": image_urls[1]}},
        ],
    }

    base: list[tuple[str, int, int, int, dict[str, Any]]] = [
        ("no_ref_landscape", 0, 1920, 1080, {"size": "1920x1080"}),
        ("no_ref_portrait", 0, 1080, 1920, {"size": "1080x1920"}),
        ("single_image", 1, 1920, 1080, {}),
        ("multi_image", 2, 1920, 1080, {}),
        (
            "storyboard_content",
            2,
            1920,
            1080,
            {"prompt_text": storyboard_prompt, "extra_metadata": storyboard_metadata},
        ),
    ]

    cases: list[Case] = []
    for suffix, refs, width, height, kwargs in base:
        cases.append(Case(f"hit_{suffix}_1080p", True, refs, width, height, 5.0, payload(hit=True, refs=refs, **kwargs)))
    for suffix, refs, width, height, kwargs in base:
        cases.append(Case(f"direct_{suffix}_1080p", False, refs, width, height, 5.0, payload(hit=False, refs=refs, **kwargs)))
    return cases


def save_json(path: Path, payload: Any) -> None:
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")


def summarize(payload: Any) -> str:
    return json.dumps(payload, ensure_ascii=False, separators=(",", ":"))[:2000]


def request_json(base_url: str, method: str, path: str, token: str, body: dict[str, Any] | None = None, timeout: int = 90) -> dict[str, Any]:
    url = base_url.rstrip("/") + path
    data = None if body is None else json.dumps(body, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
            status = resp.status
            headers = dict(resp.headers.items())
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        raise RuntimeError(f"{method} {path} HTTP {exc.code}: {raw.decode('utf-8', 'replace')[:2000]}") from exc
    if status >= 400:
        raise RuntimeError(f"{method} {path} HTTP {status}: {raw.decode('utf-8', 'replace')[:2000]}")
    payload = json.loads(raw.decode("utf-8"))
    if not isinstance(payload, dict):
        raise RuntimeError(f"{method} {path} returned non-object JSON: {summarize(payload)}")
    payload["_http"] = {"status_code": status, "headers": headers}
    return payload


def request_bytes(base_url: str, path: str, token: str, timeout: int = 180) -> tuple[bytes, dict[str, Any]]:
    url = base_url.rstrip("/") + path
    req = urllib.request.Request(url, method="GET")
    req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.read(), {"status_code": resp.status, "headers": dict(resp.headers.items())}
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        raise RuntimeError(f"GET {path} HTTP {exc.code}: {raw.decode('utf-8', 'replace')[:2000]}") from exc


def public_status(payload: dict[str, Any]) -> dict[str, Any]:
    data = payload.get("data")
    if isinstance(data, dict):
        return data
    return payload


def run_case(args: argparse.Namespace, case: Case, token: str, run_dir: Path) -> dict[str, Any]:
    case_dir = run_dir / case.name
    case_dir.mkdir(parents=True, exist_ok=True)
    save_json(case_dir / "payload.json", case.payload)

    submitted = request_json(args.base_url, "POST", "/v1/videos", token, case.payload, timeout=args.request_timeout)
    save_json(case_dir / "submit_response.json", submitted)
    task_id = submitted.get("id") or submitted.get("task_id")
    if not isinstance(task_id, str) or not task_id:
        raise RuntimeError(f"{case.name}: submit response missing task id: {summarize(submitted)}")
    print(f"[{case.name}] task_id={task_id}", flush=True)

    final: dict[str, Any] | None = None
    deadline = time.time() + args.timeout
    poll_count = 0
    while time.time() < deadline:
        poll_count += 1
        polled = request_json(args.base_url, "GET", f"/v1/videos/{task_id}", token, timeout=args.request_timeout)
        save_json(case_dir / f"poll_{poll_count:03d}.json", polled)
        status_obj = public_status(polled)
        status = str(status_obj.get("status") or "").lower()
        progress = status_obj.get("progress")
        print(f"[{case.name}] poll={poll_count} status={status or '<missing>'} progress={progress}", flush=True)
        if status in TERMINAL:
            final = polled
            save_json(case_dir / "final_response.json", polled)
            if status in {"failed", "failure"}:
                raise RuntimeError(f"{case.name}: task failed: {summarize(polled)}")
            break
        time.sleep(args.interval)
    if final is None:
        raise TimeoutError(f"{case.name}: task did not finish before timeout")

    video, meta = request_bytes(args.base_url, f"/v1/videos/{task_id}/content", token, timeout=args.download_timeout)
    save_json(case_dir / "download_response_meta.json", meta)
    content_type = str(meta.get("headers", {}).get("Content-Type") or meta.get("headers", {}).get("content-type") or "")
    ext = ".webm" if "webm" in content_type.lower() else ".mp4"
    video_path = case_dir / f"result{ext}"
    video_path.write_bytes(video)
    print(f"[{case.name}] video={video_path} bytes={video_path.stat().st_size}", flush=True)
    probe = probe_video(video_path)
    save_json(case_dir / "ffprobe.json", probe)
    assert_video(case, probe)
    print(
        f"[{case.name}] ffprobe={probe.get('width')}x{probe.get('height')} "
        f"duration={probe.get('duration')}",
        flush=True,
    )

    db = None
    if args.db_check_ssh:
        db = query_task_private_data(args, task_id)
        save_json(case_dir / "db_private_data_check.json", db)
        assert_db(case, db)
        print(
            f"[{case.name}] db pipeline={db.get('pipeline') or '<none>'} "
            f"pipeline_status={db.get('pipeline_status') or '<none>'} "
            f"asset_cleanup={asset_cleanup_summary(db)}",
            flush=True,
        )

    return {
        "case": case.name,
        "task_id": task_id,
        "expected_pipeline": case.expected_pipeline,
        "expected_refs": case.expected_refs,
        "expected_width": case.expected_width,
        "expected_height": case.expected_height,
        "expected_duration": case.expected_duration,
        "video_path": str(video_path),
        "video_bytes": video_path.stat().st_size,
        "ffprobe": probe,
        "db": db,
    }


def probe_video(path: Path) -> dict[str, Any]:
    cmd = [
        "ffprobe",
        "-v",
        "error",
        "-select_streams",
        "v:0",
        "-show_entries",
        "stream=width,height,duration:format=duration",
        "-of",
        "json",
        str(path),
    ]
    raw = subprocess.check_output(cmd, text=True)
    payload = json.loads(raw)
    streams = payload.get("streams") or []
    if not streams:
        raise RuntimeError(f"ffprobe found no video stream in {path}")
    stream = streams[0]
    duration = stream.get("duration") or (payload.get("format") or {}).get("duration") or 0
    return {
        "width": int(stream.get("width") or 0),
        "height": int(stream.get("height") or 0),
        "duration": float(duration or 0),
    }


def assert_video(case: Case, probe: dict[str, Any]) -> None:
    width = int(probe.get("width") or 0)
    height = int(probe.get("height") or 0)
    if (width, height) != (case.expected_width, case.expected_height):
        raise AssertionError(
            f"{case.name}: expected {case.expected_width}x{case.expected_height}, got {width}x{height}"
        )
    duration = float(probe.get("duration") or 0)
    if not (case.expected_duration - 1.5 <= duration <= case.expected_duration + 3.0):
        raise AssertionError(f"{case.name}: expected duration around {case.expected_duration}s, got {duration}s")


def query_task_private_data(args: argparse.Namespace, task_id: str) -> dict[str, Any]:
    if not re.fullmatch(r"[A-Za-z0-9_-]+", task_id):
        raise ValueError(f"unsafe task id: {task_id!r}")
    remote = f"""
python3 - <<'PY'
import json, re, subprocess, sys
task_id = {task_id!r}
env_file = {args.remote_env_file!r}
env = {{}}
for line in open(env_file):
    line = line.strip()
    if line and not line.startswith('#') and '=' in line:
        k, v = line.split('=', 1)
        env[k] = v.strip().strip('"').strip("'")
m = re.match(r'([^:]+):([^@]+)@tcp\\(([^:)]+):(\\d+)\\)/([^?]+)', env.get('SQL_DSN', ''))
if not m:
    raise SystemExit('bad SQL_DSN')
user, pw, host, port, db = m.groups()
q = f'''
SELECT
 status,
 progress,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.upstream_task_id')) AS upstream_task_id,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_enhance.pipeline')) AS pipeline,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_enhance.status')) AS pipeline_status,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_enhance.generation_resolution')) AS generation_resolution,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_enhance.enhance_target_resolution')) AS enhance_target,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_enhance.generation_task_id')) AS generation_task_id,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_enhance.enhance_task_id')) AS enhance_task_id,
 JSON_LENGTH(JSON_EXTRACT(private_data,'$.seedance_asset_prepare.references')) AS asset_ref_count,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_asset_prepare.references[0].cleanup_status')) AS cleanup0,
 JSON_UNQUOTE(JSON_EXTRACT(private_data,'$.seedance_asset_prepare.references[1].cleanup_status')) AS cleanup1
FROM tasks WHERE task_id={{task_id!r}};
'''
cmd = ['mysql', f'-h{{host}}', f'-P{{port}}', f'-u{{user}}', f'-p{{pw}}', '--batch', '--raw', db, '-e', q]
res = subprocess.run(cmd, text=True, capture_output=True)
if res.returncode != 0:
    print(res.stderr, file=sys.stderr)
    raise SystemExit(res.returncode)
lines = [line for line in res.stdout.splitlines() if line.strip()]
if len(lines) < 2:
    raise SystemExit('task row not found')
headers = lines[0].split('\\t')
values = lines[1].split('\\t')
row = dict(zip(headers, values))
for key, value in list(row.items()):
    if value == 'NULL':
        row[key] = ''
print(json.dumps(row, ensure_ascii=False))
PY
"""
    cmd = ["ssh", "-p", str(args.db_check_ssh_port), "-o", "BatchMode=yes", args.db_check_ssh, remote]
    raw = subprocess.check_output(cmd, text=True)
    return json.loads(raw.strip().splitlines()[-1])


def asset_cleanup_summary(db: dict[str, Any]) -> str:
    count = int(db.get("asset_ref_count") or 0)
    if count <= 0:
        return "none"
    return ",".join(str(db.get(f"cleanup{i}") or "") for i in range(count))


def assert_db(case: Case, db: dict[str, Any]) -> None:
    pipeline = db.get("pipeline") or ""
    if case.expected_pipeline:
        if pipeline != PIPELINE_NAME:
            raise AssertionError(f"{case.name}: expected pipeline {PIPELINE_NAME}, got {pipeline or '<none>'}")
        if db.get("pipeline_status") != "enhance_succeeded":
            raise AssertionError(f"{case.name}: expected enhance_succeeded, got {db.get('pipeline_status')}")
        if db.get("generation_resolution") != "720p" or db.get("enhance_target") != "1080p":
            raise AssertionError(f"{case.name}: unexpected pipeline resolutions: {db}")
        generation_task_id = db.get("generation_task_id") or ""
        if not generation_task_id.startswith("cgt-"):
            raise AssertionError(f"{case.name}: generation_task_id was not real upstream id: {generation_task_id}")
    else:
        if pipeline:
            raise AssertionError(f"{case.name}: expected direct generation, got pipeline {pipeline}")

    ref_count = int(db.get("asset_ref_count") or 0)
    if ref_count != case.expected_refs:
        raise AssertionError(f"{case.name}: expected {case.expected_refs} asset refs, got {ref_count}")
    for i in range(case.expected_refs):
        if db.get(f"cleanup{i}") != "deleted":
            raise AssertionError(f"{case.name}: expected cleanup{i}=deleted, got {db.get(f'cleanup{i}')!r}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=env("FYAPI_E2E_BASE_URL", "https://api-test.tracenex.cn"))
    parser.add_argument("--token", default=env("FYAPI_E2E_TOKEN"))
    parser.add_argument("--output-root", default="/tmp")
    parser.add_argument("--timeout", type=int, default=int(env("SEEDANCE_E2E_TIMEOUT", "1800")))
    parser.add_argument("--interval", type=int, default=int(env("SEEDANCE_E2E_INTERVAL", "15")))
    parser.add_argument("--request-timeout", type=int, default=90)
    parser.add_argument("--download-timeout", type=int, default=180)
    parser.add_argument("--case", action="append", dest="cases", help="case name to run; repeatable")
    parser.add_argument("--image-url", action="append", dest="image_urls", help="reference image URL; repeat for multi image")
    parser.add_argument("--db-check-ssh", default=env("FYAPI_E2E_DB_CHECK_SSH"))
    parser.add_argument("--db-check-ssh-port", type=int, default=int(env("FYAPI_E2E_DB_CHECK_SSH_PORT", "58422")))
    parser.add_argument("--remote-env-file", default=env("FYAPI_E2E_REMOTE_ENV_FILE", "/opt/fy-api/config/fy-api.env"))
    args = parser.parse_args()

    token = normalize_token(args.token)
    if not args.base_url or not token:
        print("FYAPI_E2E_BASE_URL and FYAPI_E2E_TOKEN are required", file=sys.stderr)
        return 2

    image_urls = args.image_urls or DEFAULT_IMAGE_URLS
    if len(image_urls) < 2:
        print("at least two image URLs are required for the multi-image case", file=sys.stderr)
        return 2

    cases = build_cases(image_urls)
    if args.cases:
        selected = set(args.cases)
        cases = [case for case in cases if case.name in selected]
        missing = selected - {case.name for case in cases}
        if missing:
            print(f"unknown case(s): {', '.join(sorted(missing))}", file=sys.stderr)
            return 2

    run_dir = Path(args.output_root) / f"seedance2-e2e-matrix-{time.strftime('%Y%m%d-%H%M%S')}"
    run_dir.mkdir(parents=True, exist_ok=False)
    print(f"RUN_DIR={run_dir}")
    print("CASES=" + ",".join(case.name for case in cases))

    results: list[dict[str, Any]] = []
    failures: list[str] = []
    for case in cases:
        try:
            results.append(run_case(args, case, token, run_dir))
        except Exception as exc:  # noqa: BLE001 - this is an ops runner.
            failures.append(f"{case.name}: {exc}")
            print(f"[{case.name}] FAILED: {exc}", file=sys.stderr, flush=True)
            if not args.cases:
                break

    summary = {"run_dir": str(run_dir), "results": results, "failures": failures}
    save_json(run_dir / "summary.json", summary)
    print("SUMMARY=" + summarize(summary))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
