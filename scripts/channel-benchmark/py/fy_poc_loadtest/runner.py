"""POC load-test runner built on fy_loadtest's client and metrics."""

from __future__ import annotations

import asyncio
import time
from dataclasses import dataclass, field

from rich.console import Console

from fy_loadtest.client import ChatClient, ChatResult
from fy_loadtest.metrics import LevelAggregate, aggregate_level

from .config import Config, Scenario


@dataclass
class ScenarioResult:
    scenario: Scenario
    levels: list[LevelAggregate] = field(default_factory=list)


@dataclass
class ChannelResult:
    channel_name: str
    pin_channel_id: int | None
    scenarios: list[ScenarioResult] = field(default_factory=list)


@dataclass
class ModelResult:
    model: str
    channels: list[ChannelResult] = field(default_factory=list)


@dataclass
class PocResult:
    base_url: str
    model_results: list[ModelResult] = field(default_factory=list)


class PocRunner:
    def __init__(self, cfg: Config, *, console: Console | None = None):
        self.cfg = cfg
        self.console = console or Console()

    async def run(self) -> PocResult:
        result = PocResult(base_url=self.cfg.gateway.base_url)
        for model in self.cfg.poc.models:
            self.console.rule(f"[bold blue]模型: {model}")
            result.model_results.append(await self._run_model(model))
        return result

    async def _run_model(self, model: str) -> ModelResult:
        channels = self.cfg.gateway.channels
        if not channels:
            return ModelResult(
                model=model,
                channels=[await self._run_channel(model, channel_name="default", pin_channel_id=None)],
            )
        out: list[ChannelResult] = []
        for ch in channels:
            self.console.rule(f"[bold magenta]渠道: {ch.name} (id={ch.pin_channel_id})")
            out.append(await self._run_channel(model, channel_name=ch.name, pin_channel_id=ch.pin_channel_id))
        return ModelResult(model=model, channels=out)

    async def _run_channel(
        self,
        model: str,
        *,
        channel_name: str,
        pin_channel_id: int | None,
    ) -> ChannelResult:
        channel_result = ChannelResult(channel_name=channel_name, pin_channel_id=pin_channel_id)
        async with ChatClient(
            self.cfg.gateway.base_url,
            self.cfg.gateway.user_token,
            request_timeout=self.cfg.poc.request_timeout_sec,
            pin_channel_id=pin_channel_id,
        ) as client:
            for scenario in self.cfg.poc.scenarios:
                self.console.rule(f"[bold cyan]场景: {scenario.name}")
                sr = ScenarioResult(scenario=scenario)
                for idx, concurrency in enumerate(self.cfg.concurrency_levels_for(scenario)):
                    if idx > 0 and self.cfg.poc.sleep_between_levels_sec > 0:
                        await asyncio.sleep(self.cfg.poc.sleep_between_levels_sec)
                    sr.levels.append(await self._measure_level(client, model, scenario, concurrency))
                channel_result.scenarios.append(sr)
        return channel_result

    async def _measure_level(
        self,
        client: ChatClient,
        model: str,
        scenario: Scenario,
        concurrency: int,
    ) -> LevelAggregate:
        total = self.cfg.requests_for(scenario, concurrency)
        self.console.print(
            f"[cyan]measure[/cyan] scenario={scenario.name} concurrency={concurrency} "
            f"requests={total} max_tokens={scenario.max_tokens}"
        )
        if self.cfg.poc.warmup_requests > 0:
            await self._fire(client, model, scenario, concurrency, self.cfg.poc.warmup_requests)
        t0 = time.monotonic()
        results = await self._fire(client, model, scenario, concurrency, total)
        wall = time.monotonic() - t0
        agg = aggregate_level(concurrency=concurrency, results=results, wall_time_s=wall)
        self.console.print(
            f"  ok={agg.ok}/{agg.total} succ={agg.success_rate_pct:.1f}% "
            f"ttft_avg={agg.ttft.avg_ms:.0f}ms tpot_avg={agg.tpot.avg_ms:.1f}ms "
            f"lat_avg={agg.e2e.avg_ms:.0f}ms tok/s={agg.per_request_tok_per_s.avg:.1f}"
        )
        return agg

    async def _fire(
        self,
        client: ChatClient,
        model: str,
        scenario: Scenario,
        concurrency: int,
        total: int,
    ) -> list[ChatResult]:
        sem = asyncio.Semaphore(concurrency)
        results: list[ChatResult] = []
        lock = asyncio.Lock()

        async def one() -> None:
            async with sem:
                r = await client.chat(
                    model=model,
                    prompt=scenario.prompt,
                    max_tokens=scenario.max_tokens,
                    temperature=self.cfg.poc.temperature,
                    stream=self.cfg.poc.stream,
                )
                async with lock:
                    results.append(r)

        await asyncio.gather(*(asyncio.create_task(one()) for _ in range(total)))
        return results
