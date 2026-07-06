"""Smoke benchmark aggregation and Prometheus exposition."""

from __future__ import annotations

import csv
import json
import math
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path

from fy_loadtest.client import ChatResult
from fy_loadtest.metrics import LatencyStats, ThroughputStats, percentile


@dataclass(frozen=True)
class CaseKey:
    channel_id: int
    channel_name: str
    model: str
    streamed: bool


@dataclass
class Aggregate:
    channel_id: int
    channel_name: str
    model: str
    streamed: bool
    total: int = 0
    ok: int = 0
    failed: int = 0
    success_rate_pct: float = 0.0
    e2e: LatencyStats = field(default_factory=LatencyStats)
    ttft: LatencyStats = field(default_factory=LatencyStats)
    itl: LatencyStats = field(default_factory=LatencyStats)
    tokens_per_sec: ThroughputStats = field(default_factory=ThroughputStats)
    avg_prompt_tokens: float = 0.0
    avg_completion_tokens: float = 0.0
    avg_cached_tokens: float = 0.0
    error_breakdown: dict[str, int] = field(default_factory=dict)


def aggregate_results(key: CaseKey, results: list[ChatResult]) -> Aggregate:
    agg = Aggregate(
        channel_id=key.channel_id,
        channel_name=key.channel_name,
        model=key.model,
        streamed=key.streamed,
        total=len(results),
    )
    ok = [r for r in results if r.success]
    agg.ok = len(ok)
    agg.failed = len(results) - len(ok)
    agg.success_rate_pct = 100.0 * agg.ok / agg.total if agg.total else 0.0
    agg.e2e = _latency_stats([r.e2e_s for r in ok])
    agg.ttft = _latency_stats([r.ttft_s for r in ok if r.ttft_s > 0])
    agg.itl = _latency_stats([gap for r in ok for gap in r.inter_token_gaps_s])
    agg.tokens_per_sec = _throughput_stats([r.tokens_per_sec() for r in ok if r.tokens_per_sec() > 0])
    if ok:
        agg.avg_prompt_tokens = sum(r.usage.prompt_tokens for r in ok) / len(ok)
        agg.avg_completion_tokens = sum(r.usage.completion_tokens for r in ok) / len(ok)
        agg.avg_cached_tokens = sum(r.usage.cached_tokens for r in ok) / len(ok)
    for r in results:
        if not r.success:
            sig = r.error or "unknown error"
            agg.error_breakdown[sig] = agg.error_breakdown.get(sig, 0) + 1
    return agg


def _latency_stats(values_s: list[float]) -> LatencyStats:
    if not values_s:
        return LatencyStats()
    ms = sorted(v * 1000.0 for v in values_s)
    avg = sum(ms) / len(ms)
    var = sum((v - avg) ** 2 for v in ms) / (len(ms) - 1) if len(ms) > 1 else 0.0
    return LatencyStats(
        samples=len(ms),
        min_ms=ms[0],
        max_ms=ms[-1],
        avg_ms=avg,
        p50_ms=percentile(ms, 50),
        p95_ms=percentile(ms, 95),
        p99_ms=percentile(ms, 99),
        stddev_ms=math.sqrt(var),
    )


def _throughput_stats(values: list[float]) -> ThroughputStats:
    if not values:
        return ThroughputStats()
    sorted_vals = sorted(values)
    return ThroughputStats(
        samples=len(sorted_vals),
        min=sorted_vals[0],
        max=sorted_vals[-1],
        avg=sum(sorted_vals) / len(sorted_vals),
        p50=percentile(sorted_vals, 50),
    )


def write_exports(
    aggs: list[Aggregate],
    *,
    base_url: str,
    test: dict,
    formats: list[str],
    output_dir: str | Path,
) -> list[Path]:
    out = Path(output_dir)
    out.mkdir(parents=True, exist_ok=True)
    ts = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
    sorted_aggs = sorted(aggs, key=lambda a: (a.channel_id, a.model, a.streamed))
    written: list[Path] = []
    for fmt in formats:
        if fmt == "json":
            written.append(_write_json(sorted_aggs, base_url=base_url, test=test, out=out, ts=ts))
        elif fmt == "csv":
            written.append(_write_csv(sorted_aggs, out=out, ts=ts))
        else:
            raise ValueError(f"unknown export format: {fmt}")
    return written


