from __future__ import annotations

from collections.abc import Awaitable, Callable
from typing import Any

import structlog
from aiogram import BaseMiddleware
from aiogram.types import TelegramObject, User

from warden.infrastructure.telemetry.metrics import bot_updates_dropped_total

logger = structlog.get_logger(__name__)


class WhitelistMiddleware(BaseMiddleware):
    """Drop updates from users outside ALLOWED_USER_IDS.

    A WARN log is emitted on every drop so abuse attempts are visible.
    """

    def __init__(self, allowed_user_ids: list[int]) -> None:
        self._allowed = frozenset(allowed_user_ids)

    async def __call__(
        self,
        handler: Callable[[TelegramObject, dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: dict[str, Any],
    ) -> Any:
        user: User | None = data.get("event_from_user")
        if user is None or user.id not in self._allowed:
            bot_updates_dropped_total.labels(reason="whitelist").inc()
            logger.warning(
                "dropped_update",
                reason="whitelist",
                user_id=user.id if user else None,
                username=user.username if user else None,
            )
            return None
        return await handler(event, data)
