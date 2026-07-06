"""CLI entrypoint for fy-smoke."""

from __future__ import annotations

import argparse
import asyncio
import signal
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from rich.console import Console

from . import __version__
from .config import SmokeConfig
from .metrics import MetricsRegistry, write_exports
from .runner import SmokeRunner


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="fy-smoke",
        description="Smoke benchmark Fy-api channels via /v1/chat/completions.",
    )
    parser.add_argument("-c", "--config", default="smoke.yaml", help="Path to smoke YAML config")
    parser.add_argument("--base-url", help="Override gateway.base_url")
    parser.add_argument("--output", help="Override export.output_dir")
    parser.add_argument("--concurrency", type=int, help="Override test.concurrency")
    parser.add_argument("--reps", type=int, help="Override test.reps_per_case")
    parser.add_argument("--formats", help="Override export.formats (comma-separated: json,csv)")
    parser.add_argument("--dry-run", action="store_true", help="Validate config and exit")
    parser.add_argument("--prom-listen", help="If set, run as daemon and expose /metrics on this address, e.g. :9090")
    parser.add_argument("--prom-interval", default="5m", help="Daemon benchmark interval, e.g. 300s, 5m, 1h")
    parser.add_argument("--no-export", action="store_true", help="Skip JSON/CSV export in daemon mode")
    parser.add_argument("--long-thinking", action="store_true", help="Run long-reasoning timeout regression preset")
    parser.add_argument("-V", "--version", action="version", version=f"fy-smoke {__version__}")
    return parser


def apply_overrides(cfg: SmokeConfig, args: argparse.Namespace) -> None:
    if args.base_url:
        cfg.gateway.base_url = args.base_url
    if args.output:
        cfg.export.output_dir = args.output
    if args.concurrency:
        cfg.test.concurrency = args.concurrency
    if args.reps:
        cfg.test.reps_per_case = args.reps
    if args.formats:
        cfg.export.formats = [x.strip() for x in args.formats.split(",") if x.strip()]
    if args.long_thinking:
        cfg.apply_long_thinking()
    cfg.validate()


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    console = Console()
    try:
        cfg = SmokeConfig.load(args.config)
        apply_overrides(cfg, args)
    except (OSError, ValueError) as exc:
        console.print(f"[red]config: {exc}[/red]")
        return 2

    console.print(f"[bold]Gateway:[/bold]       {cfg.gateway.base_url}")
    console.print(f"[bold]Channels:[/bold]      {len(cfg.channels)} configured")
    console.print(f"[bold]Concurrency:[/bold]   {cfg.test.concurrency}")
    console.print(f"[bold]Reps/case:[/bold]     {cfg.test.reps_per_case}")
    console.print(f"[bold]Stream:[/bold]        {cfg.test.stream} (+ non-stream={cfg.test.non_stream})")
    console.print(f"[bold]Max tokens:[/bold]    {cfg.test.max_tokens}")
    console.print(f"[bold]Output dir:[/bold]    {cfg.export.output_dir}")
    console.print(f"[bold]Formats:[/bold]       {cfg.export.formats}")
    if args.long_thinking:
        console.print(
            f"[bold]Preset:[/bold]        long-thinking "
            f"(timeout={cfg.test.timeout_seconds}s, max_tokens={cfg.test.max_tokens}, reps={cfg.test.reps_per_case})"
        )
    if args.dry_run:
        console.print("\n[cyan](dry-run: config valid, no requests sent)[/cyan]")
        return 0

    if args.prom_listen:
        return _run_daemon(cfg, args, console)
    return _run_once(cfg, console)


def _run_once(cfg: SmokeConfig, console: Console) -> int:
    try:
        aggs = asyncio.run(SmokeRunner(cfg, console=console).run())
        files = write_exports(
            aggs,
            base_url=cfg.gateway.base_url,
            test=_test_doc(cfg),
            formats=cfg.export.formats,
            output_dir=cfg.export.output_dir,
        )
    except KeyboardInterrupt:
        console.print("[yellow]interrupted[/yellow]")
        return 130
    except Exception as exc:
        console.print(f"[red]run: {exc}[/red]")
        return 1

    for path in files:
        console.print(f"wrote {path}")
    _print_summary(aggs, console)
    return 0


