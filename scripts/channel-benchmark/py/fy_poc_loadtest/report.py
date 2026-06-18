"""Report writers for POC load-test output."""

from __future__ import annotations

import csv
import dataclasses
import json
from datetime import datetime, timezone
from pathlib import Path

from .config import Config
from .runner import ChannelResult, PocResult


def write_reports(result: PocResult, cfg: Config) -> list[Path]:
    out = Path(cfg.export.output_dir)
    out.mkdir(parents=True, exist_ok=True)
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d_%H-%M-%S")
    written: list[Path] = []
    for fmt in cfg.export.formats:
        if fmt == "json":
            written.append(_write_json(result, cfg, out, ts))
        elif fmt == "csv":
            written.append(_write_csv(result, cfg, out, ts))
        elif fmt == "markdown":
            written.append(_write_md(result, cfg, out, ts))
        else:
            raise ValueError(f"unknown export format: {fmt}")
    return written


def _write_json(result: PocResult, cfg: Config, out: Path, ts: str) -> Path:
    path = out / f"poc_loadtest_{ts}.json"
    doc = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "gateway": result.base_url,
        "poc": dataclasses.asdict(cfg.poc),
        "models": [
            {
                "model": mr.model,
                "channels": [
                    {
                        "channel_name": ch.channel_name,
                        "pin_channel_id": ch.pin_channel_id,
                        "scenarios": [
                            {
                                "scenario": dataclasses.asdict(sr.scenario),
                                "levels": [dataclasses.asdict(lv) for lv in sr.levels],
                            }
                            for sr in ch.scenarios
                        ],
                    }
                    for ch in mr.channels
                ],
            }
            for mr in result.model_results
        ],
    }
    path.write_text(json.dumps(doc, indent=2, ensure_ascii=False), encoding="utf-8")
    return path


_CSV_HEADER = [
    "model", "channel", "channel_id", "scenario", "input_tokens", "dataset",
    "concurrency", "requests", "ok", "failed", "success_rate_pct",
    "avg_latency_ms", "p95_latency_ms", "avg_ttft_ms", "p95_ttft_ms",
    "avg_tpot_ms", "p95_tpot_ms", "avg_tokens_per_sec", "p50_tokens_per_sec",
    "aggregate_tok_per_s", "avg_prompt_tokens", "avg_completion_tokens",
    "rpm", "output_tpm", "errors_429", "errors_5xx", "errors_timeout", "top_error",
]


