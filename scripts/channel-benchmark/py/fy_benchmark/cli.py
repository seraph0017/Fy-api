"""Unified channel benchmark CLI."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from rich.console import Console

from .config import BenchmarkConfig
from .runner import BenchmarkRunner


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="fy-benchmark",
        description="Run full channel/model benchmark suite from one simple YAML config.",
    )
    parser.add_argument("-c", "--config", default="benchmark.yaml", help="Path to benchmark YAML")
    parser.add_argument("--channel-id", type=int, help="Override target.channel_id")
    parser.add_argument("--model", action="append", help="Override target.models; repeat for multiple models")
    parser.add_argument("--type", choices=["text", "image", "video"], help="Type for --model overrides")
    parser.add_argument("--mode", choices=["quick", "standard", "strict", "deep"], help="Override profile.mode")
    parser.add_argument("--output-dir", help="Override profile.output_dir")
    parser.add_argument("--dry-run", action="store_true", help="Validate and print plan only")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    console = Console()
    config_path = Path(args.config)
    try:
        cfg = BenchmarkConfig.load(config_path)
        if args.channel_id is not None:
            cfg.target.channel_id = args.channel_id
        if args.model:
            from .config import ModelTarget
            cfg.target.models = [ModelTarget(id=m, type=args.type or "text") for m in args.model]
        if args.mode:
            cfg.profile.mode = args.mode
            cfg.profile.strict = args.mode == "strict"
        if args.output_dir:
            cfg.profile.output_dir = args.output_dir
        cfg.validate()
    except (OSError, ValueError) as exc:
        console.print(f"[red]config: {exc}[/red]")
        return 2

    runner = BenchmarkRunner(cfg, config_path=config_path, console=console)
    try:
        run = runner.dry_run() if args.dry_run else runner.run()
    except KeyboardInterrupt:
        console.print("[yellow]interrupted[/yellow]")
        return 130

    if args.dry_run:
        return 0
    failed = [r for r in run.results if not r.skipped and r.returncode != 0]
    if failed:
        console.print(f"[red]completed with {len(failed)} failed step(s)[/red]")
        return 1
    console.print(f"[green]completed[/green] {run.run_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
