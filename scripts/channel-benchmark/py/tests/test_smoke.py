from __future__ import annotations

from pathlib import Path

from fy_loadtest.client import ChatResult, Usage
from fy_smoke.config import LONG_THINKING_PROMPT, SmokeConfig
from fy_smoke.metrics import CaseKey, MetricsRegistry, aggregate_results, write_exports


def test_config_defaults_stream_modes(tmp_path: Path):
    cfg_path = tmp_path / "smoke.yaml"
    cfg_path.write_text(
        """
gateway:
  base_url: http://mock
  admin_token: admin
  admin_user_id: "1"
  user_token: sk-test
channels:
  - id: 7
    test_models: [gpt-test]
""",
        encoding="utf-8",
    )
    cfg = SmokeConfig.load(cfg_path)
    assert cfg.test.stream is True
    assert cfg.test.non_stream is True
    assert cfg.export.formats == ["json"]


def test_long_thinking_preset():
    cfg = SmokeConfig.from_dict(
        {
            "gateway": {
                "base_url": "http://mock",
                "admin_token": "admin",
                "admin_user_id": "1",
                "user_token": "sk-test",
            },
            "channels": [{"id": 1, "test_models": ["x"]}],
        }
    )
    cfg.apply_long_thinking()
    assert cfg.test.prompt == LONG_THINKING_PROMPT
    assert cfg.test.timeout_seconds == 1800
    assert cfg.test.max_tokens == 32000
    assert cfg.test.reps_per_case == 1
    assert cfg.test.concurrency == 1
    assert cfg.test.stream is True
    assert cfg.test.non_stream is False


def test_aggregate_and_exports(tmp_path: Path):
    ok = ChatResult(
        success=True,
        e2e_s=0.2,
        ttft_s=0.05,
        inter_token_gaps_s=[0.03],
        streamed=True,
        usage=Usage(prompt_tokens=10, completion_tokens=3, total_tokens=13, cached_tokens=2),
    )
    fail = ChatResult(success=False, streamed=True, error="HTTP 500: upstream")
    agg = aggregate_results(CaseKey(1, "openai", "gpt", True), [ok, fail])
    assert agg.total == 2
    assert agg.ok == 1
    assert agg.failed == 1
    assert agg.success_rate_pct == 50.0
    assert agg.e2e.p95_ms == 200.0
    assert agg.ttft.p50_ms == 50.0
    assert agg.avg_cached_tokens == 2.0
    assert agg.error_breakdown == {"HTTP 500: upstream": 1}

    files = write_exports(
        [agg],
        base_url="http://mock",
        test={"concurrency": 1},
        formats=["json", "csv"],
        output_dir=tmp_path,
    )
    assert len(files) == 2
    assert any(p.suffix == ".json" for p in files)
    assert any(p.suffix == ".csv" for p in files)


def test_prometheus_exposition_escapes_labels():
    agg = aggregate_results(
        CaseKey(1, 'weird "quoted"\nname', "gpt", True),
        [
            ChatResult(
                success=True,
                e2e_s=0.2,
                ttft_s=0.05,
                streamed=True,
                usage=Usage(completion_tokens=2),
            )
        ],
    )
    registry = MetricsRegistry()
    registry.replace([agg], None)
    out = registry.exposition()
    assert "# TYPE channel_benchmark_request_total counter" in out
    assert 'channel="weird \\"quoted\\"\\nname"' in out
    assert 'channel_benchmark_ttft_seconds{channel=' in out
    assert "channel_benchmark_consecutive_runs_ok 1" in out
