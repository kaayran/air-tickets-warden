from __future__ import annotations

from typing import Any
from unittest.mock import AsyncMock

import pytest
from aiogram.types import User

from warden.bot.middlewares.whitelist import WhitelistMiddleware


@pytest.mark.asyncio
async def test_whitelist_passes_allowed_user() -> None:
    mw = WhitelistMiddleware([100])
    handler = AsyncMock(return_value="ok")
    data: dict[str, Any] = {
        "event_from_user": User(id=100, is_bot=False, first_name="A"),
    }
    result = await mw(handler, object(), data)
    assert result == "ok"
    handler.assert_awaited_once()


@pytest.mark.asyncio
async def test_whitelist_drops_unknown_user() -> None:
    mw = WhitelistMiddleware([100])
    handler = AsyncMock(return_value="ok")
    data: dict[str, Any] = {
        "event_from_user": User(id=999, is_bot=False, first_name="X"),
    }
    result = await mw(handler, object(), data)
    assert result is None
    handler.assert_not_awaited()


@pytest.mark.asyncio
async def test_whitelist_drops_when_no_user() -> None:
    mw = WhitelistMiddleware([100])
    handler = AsyncMock(return_value="ok")
    result = await mw(handler, object(), {})
    assert result is None
    handler.assert_not_awaited()
