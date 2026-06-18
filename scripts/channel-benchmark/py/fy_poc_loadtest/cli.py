"""CLI entrypoint for fy-poc-loadtest."""

from __future__ import annotations

import argparse
import asyncio

from rich.console import Console

from . import __version__
from .config import Config
from .report import write_reports
from .runner import PocRunner


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="fy-poc-loadtest",
        description="POC-style LLM performance validation for Fy-api channels.",
    )
    p.add_argument("-c", "--config", default="poc-loadtest.yaml", help="Path to YAML config")
    p.add_argument("--base-url", help="Override gateway.base_url")
    p.add_argument("--model", help="Override poc.models with one model")
    p.add_argument("--concurrencies", help="Override poc.concurrency_levels, e.g. 1,10,20")
    p.add_argument("--output", help="Override export.output_dir")
    p.add_argument("--formats", help="Override export.formats: json,csv,markdown")
    p.add_argument("--dry-run", action="store_true", help="Validate config and exit")
    p.add_argument("-V", "--version", action="version", version=f"fy-poc-loadtest {__version__}")
    return p


def apply_overrides(cfg: Config, args: argparse.Namespace) -> None:
    if args.base_url:
        cfg.gateway.base_url = args.base_url
    if args.model:
        cfg.poc.models = [args.model]
    if args.concurrencies:
        cfg.poc.concurrency_levels = [int(x) for x in args.concurrencies.split(",") if x.strip()]
    if args.output:
        cfg.export.output_dir = args.output
    if args.formats:
        cfg.export.formats = [x.strip() for x in args.formats.split(",") if x.strip()]


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    console = Console()
    try:
        cfg = Config.load(args.config)
        apply_overrides(cfg, args)
        cfg.validate()
    except (FileNotFoundError, ValueError) as e:
        console.print(f"[red]config: {e}[/red]")
        return 2

    console.print(f"[bold]Gateway:[/bold] {cfg.gateway.base_url}")
    console.print(f"[bold]Models:[/bold] {cfg.poc.models}")
    console.print(f"[bold]Scenarios:[/bold] {[s.name for s in cfg.poc.scenarios]}")
    console.print(f"[bold]Concurrency:[/bold] {cfg.poc.concurrency_levels}")
    console.print(f"[bold]Output:[/bold] {cfg.export.output_dir} ({cfg.export.formats})")
    if args.dry_run:
        console.print("[cyan](dry-run: config valid, no requests sent)[/cyan]")
        return 0

    try:
        result = asyncio.run(PocRunner(cfg, console=console).run())
        files = write_reports(result, cfg)
    except KeyboardInterrupt:
        console.print("[yellow]interrupted[/yellow]")
        return 130

    console.rule("[bold green]done")
    for f in files:
        console.print(f"wrote {f}")
    return 0
