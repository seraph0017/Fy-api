"""Orchestrate the channel benchmark suite from one simple config."""

from __future__ import annotations

import json
import subprocess
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml
from rich.console import Console

from .config import BenchmarkConfig, ModelTarget


@dataclass
class StepResult:
    name: str
    model: str
    command: list[str]
    config_path: str
    log_path: str
    elapsed_s: float
    returncode: int
    skipped: bool = False
    reason: str = ""


@dataclass
class BenchmarkRun:
    run_dir: Path
    results: list[StepResult] = field(default_factory=list)
    score_json: Path | None = None
    score_markdown: Path | None = None


class BenchmarkRunner:
    def __init__(self, cfg: BenchmarkConfig, *, config_path: Path, console: Console | None = None):
        self.cfg = cfg
        self.config_path = config_path
        self.console = console or Console()
        stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        safe_channel = f"ch{cfg.target.channel_id}"
        self.run_dir = Path(cfg.profile.output_dir) / f"{stamp}-{safe_channel}"
        self.config_dir = self.run_dir / "configs"
        self.log_dir = self.run_dir / "logs"

    def plan(self) -> list[tuple[str, ModelTarget]]:
        steps: list[tuple[str, ModelTarget]] = []
        for model in self.cfg.target.models:
            if model.type == "text":
                for name in self._text_modules():
                    steps.append((name, model))
            elif model.type == "image":
                for name in self._image_modules():
                    steps.append((name, model))
            else:
                steps.append(("video-placeholder", model))
        return steps

    def dry_run(self) -> BenchmarkRun:
        self._ensure_dirs()
        self._write_manifest(dry_run=True)
        run = BenchmarkRun(self.run_dir)
        for step, model in self.plan():
            reason = self._skip_reason(step)
            if reason:
                self._record_skip(run, step, model, reason)
        self.console.print(f"[bold]Run dir:[/bold] {self.run_dir}")
        self.console.print(f"[bold]Channel:[/bold] {self.cfg.channel_name} (id={self.cfg.target.channel_id})")
        self.console.print(f"[bold]Mode:[/bold] {self.cfg.profile.mode}")
        self.console.print("[bold]Planned steps:[/bold]")
        for step, model in self.plan():
            self.console.print(f"  - {model.id} [{model.type}] :: {step}")
        self.console.print("\n[cyan](dry-run: generated manifest only, no requests sent)[/cyan]")
        return run

    def run(self) -> BenchmarkRun:
        self._ensure_dirs()
        self._write_manifest(dry_run=False)
        run = BenchmarkRun(self.run_dir)
        for step, model in self.plan():
            reason = self._skip_reason(step)
            if reason:
                self._record_skip(run, step, model, reason)
                continue
            cfg_path, command = self._build_step(step, model)
            result = self._execute(step, model, cfg_path, command)
            run.results.append(result)
        self._score(run)
        self._write_run_report(run)
        return run

    def _text_modules(self) -> list[str]:
        mode = self.cfg.profile.mode
        names: list[str] = []
        if self.cfg.modules.smoke:
            names.append("smoke")
        if self.cfg.modules.conformance:
            names.append("conformance")
        if self.cfg.modules.integrity:
            names.append("integrity")
        if self.cfg.modules.loadtest:
            names.append("loadtest")
        if self.cfg.modules.quality:
            names.append("quality")
        if self.cfg.target.baseline_channel_id and (
            self.cfg.modules.canary or mode in {"strict", "deep"}
        ):
            names.extend(["canary-baseline", "canary-audit"])
        return names

    def _image_modules(self) -> list[str]:
        names: list[str] = []
        if self.cfg.modules.image_loadtest:
            names.append("image-loadtest")
        if self.cfg.modules.image_conformance:
            names.append("image-conformance")
        if self.cfg.target.baseline_channel_id and (
            self.cfg.modules.image_canary or self.cfg.profile.mode in {"strict", "deep"}
        ):
            names.append("image-canary")
        return names

    def _skip_reason(self, step: str) -> str:
        if step == "video-placeholder":
            return "video runner not implemented yet"
        if step == "smoke" and not self.cfg.gateway.tokens.admin:
            return "gateway.tokens.admin is not set; smoke metadata lookup skipped"
        return ""

    def _execute(self, step: str, model: ModelTarget, cfg_path: Path, command: list[str]) -> StepResult:
        log_path = self.log_dir / f"{model.id}-{step}.log"
        self.console.rule(f"{model.id} :: {step}")
        self.console.print(" ".join(command))
        start = time.perf_counter()
        with log_path.open("w", encoding="utf-8") as log:
            proc = subprocess.run(
                command,
                cwd=Path(__file__).resolve().parents[1],
                text=True,
                stdout=log,
                stderr=subprocess.STDOUT,
                check=False,
            )
        elapsed = time.perf_counter() - start
        color = "green" if proc.returncode == 0 else "red"
        self.console.print(f"[{color}]exit={proc.returncode}[/{color}] elapsed={elapsed:.1f}s log={log_path}")
        return StepResult(
            name=step,
            model=model.id,
            command=command,
            config_path=str(cfg_path),
            log_path=str(log_path),
            elapsed_s=elapsed,
            returncode=proc.returncode,
        )

    def _record_skip(self, run: BenchmarkRun, step: str, model: ModelTarget, reason: str) -> None:
        result = StepResult(
            name=step,
            model=model.id,
            command=[],
            config_path="",
            log_path="",
            elapsed_s=0.0,
            returncode=0,
            skipped=True,
            reason=reason,
        )
        run.results.append(result)
        self.console.print(f"[yellow]skip[/yellow] {model.id} :: {step} — {reason}")

    def _build_step(self, step: str, model: ModelTarget) -> tuple[Path, list[str]]:
        self._ensure_dirs()
        builders = {
            "smoke": self._smoke_config,
            "loadtest": self._loadtest_config,
            "quality": self._quality_config,
            "conformance": self._conformance_config,
            "integrity": self._integrity_config,
            "canary-baseline": lambda m: self._canary_config(m, baseline=True),
            "canary-audit": lambda m: self._canary_config(m, baseline=False),
            "image-loadtest": self._image_loadtest_config,
            "image-conformance": self._image_conformance_config,
            "image-canary": self._image_canary_config,
        }
        cfg = builders[step](model)
        path = self.config_dir / f"{model.id}-{step}.yaml"
        path.write_text(yaml.safe_dump(cfg, sort_keys=False, allow_unicode=True), encoding="utf-8")

        module_args = {
            "smoke": ["-m", "fy_smoke.cli", "-c", str(path)],
            "loadtest": ["-m", "fy_loadtest.cli", "-c", str(path)],
            "quality": ["-m", "fy_quality.cli", "-c", str(path), "--formats", "json,csv,markdown"],
            "conformance": ["-m", "fy_conformance.cli", "-c", str(path)],
            "integrity": ["-m", "fy_integrity.cli", "run", "-c", str(path)],
            "canary-baseline": ["-m", "fy_canary.cli", "baseline", "-c", str(path)],
            "canary-audit": ["-m", "fy_canary.cli", "audit", "-c", str(path), "--ignore-stale-baseline"],
            "image-loadtest": ["-m", "fy_image_loadtest.cli", "-c", str(path)],
            "image-conformance": ["-m", "fy_image_conformance.cli", str(path)],
            "image-canary": ["-m", "fy_image_canary.cli", "-c", str(path)],
        }[step]
        return path, [sys.executable, *module_args]

    def _smoke_config(self, model: ModelTarget) -> dict[str, Any]:
        return {
            "gateway": self._gateway(include_admin=True),
            "test": {
                "concurrency": 1 if self.cfg.profile.mode == "quick" else 4,
                "reps_per_case": 1 if self.cfg.profile.mode == "quick" else 3,
                "timeout_seconds": 90,
                "max_tokens": 64,
                "stream": True,
                "non_stream": True,
                "pin_channel": True,
                "prompt": "Reply with the single word: pong.",
            },
            "channels": [{
                "id": self.cfg.target.channel_id,
                "name": self.cfg.channel_name,
                "test_models": [model.id],
            }],
            "export": {"formats": ["json", "csv"], "output_dir": str(self.run_dir / "smoke-results")},
        }

    def _loadtest_config(self, model: ModelTarget) -> dict[str, Any]:
        strict = self.cfg.profile.mode == "strict" or self.cfg.profile.strict
        quick = self.cfg.profile.mode == "quick"
        return {
            "gateway": {
                "base_url": self.cfg.gateway.base_url,
                "user_token": self.cfg.gateway.tokens.user,
                "channels": [{"name": self.cfg.channel_name, "pin_channel_id": self.cfg.target.channel_id}],
            },
            "load": {
                "model": model.id,
                "prompt": "Reply with the single word: pong.",
                "max_tokens": 128 if strict else 64,
                "stream": True,
                "concurrency_levels": [1, 10] if quick else ([1, 10, 30, 50, 100, 200] if strict else [1, 10, 30, 50, 100]),
                "requests_per_level": 10 if quick else (50 if strict else 30),
                "warmup_requests": 1 if quick else 3,
                "request_timeout_sec": 180 if strict else 120,
            },
            "export": {
                "formats": ["json", "csv", "markdown"],
                "output_dir": str(self.run_dir / "loadtest-results"),
            },
        }

    def _quality_config(self, model: ModelTarget) -> dict[str, Any]:
        judges = []
        if self.cfg.judge.enabled and self.cfg.judge.model and self.cfg.gateway.tokens.judge:
            judges.append({
                "label": "judge",
                "base_url": self.cfg.judge.base_url or self.cfg.gateway.base_url,
                "api_key": self.cfg.gateway.tokens.judge,
                "model": self.cfg.judge.model,
            })
        embedding = None
        if self.cfg.embedding.enabled and self.cfg.embedding.model and self.cfg.gateway.tokens.embedding:
            embedding = {
                "base_url": self.cfg.embedding.base_url or self.cfg.gateway.base_url,
                "api_key": self.cfg.gateway.tokens.embedding,
                "model": self.cfg.embedding.model,
            }
        cfg: dict[str, Any] = {
            "channels": [{
                "name": self.cfg.channel_name,
                "model": model.id,
                "token": self.cfg.gateway.tokens.user,
                "base_url": self.cfg.gateway.base_url,
                "pin_channel_id": self.cfg.target.channel_id,
            }],
            "dataset": "fy_quality/datasets/public/quality.jsonl",
            "judges": judges,
            "concurrency": 2 if self.cfg.profile.mode == "quick" else 4,
            "request_timeout_sec": 120,
            "output_dir": str(self.run_dir / "quality-results"),
            "cache_dir": str(self.run_dir / ".cache-quality"),
        }
        if embedding:
            cfg["embedding"] = embedding
        return cfg

    def _conformance_config(self, model: ModelTarget) -> dict[str, Any]:
        return {
            "gateway": {
                "base_url": self.cfg.gateway.base_url,
                "user_token": self.cfg.gateway.tokens.user,
                "pin_channel_id": self.cfg.target.channel_id,
            },
            "target": {
                "model": model.id,
                "backend": model.backend,
                "baseline_request": {
                    "messages": [{"role": "user", "content": "ping"}],
                    "max_tokens": 16,
                    "temperature": 0,
                },
            },
            "dataset": "fy_conformance/datasets/public/conformance.jsonl",
            "concurrency": 2 if self.cfg.profile.mode == "quick" else 4,
            "request_timeout_sec": 30,
            "output_dir": str(self.run_dir / "conformance-results"),
        }

    def _integrity_config(self, model: ModelTarget) -> dict[str, Any]:
        strict = self.cfg.profile.mode == "strict" or self.cfg.profile.strict
        return {
            "gateway": {
                "base_url": self.cfg.gateway.base_url,
                "user_token": self.cfg.gateway.tokens.user,
                "pin_channel_id": self.cfg.target.channel_id,
                "secondary_token": self.cfg.gateway.tokens.secondary,
            },
            "target": {"model": model.id, "max_tokens": 256, "request_timeout_sec": 120},
            "probes": {
                "cache": {"enabled": True, "rounds": 5 if strict else 3},
                "inflation": {"enabled": True, "tolerance_tokens": 5 if strict else 10},
                "determinism": {"enabled": True, "rounds": 5 if strict else 3, "min_consistency": 0.98 if strict else 0.95},
                "tool_use": {"enabled": True},
                "stream": {"enabled": True, "rounds": 3, "burst_threshold": 0.5},
                "filtering": {"enabled": True},
                "isolation": {"enabled": bool(self.cfg.gateway.tokens.secondary), "rounds": 3},
            },
            "export": {"formats": ["json", "markdown"], "output_dir": str(self.run_dir / "integrity-results")},
        }

    def _canary_config(self, model: ModelTarget, *, baseline: bool) -> dict[str, Any]:
        pin = self.cfg.target.baseline_channel_id if baseline else self.cfg.target.channel_id
        name = f"{model.id}-ch{self.cfg.target.channel_id}"
        cfg: dict[str, Any] = {
            "source": {
                "name": name,
                "base_url": self.cfg.gateway.base_url,
                "api_key": self.cfg.gateway.tokens.user,
                "model": model.id,
                "pin_channel_id": pin,
            },
            "dataset": "fy_canary/datasets/public/canaries.jsonl",
            "baselines_dir": str(self.run_dir / "canary-baselines"),
            "output_dir": str(self.run_dir / "canary-results"),
            "mmd_enabled": False,
            "request_timeout_sec": 120,
            "concurrency": 4,
        }
        if self.cfg.embedding.enabled and self.cfg.embedding.model and self.cfg.gateway.tokens.embedding:
            cfg["embedding"] = {
                "base_url": self.cfg.embedding.base_url or self.cfg.gateway.base_url,
                "api_key": self.cfg.gateway.tokens.embedding,
                "model": self.cfg.embedding.model,
            }
        return cfg

    def _image_loadtest_config(self, model: ModelTarget) -> dict[str, Any]:
        quick = self.cfg.profile.mode == "quick"
        strict = self.cfg.profile.mode == "strict" or self.cfg.profile.strict
        return {
            "gateway": {
                "base_url": self.cfg.gateway.base_url,
                "user_token": self.cfg.gateway.tokens.user,
                "channels": [{"name": self.cfg.channel_name, "pin_channel_id": self.cfg.target.channel_id}],
            },
            "image": {
                "model": model.id,
                "prompt": "a red apple on a white table, studio lighting",
                "size": "1024x1024",
                "quality": "low",
                "n": 1,
                "concurrency_per_channel": 1 if quick else 2,
                "request_timeout_sec": 300,
                "continuous": False,
                "max_requests_per_channel": 5 if quick else (30 if strict else 15),
            },
            "export": {"formats": ["json", "csv", "markdown"], "output_dir": str(self.run_dir / "image-loadtest-results")},
        }

    def _image_conformance_config(self, model: ModelTarget) -> dict[str, Any]:
        quick = self.cfg.profile.mode == "quick"
        return {
            "gateway": {
                "base_url": self.cfg.gateway.base_url,
                "user_token": self.cfg.gateway.tokens.user,
                "channels": [{"name": self.cfg.channel_name, "pin_channel_id": self.cfg.target.channel_id}],
            },
            "model": {"name": model.id, "default_prompt": "a red apple on a white table, studio lighting"},
            "suites": {
                "api_compat": True,
                "output_valid": True,
                "prompt_follow": {"enabled": False},
                "perf": {"enabled": not quick, "duration_sec": 60, "max_requests_per_channel": 10},
                "safety": not quick,
            },
            "export": {"output_dir": str(self.run_dir / "image-conformance-results")},
        }

    def _image_canary_config(self, model: ModelTarget) -> dict[str, Any]:
        return {
            "gateway": {
                "name": self.cfg.channel_name,
                "base_url": self.cfg.gateway.base_url,
                "user_token": self.cfg.gateway.tokens.user,
                "pin_channel_id": self.cfg.target.channel_id,
                "model": model.id,
            },
            "output_dir": str(self.run_dir / "image-canary-results"),
            "request_timeout_sec": 300,
            "concurrency": 2,
        }

    def _gateway(self, *, include_admin: bool = False) -> dict[str, Any]:
        data = {
            "base_url": self.cfg.gateway.base_url,
            "user_token": self.cfg.gateway.tokens.user,
        }
        if include_admin:
            data["admin_token"] = self.cfg.gateway.tokens.admin or "unused"
            data["admin_user_id"] = self.cfg.gateway.tokens.admin_user_id
        return data

    def _score(self, run: BenchmarkRun) -> None:
        score_json = self.run_dir / "reports" / "scorecard.json"
        score_md = self.run_dir / "reports" / "scorecard.md"
        command = [
            sys.executable, "-m", "fy_score.cli",
            "--smoke-dir", str(self.run_dir / "smoke-results"),
            "--loadtest-dir", str(self.run_dir / "loadtest-results"),
            "--quality-dir", str(self.run_dir / "quality-results"),
            "--canary-dir", str(self.run_dir / "canary-results"),
            "--conformance-dir", str(self.run_dir / "conformance-results"),
            "--integrity-dir", str(self.run_dir / "integrity-results"),
            "--image-loadtest-dir", str(self.run_dir / "image-loadtest-results"),
            "--image-conformance-dir", str(self.run_dir / "image-conformance-results"),
            "--image-canary-dir", str(self.run_dir / "image-canary-results"),
            "--channel-id", str(self.cfg.target.channel_id),
            "--channel-name", self.cfg.channel_name,
            "--output", str(score_json),
            "--markdown", str(score_md),
        ]
        log_path = self.log_dir / "score.log"
        start = time.perf_counter()
        with log_path.open("w", encoding="utf-8") as log:
            proc = subprocess.run(
                command,
                cwd=Path(__file__).resolve().parents[1],
                text=True,
                stdout=log,
                stderr=subprocess.STDOUT,
                check=False,
            )
        run.results.append(StepResult(
            name="score",
            model="*",
            command=command,
            config_path="",
            log_path=str(log_path),
            elapsed_s=time.perf_counter() - start,
            returncode=proc.returncode,
        ))
        if proc.returncode == 0:
            run.score_json = score_json
            run.score_markdown = score_md
            self.console.print(f"[green]scorecard[/green] {score_md}")
        else:
            self.console.print(f"[red]score failed[/red] log={log_path}")

    def _ensure_dirs(self) -> None:
        self.config_dir.mkdir(parents=True, exist_ok=True)
        self.log_dir.mkdir(parents=True, exist_ok=True)
        (self.run_dir / "reports").mkdir(parents=True, exist_ok=True)

    def _write_manifest(self, *, dry_run: bool) -> None:
        manifest = {
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "dry_run": dry_run,
            "source_config": str(self.config_path),
            "channel_id": self.cfg.target.channel_id,
            "channel_name": self.cfg.channel_name,
            "mode": self.cfg.profile.mode,
            "models": [m.__dict__ for m in self.cfg.target.models],
            "planned_steps": [{"model": m.id, "type": m.type, "step": s} for s, m in self.plan()],
        }
        (self.run_dir / "manifest.json").write_text(
            json.dumps(manifest, indent=2, ensure_ascii=False),
            encoding="utf-8",
        )

    def _write_run_report(self, run: BenchmarkRun) -> None:
        data = {
            "run_dir": str(run.run_dir),
            "score_json": str(run.score_json) if run.score_json else "",
            "score_markdown": str(run.score_markdown) if run.score_markdown else "",
            "steps": [r.__dict__ for r in run.results],
        }
        (self.run_dir / "run-summary.json").write_text(
            json.dumps(data, indent=2, ensure_ascii=False),
            encoding="utf-8",
        )
        lines = ["# Benchmark Run Summary", ""]
        lines.append(f"- Channel: `{self.cfg.channel_name}` (`{self.cfg.target.channel_id}`)")
        lines.append(f"- Mode: `{self.cfg.profile.mode}`")
        lines.append(f"- Run dir: `{self.run_dir}`")
        if run.score_markdown:
            lines.append(f"- Scorecard: `{run.score_markdown}`")
        lines.append("")
        lines.append("| Step | Model | Status | Seconds | Log |")
        lines.append("|---|---|---:|---:|---|")
        for r in run.results:
            status = "SKIP" if r.skipped else ("PASS" if r.returncode == 0 else f"FAIL({r.returncode})")
            lines.append(f"| {r.name} | {r.model} | {status} | {r.elapsed_s:.1f} | `{r.log_path}` |")
        (self.run_dir / "run-summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
