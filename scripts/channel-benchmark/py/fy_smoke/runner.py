"""Smoke runner for channel x model x stream-mode checks."""

from __future__ import annotations

import asyncio
from collections import defaultdict

from rich.console import Console

from fy_loadtest.client import ChatClient, ChatResult

from .admin import AdminClient
from .config import SmokeConfig
from .metrics import Aggregate, CaseKey, aggregate_results


class SmokeRunner:
    def __init__(self, cfg: SmokeConfig, *, console: Console | None = None):
        self.cfg = cfg
        self.console = console or Console()

    async def run(self) -> list[Aggregate]:
        async with AdminClient(
            self.cfg.gateway.base_url,
            self.cfg.gateway.admin_token,
            self.cfg.gateway.admin_user_id,
        ) as admin:
            catalog = await admin.list_channels(only_enabled=False)
        by_id = {ch.id: ch for ch in catalog}

        jobs: list[tuple[int, str, str, bool]] = []
        for configured in self.cfg.channels:
            channel = by_id.get(configured.id)
            if channel is None:
                self.console.print(
                    f"  ! channel id={configured.id} ({configured.name!r}) not found in gateway, skipping"
                )
                continue
            if channel.status != 1:
                self.console.print(
                    f"  ! channel id={channel.id} ({channel.name!r}) is disabled (status={channel.status}), testing anyway"
                )
            channel_name = channel.name or configured.name or f"channel-{configured.id}"
            for model in configured.test_models:
                if self.cfg.test.stream:
                    jobs.append((channel.id, channel_name, model, True))
                if self.cfg.test.non_stream:
                    jobs.append((channel.id, channel_name, model, False))
        if not jobs:
            raise RuntimeError("no valid cases to run")

        total_requests = len(jobs) * self.cfg.test.reps_per_case
        pin_note = (
            "channel pinned via admin token suffix"
            if self.cfg.test.pin_channel
            else "via distributor (no channel pin; model overlap may skew results)"
        )
        self.console.print(
            f"Plan: {len(jobs)} cases x {self.cfg.test.reps_per_case} reps = "
            f"{total_requests} requests at concurrency={self.cfg.test.concurrency} ({pin_note})"
        )

        sem = asyncio.Semaphore(self.cfg.test.concurrency)
        buckets: dict[CaseKey, list[ChatResult]] = defaultdict(list)
        lock = asyncio.Lock()
        completed = 0

        async def one(channel_id: int, channel_name: str, model: str, streamed: bool, rep: int) -> None:
            nonlocal completed
            async with sem:
                pin = channel_id if self.cfg.test.pin_channel else None
                async with ChatClient(
                    self.cfg.gateway.base_url,
                    self.cfg.gateway.user_token,
                    request_timeout=float(self.cfg.test.timeout_seconds),
                    pin_channel_id=pin,
                ) as client:
                    result = await client.chat(
                        model=model,
                        prompt=self.cfg.test.prompt,
                        max_tokens=self.cfg.test.max_tokens,
                        temperature=None,
                        stream=streamed,
                    )
                key = CaseKey(channel_id, channel_name, model, streamed)
                async with lock:
                    buckets[key].append(result)
                    completed += 1
                    status = "ok" if result.success else f"FAIL: {(result.error or '')[:120]}"
                    self.console.print(
                        f"  [{completed}/{total_requests}] ch={channel_id} {model} "
                        f"stream={streamed} rep={rep} E2E={result.e2e_s*1000:.0f}ms "
                        f"TTFT={result.ttft_s*1000:.0f}ms tok={result.usage.completion_tokens} -> {status}"
                    )

        tasks = [
            asyncio.create_task(one(channel_id, channel_name, model, streamed, rep + 1))
            for channel_id, channel_name, model, streamed in jobs
            for rep in range(self.cfg.test.reps_per_case)
        ]
        await asyncio.gather(*tasks)
        return [aggregate_results(key, results) for key, results in buckets.items()]
