"""Single-file config for the unified channel benchmark runner."""

from __future__ import annotations

import os
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

_ENV_RE = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}")


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
        raise ValueError(f"undefined environment variables: {sorted(set(missing))}")
    return "\n".join(out)


@dataclass
class TokenConfig:
    user: str
    admin: str = ""
    admin_user_id: str = "1"
    judge: str = ""
    embedding: str = ""
    secondary: str = ""


@dataclass
class GatewayConfig:
    base_url: str
    tokens: TokenConfig


@dataclass
class ModelTarget:
    id: str
    type: str = "text"
    backend: str = "openai"


@dataclass
class TargetConfig:
    channel_id: int
    channel_name: str = ""
    models: list[ModelTarget] = field(default_factory=list)
    baseline_channel_id: int | None = None
    baseline_channel_name: str = ""


@dataclass
class JudgeConfig:
    enabled: bool = False
    model: str = ""
    base_url: str = ""


@dataclass
class EmbeddingConfig:
    enabled: bool = False
    model: str = ""
    base_url: str = ""


@dataclass
class ModuleConfig:
    smoke: bool = True
    loadtest: bool = True
    quality: bool = True
    conformance: bool = True
    integrity: bool = True
    canary: bool = False
    image_loadtest: bool = True
    image_conformance: bool = True
    image_canary: bool = False
    video: bool = False


@dataclass
class ProfileConfig:
    mode: str = "standard"
    parallel_models: int = 1
    output_dir: str = "benchmark-runs"
    formats: list[str] = field(default_factory=lambda: ["json", "csv", "markdown"])
    strict: bool = False


@dataclass
class BenchmarkConfig:
    gateway: GatewayConfig
    target: TargetConfig
    profile: ProfileConfig = field(default_factory=ProfileConfig)
    modules: ModuleConfig = field(default_factory=ModuleConfig)
    judge: JudgeConfig = field(default_factory=JudgeConfig)
    embedding: EmbeddingConfig = field(default_factory=EmbeddingConfig)
    raw: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def load(cls, path: str | Path) -> BenchmarkConfig:
        data = yaml.safe_load(_expand_env(Path(path).read_text(encoding="utf-8"))) or {}
        cfg = cls.from_dict(data)
        cfg.validate()
        return cfg

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> BenchmarkConfig:
        gw = data.get("gateway") or {}
        toks = gw.get("tokens") or data.get("tokens") or {}
        target = data.get("target") or {}
        profile = data.get("profile") or {}
        modules = data.get("modules") or {}
        judge = data.get("judge") or {}
        embedding = data.get("embedding") or {}

        models = [
            ModelTarget(
                id=str(m.get("id", m.get("model", ""))),
                type=str(m.get("type", target.get("type", "text"))),
                backend=str(m.get("backend", m.get("provider", "openai"))),
            )
            if isinstance(m, dict)
            else ModelTarget(id=str(m), type=str(target.get("type", "text")))
            for m in (target.get("models") or [])
        ]
        if target.get("model"):
            models.append(ModelTarget(
                id=str(target["model"]),
                type=str(target.get("type", "text")),
                backend=str(target.get("backend", "openai")),
            ))

        mode = str(profile.get("mode", data.get("mode", "standard")))
        return cls(
            gateway=GatewayConfig(
                base_url=str(gw.get("base_url", "")),
                tokens=TokenConfig(
                    user=str(toks.get("user", toks.get("user_token", ""))),
                    admin=str(toks.get("admin", toks.get("admin_token", ""))),
                    admin_user_id=str(toks.get("admin_user_id", "1")),
                    judge=str(toks.get("judge", toks.get("judge_token", ""))),
                    embedding=str(toks.get("embedding", toks.get("embedding_token", ""))),
                    secondary=str(toks.get("secondary", toks.get("secondary_token", ""))),
                ),
            ),
            target=TargetConfig(
                channel_id=int(target.get("channel_id", 0) or 0),
                channel_name=str(target.get("channel_name", "")),
                models=models,
                baseline_channel_id=(
                    int(target["baseline_channel_id"])
                    if target.get("baseline_channel_id") is not None
                    else None
                ),
                baseline_channel_name=str(target.get("baseline_channel_name", "")),
            ),
            profile=ProfileConfig(
                mode=mode,
                parallel_models=int(profile.get("parallel_models", 1)),
                output_dir=str(profile.get("output_dir", "benchmark-runs")),
                formats=[str(x) for x in profile.get("formats", ["json", "csv", "markdown"])],
                strict=bool(profile.get("strict", mode == "strict")),
            ),
            modules=ModuleConfig(
                smoke=bool(modules.get("smoke", True)),
                loadtest=bool(modules.get("loadtest", True)),
                quality=bool(modules.get("quality", True)),
                conformance=bool(modules.get("conformance", True)),
                integrity=bool(modules.get("integrity", True)),
                canary=bool(modules.get("canary", False)),
                image_loadtest=bool(modules.get("image_loadtest", True)),
                image_conformance=bool(modules.get("image_conformance", True)),
                image_canary=bool(modules.get("image_canary", False)),
                video=bool(modules.get("video", False)),
            ),
            judge=JudgeConfig(
                enabled=bool(judge.get("enabled", False)),
                model=str(judge.get("model", "")),
                base_url=str(judge.get("base_url", gw.get("base_url", ""))),
            ),
            embedding=EmbeddingConfig(
                enabled=bool(embedding.get("enabled", False)),
                model=str(embedding.get("model", "")),
                base_url=str(embedding.get("base_url", gw.get("base_url", ""))),
            ),
            raw=data,
        )

    def validate(self) -> None:
        if not self.gateway.base_url:
            raise ValueError("gateway.base_url is required")
        if not self.gateway.tokens.user:
            raise ValueError("gateway.tokens.user is required")
        if self.target.channel_id <= 0:
            raise ValueError("target.channel_id must be > 0")
        if not self.target.models:
            raise ValueError("target.models must contain at least one model")
        if self.profile.mode not in {"quick", "standard", "strict", "deep"}:
            raise ValueError("profile.mode must be quick, standard, strict, or deep")
        if self.profile.parallel_models != 1:
            raise ValueError("parallel_models > 1 is not implemented yet; keep it 1 for strict serial runs")
        for m in self.target.models:
            if not m.id:
                raise ValueError("target.models contains an empty model id")
            if m.type not in {"text", "image", "video"}:
                raise ValueError(f"model {m.id}: type must be text, image, or video")

    @property
    def channel_name(self) -> str:
        return self.target.channel_name or f"channel-{self.target.channel_id}"
