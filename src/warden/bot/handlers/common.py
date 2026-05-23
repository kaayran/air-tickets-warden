from __future__ import annotations

import structlog
from aiogram import Router
from aiogram.filters import Command, CommandStart
from aiogram.types import Message
from sqlalchemy.ext.asyncio import AsyncEngine

from warden.infrastructure import db as db_module

router = Router(name="common")
logger = structlog.get_logger(__name__)


HELP_TEXT = (
    "✈️ <b>Air Tickets Warden</b>\n\n"
    "Доступные команды:\n"
    "/new — создать подписку на маршрут (FSM-диалог)\n"
    "/list — список активных подписок\n"
    "/pause &lt;id&gt; — приостановить подписку\n"
    "/resume &lt;id&gt; — возобновить подписку\n"
    "/delete &lt;id&gt; — удалить подписку\n"
    "/search &lt;id&gt; — ручной запуск проверки\n"
    "/stats &lt;id&gt; — статистика по маршруту\n"
    "/health — состояние бота\n"
    "/help — эта справка\n\n"
    "Сейчас работает только каркас (phase 0). Реальный поиск и алерты появятся в v1.0."
)


@router.message(CommandStart())
async def cmd_start(message: Message) -> None:
    await message.answer(HELP_TEXT, parse_mode="HTML")


@router.message(Command("help"))
async def cmd_help(message: Message) -> None:
    await message.answer(HELP_TEXT, parse_mode="HTML")


@router.message(Command("health"))
async def cmd_health(message: Message, engine: AsyncEngine) -> None:
    db_ok = await db_module.ping(engine)
    if db_ok:
        text = "✅ Bot alive\n✅ DB: OK"
    else:
        text = "✅ Bot alive\n❌ DB: FAIL"
        logger.warning("health_check_db_failed_via_bot")
    await message.answer(text)
