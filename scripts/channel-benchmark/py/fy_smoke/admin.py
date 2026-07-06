"""Admin API client for resolving channel metadata."""

from __future__ import annotations

from dataclasses import dataclass

import httpx


@dataclass
class Channel:
    id: int
    type: int = 0
    name: str = ""
    status: int = 0
    group: str = ""
    models: str = ""
    test_model: str | None = None
    response_time: int = 0
    test_time: int = 0
    priority: int | None = None
    weight: int | None = None


class AdminClient:
    def __init__(
        self,
        base_url: str,
        token: str,
        user_id: str,
        *,
        timeout: float = 30.0,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._client = httpx.AsyncClient(
            timeout=timeout,
            headers={
                "Authorization": token,
                "New-Api-User": user_id,
                "Accept": "application/json",
            },
        )

    async def aclose(self) -> None:
        await self._client.aclose()

    async def __aenter__(self) -> AdminClient:
        return self

    async def __aexit__(self, *exc) -> None:
        await self.aclose()

    async def list_channels(self, *, only_enabled: bool = False) -> list[Channel]:
        all_channels: list[Channel] = []
        page = 1
        page_size = 200
        while True:
            items, total = await self._fetch_page(page=page, page_size=page_size)
            for ch in items:
                if only_enabled and ch.status != 1:
                    continue
                all_channels.append(ch)
            if page * page_size >= total or not items:
                break
            page += 1
        return all_channels

    async def _fetch_page(self, *, page: int, page_size: int) -> tuple[list[Channel], int]:
        resp = await self._client.get(
            f"{self._base_url}/api/channel/",
            params={"p": page, "page_size": page_size},
        )
        body = resp.text
        if resp.status_code != 200:
            raise RuntimeError(
                f"admin GET /api/channel/ returned HTTP {resp.status_code}: {body[:400]}"
            )
        payload = resp.json()
        if not payload.get("success"):
            raise RuntimeError(f"admin API success=false: {payload.get('message', '')}")
        data = payload.get("data") or {}
        return [_channel_from_dict(ch) for ch in data.get("items") or []], int(data.get("total") or 0)


def _channel_from_dict(data: dict) -> Channel:
    return Channel(
        id=int(data.get("id") or 0),
        type=int(data.get("type") or 0),
        name=str(data.get("name") or ""),
        status=int(data.get("status") or 0),
        group=str(data.get("group") or ""),
        models=str(data.get("models") or ""),
        test_model=data.get("test_model"),
        response_time=int(data.get("response_time") or 0),
        test_time=int(data.get("test_time") or 0),
        priority=data.get("priority"),
        weight=data.get("weight"),
    )
