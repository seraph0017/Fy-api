from __future__ import annotations

import json
import time
from pathlib import Path

import httpx
import pytest

from fy_loadtest.client import ChatClient
from fy_poc_loadtest.config import Config, default_requests_for_concurrency
from fy_poc_loadtest.report import write_reports
from fy_poc_loadtest.runner import PocRunner


def _sse_frame(obj: dict) -> bytes:
    return f"data: {json.dumps(obj)}\n\n".encode()


def _transport() -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        def stream_body():
            time.sleep(0.002)
            yield _sse_frame({"choices": [{"delta": {"content": "po"}}]})
            time.sleep(0.002)
            yield _sse_frame({"choices": [{"delta": {"content": "ng"}}]})
            yield _sse_frame({
                "choices": [{"delta": {}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 23, "completion_tokens": 2, "total_tokens": 25},
            })
            yield b"data: [DONE]\n\n"

        return httpx.Response(
            200,
            headers={"content-type": "text/event-stream"},
            content=b"".join(stream_body()),
        )

    return httpx.MockTransport(handler)


def test_default_requests_match_poc_template():
    assert default_requests_for_concurrency(1) == 50
    assert default_requests_for_concurrency(10) == 100
    assert default_requests_for_concurrency(20) == 200
    assert default_requests_for_concurrency(64) == 250
    assert default_requests_for_concurrency(80) == 300
    assert default_requests_for_concurrency(128) == 350
    assert default_requests_for_concurrency(256) == 500


def test_config_round_trip(tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setenv("FY_API_USER_TOKEN", "sk-test")
    p = tmp_path / "poc.yaml"
    p.write_text(
        "gateway:\n"
        "  base_url: http://mock\n"
        "  user_token: ${FY_API_USER_TOKEN}\n"
        "  channels:\n"
        "    - name: c42\n"
        "      pin_channel_id: 42\n"
        "poc:\n"
        "  models: [gpt-test]\n"
        "  concurrency_levels: [1, 10]\n"
        "  requests_by_concurrency:\n"
        "    1: 2\n"
        "    10: 3\n"
        "  sleep_between_levels_sec: 0\n"
        "  scenarios:\n"
        "    - name: 短文本\n"
        "      input_tokens: 23\n"
        "      prompt: hi\n"
        "      max_tokens: 8\n",
        encoding="utf-8",
    )
    cfg = Config.load(p)
    cfg.validate()
    assert cfg.gateway.channels[0].pin_channel_id == 42
    assert cfg.poc.models == ["gpt-test"]
    assert cfg.requests_for(cfg.poc.scenarios[0], 1) == 2
    assert cfg.requests_for(cfg.poc.scenarios[0], 10) == 3


def test_scenario_can_override_concurrency_and_requests():
    cfg = Config._from_dict({
        "gateway": {"base_url": "http://mock", "user_token": "sk-test"},
        "poc": {
            "models": ["gpt-test"],
            "concurrency_levels": [1, 20, 200],
            "requests_by_concurrency": {1: 3, 20: 20, 200: 200},
            "scenarios": [{
                "name": "long",
                "prompt": "hi",
                "concurrency_levels": [1, 20],
                "requests_by_concurrency": {1: 2, 20: 10},
            }],
        },
    })
    cfg.validate()
    scenario = cfg.poc.scenarios[0]
    assert cfg.concurrency_levels_for(scenario) == [1, 20]
    assert cfg.requests_for(scenario, 1) == 2
    assert cfg.requests_for(scenario, 20) == 10


@pytest.mark.asyncio
async def test_poc_runner_writes_reports(tmp_path: Path):
    cfg = Config._from_dict({
        "gateway": {
            "base_url": "http://mock",
            "user_token": "sk-test",
            "channels": [{"name": "c42", "pin_channel_id": 42}],
        },
        "poc": {
            "models": ["gpt-test"],
            "concurrency_levels": [1],
            "requests_by_concurrency": {1: 2},
            "sleep_between_levels_sec": 0,
            "scenarios": [{"name": "短文本", "input_tokens": 23, "prompt": "hi", "max_tokens": 8}],
        },
        "export": {"formats": ["json", "csv", "markdown"], "output_dir": str(tmp_path)},
    })
    cfg.validate()

    transport = _transport()
    real_init = ChatClient.__init__

    def patched_init(self, *args, **kwargs):
        real_init(self, *args, **kwargs)
        old = self._client
        self._client = httpx.AsyncClient(
            transport=transport,
            timeout=old.timeout,
            headers=old.headers,
        )

    ChatClient.__init__ = patched_init  # type: ignore[method-assign]
    try:
        result = await PocRunner(cfg).run()
    finally:
        ChatClient.__init__ = real_init  # type: ignore[method-assign]

    assert result.model_results[0].channels[0].scenarios[0].levels[0].ok == 2
    files = write_reports(result, cfg)
    assert len(files) == 3
    assert all(f.exists() and f.stat().st_size > 0 for f in files)
    md = next(f for f in files if f.suffix == ".md").read_text(encoding="utf-8")
    assert "性能测评关注指标" in md
    assert "TTFT" in md
