"""Config parser for the POC load-test runner."""

from __future__ import annotations

import os
import re
from dataclasses import dataclass, field
from pathlib import Path

import yaml

from fy_loadtest.config import ChannelTarget

_ENV_RE = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}")
_DEFAULT_CONCURRENCIES = [1, 10, 20, 30, 40, 50, 64, 80, 128, 256]


def _expand_env(raw: str) -> str:
    missing: list[str] = []

    def _sub(m: re.Match[str]) -> str:
        name, default = m.group(1), m.group(2)
        if name in os.environ:
            return os.environ[name]
        if default is not None:
            return default
        missing.append(name)
        return ""

    lines: list[str] = []
    for line in raw.splitlines():
        if line.lstrip().startswith("#"):
            lines.append(line)
        else:
            lines.append(_ENV_RE.sub(_sub, line))
    if missing:
        raise ValueError(f"config references undefined environment variables: {sorted(set(missing))}")
    return "\n".join(lines)


def default_requests_for_concurrency(concurrency: int) -> int:
    if concurrency == 1:
        return 50
    if concurrency == 10:
        return 100
    if concurrency in (20, 30, 40):
        return 200
    if concurrency in (50, 64):
        return 250
    if concurrency == 80:
        return 300
    if concurrency == 128:
        return 350
    return 500


@dataclass
class Gateway:
    base_url: str
    user_token: str
    channels: list[ChannelTarget] = field(default_factory=list)


@dataclass
class Scenario:
    name: str
    description: str = ""
    dataset: str = "custom"
    prompt: str = ""
    dataset_path: str | None = None
    input_tokens: int | None = None
    max_tokens: int = 1024
    min_tokens: int | None = None
    concurrency_levels: list[int] | None = None
    requests_by_concurrency: dict[int, int] = field(default_factory=dict)


@dataclass
class PocProfile:
    platform_name: str = "TraceNex"
    report_title: str = "LLM性能验证报告"
    report_id: str = ""
    models: list[str] = field(default_factory=list)
    scenarios: list[Scenario] = field(default_factory=list)
    concurrency_levels: list[int] = field(default_factory=lambda: list(_DEFAULT_CONCURRENCIES))
    requests_by_concurrency: dict[int, int] = field(default_factory=dict)
    warmup_requests: int = 0
    request_timeout_sec: float = 600.0
    temperature: float | None = 0.6
    stream: bool = True
    sleep_between_levels_sec: float = 60.0
    tokenizer_path: str = ""
    test_scope: str = "短文本 / 中文本 / 长文本不同并发性能测试"
    environment: dict[str, str] = field(default_factory=dict)


@dataclass
class ExportConfig:
    formats: list[str] = field(default_factory=lambda: ["json", "csv", "markdown"])
    output_dir: str = "poc-loadtest-results"


