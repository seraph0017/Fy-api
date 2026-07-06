"""YAML config parsing for the channel smoke benchmark."""

from __future__ import annotations

import os
import re
from dataclasses import dataclass, field
from pathlib import Path

import yaml

_ENV_RE = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}")

LONG_THINKING_PROMPT = (
    "Prove that for every prime p > 3, there exist infinitely many integers n "
    "such that n^2 + 1 has at least one prime factor congruent to 1 mod p, "
    "and characterize the density of such n. Be rigorous: state every lemma "
    "you depend on, give a fully-stated proof, and conclude with a "
    "quantitative density estimate. End with QED on its own line."
)


def _expand_env(raw: str) -> str:
    missing: list[str] = []
    out: list[str] = []
    for line in raw.splitlines():
        if line.lstrip(" \t").startswith("#"):
            out.append(line)
            continue

        def sub(match: re.Match[str]) -> str:
            name, default = match.group(1), match.group(2)
            if name in os.environ:
                return os.environ[name]
            if default is not None:
                return default
            missing.append(name)
            return ""

        out.append(_ENV_RE.sub(sub, line))
    if missing:
        raise ValueError(
            f"config references undefined environment variables: {sorted(set(missing))}"
        )
    return "\n".join(out)


@dataclass
class GatewayConfig:
    base_url: str
    admin_token: str
    admin_user_id: str
    user_token: str


@dataclass
class TestConfig:
    concurrency: int = 4
    timeout_seconds: int = 60
    reps_per_case: int = 3
    stream: bool = True
    non_stream: bool = True
    max_tokens: int = 64
    prompt: str = "Reply with the single word: pong."
    pin_channel: bool = False


@dataclass
class ChannelConfig:
    id: int
    name: str = ""
    test_models: list[str] = field(default_factory=list)


@dataclass
class ExportConfig:
    formats: list[str] = field(default_factory=lambda: ["json"])
    output_dir: str = "benchmark-results"


@dataclass
class MetricsConfig:
    latency_percentiles: list[float] = field(default_factory=lambda: [50, 95, 99])


@dataclass
class SmokeConfig:
    gateway: GatewayConfig
    test: TestConfig = field(default_factory=TestConfig)
    channels: list[ChannelConfig] = field(default_factory=list)
    export: ExportConfig = field(default_factory=ExportConfig)
    metrics: MetricsConfig = field(default_factory=MetricsConfig)

    @classmethod
    def load(cls, path: str | Path) -> SmokeConfig:
        text = Path(path).read_text(encoding="utf-8")
        data = yaml.safe_load(_expand_env(text)) or {}
        return cls.from_dict(data)

    @classmethod
    def from_dict(cls, data: dict) -> SmokeConfig:
        gw = data.get("gateway") or {}
        test = data.get("test") or {}
        exp = data.get("export") or {}
        metrics = data.get("metrics") or {}
        channels = [
            ChannelConfig(
                id=int(ch.get("id", 0)),
                name=str(ch.get("name", "")),
                test_models=[str(m) for m in (ch.get("test_models") or [])],
            )
            for ch in (data.get("channels") or [])
        ]

        stream = bool(test.get("stream", False))
        non_stream = bool(test.get("non_stream", False))
        if not stream and not non_stream:
            stream = True
            non_stream = True

        cfg = cls(
            gateway=GatewayConfig(
                base_url=str(gw.get("base_url", "")),
                admin_token=str(gw.get("admin_token", "")),
                admin_user_id=str(gw.get("admin_user_id", "")),
                user_token=str(gw.get("user_token", "")),
            ),
            test=TestConfig(
                concurrency=int(test.get("concurrency", 4) or 4),
                timeout_seconds=int(test.get("timeout_seconds", 60) or 60),
                reps_per_case=int(test.get("reps_per_case", 3) or 3),
                stream=stream,
                non_stream=non_stream,
                max_tokens=int(test.get("max_tokens", 64) or 64),
                prompt=str(test.get("prompt") or "Reply with the single word: pong."),
                pin_channel=bool(test.get("pin_channel", False)),
            ),
            channels=channels,
            export=ExportConfig(
                formats=[str(x) for x in exp.get("formats", ["json"])],
                output_dir=str(exp.get("output_dir") or "benchmark-results"),
            ),
            metrics=MetricsConfig(
                latency_percentiles=[float(x) for x in metrics.get("latency_percentiles", [50, 95, 99])]
            ),
        )
        cfg.validate()
        return cfg

    def validate(self) -> None:
        if not self.gateway.base_url:
            raise ValueError("gateway.base_url is required")
        if not self.gateway.admin_token:
            raise ValueError("gateway.admin_token is required (used for GET /api/channel/)")
        if not self.gateway.admin_user_id:
            raise ValueError("gateway.admin_user_id is required (New-Api-User header)")
        if not self.gateway.user_token:
            raise ValueError("gateway.user_token is required (used for /v1/chat/completions)")
        if not self.channels:
            raise ValueError("no channels configured")
        for i, ch in enumerate(self.channels):
            if ch.id <= 0:
                raise ValueError(f"channels[{i}]: id must be > 0")
            if not ch.test_models:
                raise ValueError(
                    f"channels[{i}] (id={ch.id}): test_models is empty; specify at least one model"
                )
        if self.test.concurrency <= 0:
            raise ValueError("test.concurrency must be > 0")
        if self.test.reps_per_case <= 0:
            raise ValueError("test.reps_per_case must be > 0")
        bad_formats = set(self.export.formats) - {"json", "csv"}
        if bad_formats:
            raise ValueError(f"unknown export formats: {sorted(bad_formats)}")

    def apply_long_thinking(self) -> None:
        self.test.prompt = LONG_THINKING_PROMPT
        self.test.timeout_seconds = 1800
        self.test.max_tokens = 32000
        self.test.reps_per_case = 1
        self.test.concurrency = 1
        self.test.stream = True
        self.test.non_stream = False
