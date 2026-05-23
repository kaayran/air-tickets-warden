from __future__ import annotations

import time
import uuid
from collections.abc import Awaitable, Callable
from typing import Any

import structlog
from aiogram import BaseMiddleware
from aiogram.types import Message, TelegramObject, Update

from warden.infrastructure.telemetry.metrics import (
    bot_updates_total,
    command_duration_seconds,
)

logger = structlog.get_logger(__name__)


class LoggingMiddleware(BaseMiddleware):
    """Bind per-update context to structlog and time handler execution."""

    async def __call__(
        self,
        handler: Callable[[TelegramObject, dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: dict[str, Any],
    ) -> Any:
        update: Update | None = event if isinstance(event, Update) else None
        message = update.message if update else None
        command = _extract_command(message)
        update_type = _extract_update_type(update)

        user = data.get("event_from_user")
        chat = data.get("event_chat")
        trace_id = uuid.uuid4().hex

        bindings: dict[str, Any] = {
            "trace_id": trace_id,
            "update_id": update.update_id if update else None,
            "user_id": getattr(user, "id", None),
            "chat_id": getattr(chat, "id", None),
            "update_type": update_type,
        }
        if command:
            bindings["command"] = command

        structlog.contextvars.bind_contextvars(**bindings)
        bot_updates_total.labels(type=update_type, command=command or "").inc()

        start = time.perf_counter()
        try:
            return await handler(event, data)
        finally:
            duration = time.perf_counter() - start
            if command:
                command_duration_seconds.labels(command=command).observe(duration)
            logger.info(
                "update_handled",
                duration_ms=round(duration * 1000, 2),
            )
            structlog.contextvars.clear_contextvars()


def _extract_command(message: Message | None) -> str | None:
    if message is None or not message.text:
        return None
    text = message.text.strip()
    if not text.startswith("/"):
        return None
    head = text.split(maxsplit=1)[0]
    return head.lstrip("/").split("@", maxsplit=1)[0].lower() or None


def _extract_update_type(update: Update | None) -> str:
    if update is None:
        return "unknown"
    for field in (
        "message",
        "edited_message",
        "callback_query",
        "inline_query",
        "my_chat_member",
        "chat_member",
    ):
        if getattr(update, field, None) is not None:
            return field
    return "other"