@dataclass
class Config:
    gateway: Gateway
    poc: PocProfile
    export: ExportConfig = field(default_factory=ExportConfig)

    @classmethod
    def load(cls, path: str | Path) -> Config:
        text = Path(path).read_text(encoding="utf-8")
        data = yaml.safe_load(_expand_env(text)) or {}
        return cls._from_dict(data)

    @classmethod
    def _from_dict(cls, d: dict) -> Config:
        gw = d.get("gateway") or {}
        if not gw.get("base_url"):
            raise ValueError("gateway.base_url is required")
        if not gw.get("user_token"):
            raise ValueError("gateway.user_token is required")

        channels: list[ChannelTarget] = []
        for ch in gw.get("channels") or []:
            channels.append(ChannelTarget(
                name=str(ch.get("name", f"channel-{ch['pin_channel_id']}")),
                pin_channel_id=int(ch["pin_channel_id"]),
            ))
        if gw.get("pin_channel_id") is not None and not channels:
            pin = int(gw["pin_channel_id"])
            channels.append(ChannelTarget(name=f"channel-{pin}", pin_channel_id=pin))

        raw_poc = d.get("poc") or {}
        models = [str(x) for x in raw_poc.get("models") or []]
        if raw_poc.get("model"):
            models = [str(raw_poc["model"])]
        if not models:
            raise ValueError("poc.models or poc.model is required")

        scenarios = [
            Scenario(
                name=str(sc["name"]),
                description=str(sc.get("description", "")),
                dataset=str(sc.get("dataset", "custom")),
                prompt=str(sc.get("prompt", "")),
                dataset_path=sc.get("dataset_path"),
                input_tokens=int(sc["input_tokens"]) if sc.get("input_tokens") is not None else None,
                max_tokens=int(sc.get("max_tokens", raw_poc.get("max_tokens", 1024))),
                min_tokens=int(sc["min_tokens"]) if sc.get("min_tokens") is not None else None,
                concurrency_levels=[int(x) for x in sc["concurrency_levels"]] if sc.get("concurrency_levels") else None,
                requests_by_concurrency={int(k): int(v) for k, v in (sc.get("requests_by_concurrency") or {}).items()},
            )
            for sc in raw_poc.get("scenarios") or []
        ]
        if not scenarios:
            raise ValueError("poc.scenarios must have at least one scenario")

        raw_reqs = raw_poc.get("requests_by_concurrency") or {}
        reqs = {int(k): int(v) for k, v in raw_reqs.items()}
        raw_env = raw_poc.get("environment") or {}

        exp = d.get("export") or {}
        return cls(
            gateway=Gateway(
                base_url=str(gw["base_url"]),
                user_token=str(gw["user_token"]),
                channels=channels,
            ),
            poc=PocProfile(
                platform_name=str(raw_poc.get("platform_name", "TraceNex")),
                report_title=str(raw_poc.get("report_title", "LLM性能验证报告")),
                report_id=str(raw_poc.get("report_id", "")),
                models=models,
                scenarios=scenarios,
                concurrency_levels=[int(x) for x in raw_poc.get("concurrency_levels", _DEFAULT_CONCURRENCIES)],
                requests_by_concurrency=reqs,
                warmup_requests=int(raw_poc.get("warmup_requests", 0)),
                request_timeout_sec=float(raw_poc.get("request_timeout_sec", 600.0)),
                temperature=float(raw_poc["temperature"]) if raw_poc.get("temperature") is not None else None,
                stream=bool(raw_poc.get("stream", True)),
                sleep_between_levels_sec=float(raw_poc.get("sleep_between_levels_sec", 60.0)),
                tokenizer_path=str(raw_poc.get("tokenizer_path", "")),
                test_scope=str(raw_poc.get("test_scope", "短文本 / 中文本 / 长文本不同并发性能测试")),
                environment={str(k): str(v) for k, v in raw_env.items()},
            ),
            export=ExportConfig(
                formats=[str(x) for x in exp.get("formats", ["json", "csv", "markdown"])],
                output_dir=str(exp.get("output_dir", "poc-loadtest-results")),
            ),
        )

    def validate(self) -> None:
        if any(c <= 0 for c in self.poc.concurrency_levels):
            raise ValueError(f"poc.concurrency_levels must be positive: {self.poc.concurrency_levels}")
        if any(v <= 0 for v in self.poc.requests_by_concurrency.values()):
            raise ValueError("poc.requests_by_concurrency values must be positive")
        if self.poc.warmup_requests < 0:
            raise ValueError("poc.warmup_requests cannot be negative")
        for scenario in self.poc.scenarios:
            levels = scenario.concurrency_levels or self.poc.concurrency_levels
            if any(c <= 0 for c in levels):
                raise ValueError(f"scenario {scenario.name} concurrency_levels must be positive: {levels}")
            if any(v <= 0 for v in scenario.requests_by_concurrency.values()):
                raise ValueError(f"scenario {scenario.name} requests_by_concurrency values must be positive")
        bad_formats = set(self.export.formats) - {"json", "csv", "markdown"}
        if bad_formats:
            raise ValueError(f"unsupported export.formats: {sorted(bad_formats)}")

    def concurrency_levels_for(self, scenario: Scenario) -> list[int]:
        return scenario.concurrency_levels or self.poc.concurrency_levels

    def requests_for(self, scenario: Scenario, concurrency: int) -> int:
        if concurrency in scenario.requests_by_concurrency:
            return scenario.requests_by_concurrency[concurrency]
        return self.poc.requests_by_concurrency.get(
            concurrency,
            default_requests_for_concurrency(concurrency),
        )