def _write_json(aggs: list[Aggregate], *, base_url: str, test: dict, out: Path, ts: str) -> Path:
    path = out / f"benchmark_{ts}.json"
    doc = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "gateway": base_url,
        "test": test,
        "results": [asdict(a) for a in aggs],
    }
    path.write_text(json.dumps(doc, indent=2, ensure_ascii=False), encoding="utf-8")
    return path


_CSV_HEADER = [
    "channel_id", "channel_name", "model", "streamed",
    "total", "ok", "failed", "success_rate_pct",
    "e2e_p50_ms", "e2e_p95_ms", "e2e_p99_ms", "e2e_avg_ms", "e2e_max_ms",
    "ttft_p50_ms", "ttft_p95_ms", "ttft_p99_ms",
    "itl_p50_ms", "itl_p95_ms",
    "tokens_per_sec_avg", "tokens_per_sec_p50",
    "avg_prompt_tokens", "avg_completion_tokens", "avg_cached_tokens",
    "top_error",
]


def _write_csv(aggs: list[Aggregate], *, out: Path, ts: str) -> Path:
    path = out / f"benchmark_{ts}.csv"
    with path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow(_CSV_HEADER)
        for a in aggs:
            writer.writerow([
                a.channel_id,
                a.channel_name,
                a.model,
                str(a.streamed).lower(),
                a.total,
                a.ok,
                a.failed,
                _fmt(a.success_rate_pct, 1),
                _fmt(a.e2e.p50_ms, 1),
                _fmt(a.e2e.p95_ms, 1),
                _fmt(a.e2e.p99_ms, 1),
                _fmt(a.e2e.avg_ms, 1),
                _fmt(a.e2e.max_ms, 1),
                _fmt(a.ttft.p50_ms, 1),
                _fmt(a.ttft.p95_ms, 1),
                _fmt(a.ttft.p99_ms, 1),
                _fmt(a.itl.p50_ms, 1),
                _fmt(a.itl.p95_ms, 1),
                _fmt(a.tokens_per_sec.avg, 2),
                _fmt(a.tokens_per_sec.p50, 2),
                _fmt(a.avg_prompt_tokens, 1),
                _fmt(a.avg_completion_tokens, 1),
                _fmt(a.avg_cached_tokens, 1),
                top_error(a.error_breakdown),
            ])
    return path


def _fmt(value: float, digits: int) -> str:
    return "" if value == 0 else f"{value:.{digits}f}"


def top_error(errors: dict[str, int]) -> str:
    if not errors:
        return ""
    sig, n = max(errors.items(), key=lambda item: item[1])
    if len(sig) > 80:
        sig = sig[:77] + "..."
    return f"{sig} (x{n})"


