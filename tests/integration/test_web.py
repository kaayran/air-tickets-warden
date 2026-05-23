from __future__ import annotations

import pytest
from aiohttp.test_utils import TestClient, TestServer
from sqlalchemy.ext.asyncio import AsyncEngine

from warden.infrastructure import db as db_module
from warden.infrastructure.web import make_app


@pytest.mark.asyncio
async def test_health_ok(engine: AsyncEngine) -> None:
    app = make_app(engine)
    async with TestClient(TestServer(app)) as client:
        resp = await client.get("/health")
        assert resp.status == 200
        body = await resp.json()
        assert body == {"status": "ok", "db": "ok"}


@pytest.mark.asyncio
async def test_health_503_when_db_down(
    engine: AsyncEngine, monkeypatch: pytest.MonkeyPatch
) -> None:
    async def fake_ping(_: AsyncEngine) -> bool:
        return False

    monkeypatch.setattr(db_module, "ping", fake_ping)
    app = make_app(engine)
    async with TestClient(TestServer(app)) as client:
        resp = await client.get("/health")
        assert resp.status == 503
        body = await resp.json()
        assert body == {"status": "degraded", "db": "fail"}


@pytest.mark.asyncio
async def test_metrics_exposes_text(engine: AsyncEngine) -> None:
    from warden.infrastructure.telemetry.metrics import set_build_info

    set_build_info(version="0.0.0-test", environment="test")
    app = make_app(engine)
    async with TestClient(TestServer(app)) as client:
        resp = await client.get("/metrics")
        assert resp.status == 200
        text = await resp.text()
        assert "warden_build_info" in text
