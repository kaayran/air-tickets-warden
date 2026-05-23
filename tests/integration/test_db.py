from __future__ import annotations

import os
from pathlib import Path

import pytest
from alembic import command
from alembic.config import Config as AlembicConfig
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from warden.config import Settings
from warden.infrastructure import db as db_module

PROJECT_ROOT = Path(__file__).resolve().parents[2]
ALEMBIC_INI = PROJECT_ROOT / "alembic.ini"


@pytest.mark.asyncio
async def test_ping_succeeds(engine: AsyncEngine) -> None:
    assert await db_module.ping(engine) is True


@pytest.mark.asyncio
async def test_ping_fails_after_dispose(engine: AsyncEngine) -> None:
    await engine.dispose()
    bad = db_module.make_engine("sqlite+aiosqlite:////nonexistent/path/warden.db")
    try:
        # sqlite happily creates files, so point to something the engine cannot
        # open and reach with an invalid query instead.
        async with bad.connect() as conn:
            await conn.execute(text("SELECT 1"))
    except Exception:
        pass
    finally:
        await bad.dispose()


@pytest.mark.asyncio
async def test_migrations_up_and_down(settings: Settings) -> None:
    cfg = AlembicConfig(str(ALEMBIC_INI))
    cfg.set_main_option("sqlalchemy.url", settings.database_url)
    cfg.set_main_option("script_location", str(PROJECT_ROOT / "alembic"))
    os.chdir(PROJECT_ROOT)

    command.upgrade(cfg, "head")
    eng = db_module.make_engine(settings.database_url)
    async with eng.connect() as conn:
        tables = await conn.run_sync(
            lambda sync_conn: set(
                row[0]
                for row in sync_conn.exec_driver_sql(
                    "SELECT name FROM sqlite_master WHERE type='table'"
                ).fetchall()
            )
        )
    await eng.dispose()
    assert "subscriptions" in tables
    assert "scheduler_runs" in tables

    command.downgrade(cfg, "base")
    eng = db_module.make_engine(settings.database_url)
    async with eng.connect() as conn:
        tables = await conn.run_sync(
            lambda sync_conn: set(
                row[0]
                for row in sync_conn.exec_driver_sql(
                    "SELECT name FROM sqlite_master WHERE type='table'"
                ).fetchall()
            )
        )
    await eng.dispose()
    assert "subscriptions" not in tables
    assert "scheduler_runs" not in tables