class MetricsRegistry:
    def __init__(self) -> None:
        self.request_total: dict[tuple[str, str, bool, str], int] = {}
        self.latest: list[Aggregate] = []
        self.last_run: float = 0.0
        self.consecutive_ok: int = 0
        self.last_error: str = ""

    def replace(self, aggs: list[Aggregate], error: Exception | None = None) -> None:
        self.latest = aggs
        self.last_run = time.time()
        if error is None:
            self.consecutive_ok += 1
            self.last_error = ""
        else:
            self.consecutive_ok = 0
            self.last_error = str(error)
        for a in aggs:
            if a.ok:
                self.request_total[(a.channel_name, a.model, a.streamed, "ok")] = (
                    self.request_total.get((a.channel_name, a.model, a.streamed, "ok"), 0) + a.ok
                )
            if a.failed:
                self.request_total[(a.channel_name, a.model, a.streamed, "fail")] = (
                    self.request_total.get((a.channel_name, a.model, a.streamed, "fail"), 0) + a.failed
                )

    def exposition(self) -> str:
        lines: list[str] = []
        _help(lines, "channel_benchmark_request_total", "Cumulative chat-completion requests issued, by outcome.")
        _type(lines, "channel_benchmark_request_total", "counter")
        for channel, model, streamed, outcome in sorted(self.request_total):
            _metric(
                lines,
                "channel_benchmark_request_total",
                self.request_total[(channel, model, streamed, outcome)],
                {"channel": channel, "model": model, "streamed": str(streamed).lower(), "outcome": outcome},
            )

        _help(lines, "channel_benchmark_success_rate", "Per-case success rate from the latest run, in [0,1].")
        _type(lines, "channel_benchmark_success_rate", "gauge")
        for a in self.latest:
            if a.total:
                _metric(lines, "channel_benchmark_success_rate", a.success_rate_pct / 100.0, _case_labels(a))

        _help(lines, "channel_benchmark_e2e_seconds", "End-to-end latency quantiles from the latest run.")
        _type(lines, "channel_benchmark_e2e_seconds", "gauge")
        for a in self.latest:
            if a.e2e.samples:
                _quantile(lines, "channel_benchmark_e2e_seconds", a, "0.5", a.e2e.p50_ms / 1000.0)
                _quantile(lines, "channel_benchmark_e2e_seconds", a, "0.95", a.e2e.p95_ms / 1000.0)
                _quantile(lines, "channel_benchmark_e2e_seconds", a, "0.99", a.e2e.p99_ms / 1000.0)

        _help(lines, "channel_benchmark_ttft_seconds", "Time-to-first-token quantiles from the latest run (streaming only).")
        _type(lines, "channel_benchmark_ttft_seconds", "gauge")
        for a in self.latest:
            if a.streamed and a.ttft.samples:
                _quantile(lines, "channel_benchmark_ttft_seconds", a, "0.5", a.ttft.p50_ms / 1000.0)
                _quantile(lines, "channel_benchmark_ttft_seconds", a, "0.95", a.ttft.p95_ms / 1000.0)
                _quantile(lines, "channel_benchmark_ttft_seconds", a, "0.99", a.ttft.p99_ms / 1000.0)

        _help(lines, "channel_benchmark_tokens_per_sec", "Average decode throughput from the latest run.")
        _type(lines, "channel_benchmark_tokens_per_sec", "gauge")
        for a in self.latest:
            if a.tokens_per_sec.samples:
                _metric(lines, "channel_benchmark_tokens_per_sec", a.tokens_per_sec.avg, _case_labels(a))

        stale = time.time() - self.last_run if self.last_run else 0.0
        _help(lines, "channel_benchmark_run_age_seconds", "Seconds since the most recent benchmark run completed.")
        _type(lines, "channel_benchmark_run_age_seconds", "gauge")
        _metric(lines, "channel_benchmark_run_age_seconds", stale, None)
        _help(lines, "channel_benchmark_last_run_unix_seconds", "Unix timestamp of the most recent benchmark run; 0 if none yet.")
        _type(lines, "channel_benchmark_last_run_unix_seconds", "gauge")
        _metric(lines, "channel_benchmark_last_run_unix_seconds", self.last_run, None)
        _help(lines, "channel_benchmark_consecutive_runs_ok", "Consecutive benchmark cycles that completed without a top-level error.")
        _type(lines, "channel_benchmark_consecutive_runs_ok", "gauge")
        _metric(lines, "channel_benchmark_consecutive_runs_ok", self.consecutive_ok, None)
        return "\n".join(lines) + "\n"


def _case_labels(a: Aggregate) -> dict[str, str]:
    return {
        "channel": a.channel_name,
        "model": a.model,
        "streamed": str(a.streamed).lower(),
    }


def _help(lines: list[str], name: str, text: str) -> None:
    lines.append(f"# HELP {name} {text}")


def _type(lines: list[str], name: str, kind: str) -> None:
    lines.append(f"# TYPE {name} {kind}")


def _quantile(lines: list[str], name: str, a: Aggregate, quantile: str, value: float) -> None:
    labels = _case_labels(a)
    labels["quantile"] = quantile
    _metric(lines, name, value, labels)


def _metric(lines: list[str], name: str, value: float | int, labels: dict[str, str] | None) -> None:
    label_text = ""
    if labels:
        parts = [f'{k}="{_escape_label(v)}"' for k, v in sorted(labels.items())]
        label_text = "{" + ",".join(parts) + "}"
    if isinstance(value, int):
        val = str(value)
    else:
        val = f"{value:.12g}"
    lines.append(f"{name}{label_text} {val}")


def _escape_label(value: str) -> str:
    return value.replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"')
