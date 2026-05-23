from __future__ import annotations

from typing import Any

import structlog
from aiohttp import web
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from sqlalchemy.ext.asyncio import AsyncEngine

from warden.infrastructure import db as db_module
from warden.infrastructure.telemetry.metrics import REGISTRY, db_ping_failures_total

logger = structlog.get_logger(__name__)

ENGINE_KEY: web.AppKey[AsyncEngine] = web.AppKey("engine", AsyncEngine)


async def health(request: web.Request) -> web.Response:
    engine: AsyncEngine = request.app[ENGINE_KEY]
    db_ok = await db_module.ping(engine)
    if not db_ok:
        db_ping_failures_total.inc()
        logger.warning("health_check_db_failed")
        return web.json_response({"status": "degraded", "db": "fail"}, status=503)
    return web.json_response({"status": "ok", "db": "ok"})


async def metrics(_: web.Request) -> web.Response:
    payload = generate_latest(REGISTRY)
    return web.Response(body=payload, content_type=CONTENT_TYPE_LATEST.split(";")[0])


def make_app(engine: AsyncEngine) -> web.Application:
    app = web.Application()
    app[ENGINE_KEY] = engine
    app.router.add_get("/health", health)
    app.router.add_get("/metrics", metrics)
    return app


async def start_web(
    engine: AsyncEngine, host: str, port: int
) -> tuple[web.AppRunner, dict[str, Any]]:
    app = make_app(engine)
    runner = web.AppRunner(app, access_log=None)
    await runner.setup()
    site = web.TCPSite(runner, host=host, port=port)
    await site.start()
    logger.info("web_started", host=host, port=port)
    return runner, {"host": host, "port": port}
