from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from fy_benchmark.config import BenchmarkConfig
from fy_benchmark.runner import BenchmarkRunner


def _config(tmp_path: Path, *, model_type: str = "text") -> Path:
    path = tmp_path / "benchmark.yaml"
    path.write_text(
        f"""
gateway:
  base_url: http://mock
  tokens:
    user: sk-test
target:
  channel_id: 42
  channel_name: test-channel
  models:
    - id: test-model
      type: {model_type}
      backend: openai
profile:
  mode: standard
  output_dir: "{tmp_path / 'runs'}"
modules:
  smoke: true
  loadtest: true
  quality: true
  conformance: true
  integrity: true
""",
        encoding="utf-8",
    )
    return path


def test_single_config_parses_text_model(tmp_path: Path):
    cfg = BenchmarkConfig.load(_config(tmp_path))
    assert cfg.gateway.base_url == "http://mock"
    assert cfg.gateway.tokens.user == "sk-test"
    assert cfg.target.channel_id == 42
    assert cfg.target.models[0].id == "test-model"
    assert cfg.target.models[0].type == "text"


def test_plan_is_serial_text_suite(tmp_path: Path):
    path = _config(tmp_path)
    cfg = BenchmarkConfig.load(path)
    runner = BenchmarkRunner(cfg, config_path=path)
    assert [(step, model.id) for step, model in runner.plan()] == [
        ("smoke", "test-model"),
        ("conformance", "test-model"),
        ("integrity", "test-model"),
        ("loadtest", "test-model"),
        ("quality", "test-model"),
    ]


def test_smoke_skips_without_admin_token(tmp_path: Path):
    path = _config(tmp_path)
    cfg = BenchmarkConfig.load(path)
    runner = BenchmarkRunner(cfg, config_path=path)
    run = runner.dry_run()
    skipped = [r for r in run.results if r.skipped]
    assert len(skipped) == 1
    assert skipped[0].name == "smoke"
    assert "admin" in skipped[0].reason


def test_generates_child_configs(tmp_path: Path):
    path = _config(tmp_path)
    cfg = BenchmarkConfig.load(path)
    runner = BenchmarkRunner(cfg, config_path=path)
    model = cfg.target.models[0]

    load_path, load_cmd = runner._build_step("loadtest", model)
    load_cfg = yaml.safe_load(load_path.read_text(encoding="utf-8"))
    assert load_cfg["gateway"]["channels"][0]["pin_channel_id"] == 42
    assert load_cfg["load"]["model"] == "test-model"
    assert load_cmd[:3] == [pytest.importorskip("sys").executable, "-m", "fy_loadtest.cli"]

    quality_path, _ = runner._build_step("quality", model)
    quality_cfg = yaml.safe_load(quality_path.read_text(encoding="utf-8"))
    assert quality_cfg["channels"][0]["pin_channel_id"] == 42
    assert quality_cfg["judges"] == []
    assert "embedding" not in quality_cfg