def _run_daemon(cfg: SmokeConfig, args: argparse.Namespace, console: Console) -> int:
    registry = MetricsRegistry()
    host, port = _parse_listen(args.prom_listen)
    interval = _parse_duration(args.prom_interval)
    server = _make_server(host, port, registry)
    stop = threading.Event()

    def serve() -> None:
        console.print(f"prom: serving /metrics on {args.prom_listen}")
        server.serve_forever(poll_interval=0.5)

    thread = threading.Thread(target=serve, daemon=True)
    thread.start()

    def handle_signal(signum, frame) -> None:  # noqa: ANN001
        stop.set()
        server.shutdown()

    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)

    while not stop.is_set():
        start = time.monotonic()
        try:
            console.print("prom: starting benchmark cycle")
            aggs = asyncio.run(SmokeRunner(cfg, console=console).run())
            registry.replace(aggs, None)
            if not args.no_export:
                for path in write_exports(
                    aggs,
                    base_url=cfg.gateway.base_url,
                    test=_test_doc(cfg),
                    formats=cfg.export.formats,
                    output_dir=cfg.export.output_dir,
                ):
                    console.print(f"prom: wrote {path}")
        except Exception as exc:
            registry.replace([], exc)
            console.print(f"[red]prom: cycle failed: {exc}[/red]")
        elapsed = time.monotonic() - start
        wait = max(1.0, interval - elapsed)
        stop.wait(wait)
    server.server_close()
    thread.join(timeout=5)
    return 0


def _make_server(host: str, port: int, registry: MetricsRegistry) -> ThreadingHTTPServer:
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:  # noqa: N802
            if self.path != "/metrics":
                self.send_response(404)
                self.end_headers()
                return
            data = registry.exposition().encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/plain; version=0.0.4")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, format: str, *args) -> None:  # noqa: A002
            return

    return ThreadingHTTPServer((host, port), Handler)


def _parse_listen(raw: str) -> tuple[str, int]:
    if raw.startswith(":"):
        return "", int(raw[1:])
    host, port = raw.rsplit(":", 1)
    return host, int(port)


def _parse_duration(raw: str) -> float:
    raw = raw.strip().lower()
    mult = 1.0
    if raw.endswith("ms"):
        mult = 0.001
        raw = raw[:-2]
    elif raw.endswith("s"):
        raw = raw[:-1]
    elif raw.endswith("m"):
        mult = 60.0
        raw = raw[:-1]
    elif raw.endswith("h"):
        mult = 3600.0
        raw = raw[:-1]
    return float(raw) * mult


def _test_doc(cfg: SmokeConfig) -> dict:
    return {
        "concurrency": cfg.test.concurrency,
        "reps_per_case": cfg.test.reps_per_case,
        "timeout_seconds": cfg.test.timeout_seconds,
        "max_tokens": cfg.test.max_tokens,
        "prompt": cfg.test.prompt,
    }


def _print_summary(aggs, console: Console) -> None:  # noqa: ANN001
    console.print()
    console.print(
        f"{'chID':<5} {'channel':<20} {'model':<28} {'stream':<7} "
        f"{'ok':>5} {'fail':>5} {'succ%':>7} {'e2e_p95':>8} {'ttft_p95':>8} {'tok/s':>8}"
    )
    console.print("-" * 110)
    for a in sorted(aggs, key=lambda x: (x.channel_id, x.model, x.streamed)):
        console.print(
            f"{a.channel_id:<5} {a.channel_name[:20]:<20} {a.model[:28]:<28} "
            f"{str(a.streamed):<7} {a.ok:>5} {a.failed:>5} "
            f"{a.success_rate_pct:>6.1f}% {a.e2e.p95_ms:>7.0f} "
            f"{a.ttft.p95_ms:>8.0f} {a.tokens_per_sec.avg:>8.1f}"
        )


if __name__ == "__main__":
    sys.exit(main())
