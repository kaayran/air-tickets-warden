from __future__ import annotations

import asyncio
import contextlib
import signal
import sys

import structlog

from warden import __version__
from warden.bot.dispatcher import make_bot, make_dispatcher
from warden.config import Settings, get_settings
from warden.infrastructure import db as db_module
from warden.infrastructure.telemetry.logging import setup_logging
from warden.infrastructure.telemetry.metrics import set_build_info
from warden.infrastructure.telemetry.sentry import init_sentry
from warden.infrastructure.web import start_web


async def run(settings: Settings) -> None:
    logger = structlog.get_logger("warden.main")

    engine = db_module.make_engine(settings.database_url)
    bot = make_bot(settings)
    dp = make_dispatcher(settings, engine)

    set_build_info(version=__version__, environment=settings.environment)

    web_runner, _ = await start_web(engine, settings.web_host, settings.web_port)

    stop_event = asyncio.Event()

    def _request_stop() -> None:
        if not stop_event.is_set():
            logger.info("shutdown_requested")
            stop_event.set()

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        with contextlib.suppress(NotImplementedError):
            loop.add_signal_handler(sig, _request_stop)

    logger.info("bot_started", version=__version__, environment=settings.environment)

    polling_task = asyncio.create_task(
        dp.start_polling(bot, allowed_updates=dp.resolve_used_update_types()),
        name="aiogram-polling",
    )
    stop_task = asyncio.create_task(stop_event.wait(), name="stop-signal")

    try:
        done, _pending = await asyncio.wait(
            {polling_task, stop_task}, return_when=asyncio.FIRST_COMPLETED
        )
        for task in done:
            exc = task.exception()
            if exc is not None and task is polling_task:
                logger.error("polling_crashed", exc_info=exc)
    finally:
        if not polling_task.done():
            with contextlib.suppress(RuntimeError):
                await dp.stop_polling()
        for task in (polling_task, stop_task):
            if not task.done():
                task.cancel()
        for task in (polling_task, stop_task):
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await task
        await bot.session.close()
        await web_runner.cleanup()
        await engine.dispose()
        logger.info("bot_stopped")


def main() -> int:
    try:
        settings = get_settings()
    except Exception as exc:
        print(f"Configuration error: {exc}", file=sys.stderr)
        return 2

    setup_logging(log_level=settings.log_level, json=settings.log_json)
    init_sentry(settings)

    with contextlib.suppress(KeyboardInterrupt):
        asyncio.run(run(settings))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