def _write_csv(result: PocResult, cfg: Config, out: Path, ts: str) -> Path:
    path = out / f"poc_loadtest_{ts}.csv"
    with path.open("w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(_CSV_HEADER)
        for mr in result.model_results:
            for ch in mr.channels:
                for sr in ch.scenarios:
                    for lv in sr.levels:
                        w.writerow([
                            mr.model, ch.channel_name, ch.pin_channel_id or "",
                            sr.scenario.name, sr.scenario.input_tokens or "",
                            sr.scenario.dataset, lv.concurrency, lv.total, lv.ok, lv.failed,
                            f"{lv.success_rate_pct:.1f}",
                            _fmt(lv.e2e.avg_ms), _fmt(lv.e2e.p95_ms),
                            _fmt(lv.ttft.avg_ms), _fmt(lv.ttft.p95_ms),
                            _fmt(lv.tpot.avg_ms), _fmt(lv.tpot.p95_ms),
                            f"{lv.per_request_tok_per_s.avg:.2f}",
                            f"{lv.per_request_tok_per_s.p50:.2f}",
                            f"{lv.aggregate_tok_per_s:.2f}",
                            f"{lv.avg_prompt_tokens:.1f}",
                            f"{lv.avg_completion_tokens:.1f}",
                            f"{lv.rpm:.1f}", f"{lv.output_tpm:.1f}",
                            lv.errors_429, lv.errors_5xx, lv.errors_timeout,
                            _top_error(lv.error_breakdown),
                        ])
    return path


def _write_md(result: PocResult, cfg: Config, out: Path, ts: str) -> Path:
    path = out / f"poc_loadtest_{ts}.md"
    now = datetime.now().strftime("%Y-%m-%d %H:%M")
    lines: list[str] = [
        f"# {cfg.poc.report_title}",
        "",
        f"{cfg.poc.platform_name} 是企业级 AI 多模型服务平台，提供统一 API 调用入口。本报告按 POC 压测方法，对被测模型进行短文本、中文本、长文本在不同并发下的性能测试。",
        "",
        "## 测评基本信息",
        "",
        "| 字段名称 | 内容 |",
        "|---|---|",
        f"| 平台名称 | {cfg.poc.platform_name} |",
        f"| 模型名称与版本 | {', '.join(mr.model for mr in result.model_results)} |",
        "| 测评类型 | LLM 性能验证 / POC 压测 |",
        f"| 测评范围 | {cfg.poc.test_scope} |",
        f"| 报告编号 | {cfg.poc.report_id or ts} |",
        f"| 测评时间 | {now} |",
        f"| 网关地址 | `{result.base_url}` |",
        "",
        "## 测评环境",
        "",
        "| 环境类别 | 参数配置 | 备注 |",
        "|---|---|---|",
    ]
    if cfg.poc.environment:
        for k, v in cfg.poc.environment.items():
            lines.append(f"| {k} | {v} |  |")
    else:
        lines.extend([
            "| 硬件配置 | 待补充 | 服务器型号/云实例规格 |",
            "| CPU/内存 | 待补充 | CPU 核心数、内存容量 |",
            "| 存储 | 待补充 | 存储类型与容量 |",
        ])
    lines.extend([
        f"| 测评工具 | `fy-poc-loadtest` | 基于 channel-benchmark / OpenAI-compatible streaming |",
        f"| 语言/框架 | Python / httpx | request_timeout={cfg.poc.request_timeout_sec}s |",
        "",
        "## 测试场景",
        "",
        "| 场景 | 输入规格 | 数据集 | 输出 token 设置 |",
        "|---|---:|---|---:|",
    ])
    for sc in cfg.poc.scenarios:
        dataset = sc.dataset_path or sc.dataset
        lines.append(f"| {sc.name} | {sc.input_tokens or '-'} tokens | `{dataset}` | {sc.max_tokens} |")

    lines.extend([
        "",
        "## 性能测评关注指标",
        "",
        "| 指标名称 | 单位 | 描述 |",
        "|---|---|---|",
        "| 首次生成 token 时间（TTFT） | ms | 首包延迟，取平均值和 p95 |",
        "| 平均延迟（Latency） | ms | 端到端请求耗时，取平均值和 p95 |",
        "| 每 token 延迟（TPOT） | ms | 输出平稳性，取平均值和 p95 |",
        "| 单用户推理速度 | tokens/s | 单请求解码速度，取平均值和 p50 |",
        "| 请求成功率 | % | 高并发稳定性指标 |",
        "",
    ])

    for mr in result.model_results:
        lines.extend(["## 测评内容结果", "", f"### 模型：{mr.model}", ""])
        for ch in mr.channels:
            _append_channel(lines, ch)

    lines.extend(_append_summary(result))
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return path


def _append_channel(lines: list[str], ch: ChannelResult) -> None:
    label = ch.channel_name
    if ch.pin_channel_id is not None:
        label = f"{label} (channel_id={ch.pin_channel_id})"
    lines.extend([f"#### 渠道：{label}", ""])
    for sr in ch.scenarios:
        lines.extend([
            f"##### {sr.scenario.name}",
            "",
            "| 并发 | 请求数 | 成功率 | TTFT avg/p95 (ms) | Latency avg/p95 (ms) | TPOT avg/p95 (ms) | tokens/s avg/p50 | RPM | 错误 |",
            "|---:|---:|---:|---:|---:|---:|---:|---:|---|",
        ])
        for lv in sr.levels:
            err = _error_cell(lv.errors_429, lv.errors_5xx, lv.errors_timeout)
            lines.append(
                f"| {lv.concurrency} | {lv.total} | {lv.success_rate_pct:.1f}% "
                f"| {_fmt_pair(lv.ttft.avg_ms, lv.ttft.p95_ms)} "
                f"| {_fmt_pair(lv.e2e.avg_ms, lv.e2e.p95_ms)} "
                f"| {_fmt_pair(lv.tpot.avg_ms, lv.tpot.p95_ms)} "
                f"| {lv.per_request_tok_per_s.avg:.1f}/{lv.per_request_tok_per_s.p50:.1f} "
                f"| {lv.rpm:.0f} | {err} |"
            )
        if any(lv.error_breakdown for lv in sr.levels):
            lines.extend(["", "错误样例：", ""])
            for lv in sr.levels:
                top = _top_error(lv.error_breakdown)
                if top:
                    lines.append(f"- 并发 {lv.concurrency}: `{top}`")
        lines.append("")


def _append_summary(result: PocResult) -> list[str]:
    lines = ["## 结论摘要", ""]
    for mr in result.model_results:
        for ch in mr.channels:
            levels = [lv for sr in ch.scenarios for lv in sr.levels]
            if not levels:
                continue
            total = sum(lv.total for lv in levels)
            ok = sum(lv.ok for lv in levels)
            sr_pct = ok / total * 100 if total else 0
            best_tps = max(levels, key=lambda lv: lv.per_request_tok_per_s.avg)
            worst_success = min(levels, key=lambda lv: lv.success_rate_pct)
            lines.append(
                f"- {mr.model} / {ch.channel_name}: 总成功率 {sr_pct:.1f}% ({ok}/{total})，"
                f"最高平均 tokens/s {best_tps.per_request_tok_per_s.avg:.1f}（并发 {best_tps.concurrency}），"
                f"最低成功率 {worst_success.success_rate_pct:.1f}%（并发 {worst_success.concurrency}）。"
            )
    if len(lines) == 2:
        lines.append("- 未产生有效测评数据。")
    return lines


def _fmt(v: float) -> str:
    return f"{v:.1f}" if v else ""


def _fmt_pair(avg: float, p95: float) -> str:
    if avg <= 0 and p95 <= 0:
        return "-"
    return f"{avg:.1f}/{p95:.1f}"


def _error_cell(errors_429: int, errors_5xx: int, errors_timeout: int) -> str:
    if errors_429 == 0 and errors_5xx == 0 and errors_timeout == 0:
        return "0"
    return f"429={errors_429}, 5xx={errors_5xx}, timeout={errors_timeout}"


def _top_error(errors: dict[str, int]) -> str:
    if not errors:
        return ""
    sig, n = max(errors.items(), key=lambda kv: kv[1])
    sig = sig.replace("\n", " ")
    if len(sig) > 140:
        sig = sig[:137] + "..."
    return f"{sig} (x{n})"
