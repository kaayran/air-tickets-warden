from __future__ import annotations

import structlog
from aiogram import Router
from aiogram.filters import Command, CommandObject
from aiogram.types import Message

router = Router(name="search")
logger = structlog.get_logger(__name__)


@router.message(Command("search"))
async def cmd_search(message: Message, command: CommandObject) -> None:
    arg = (command.args or "").strip()
    if not arg:
        await message.answer("⚠️ Укажи id подписки: /search &lt;id&gt;", parse_mode="HTML")
        return
    logger.info("manual_search_stub", subscription_id=arg)
    await message.answer(
        f"🔎 Ручной поиск для подписки <code>{arg}</code> запрошен.\n"
        "<i>Адаптеры и Aggregator появятся в v1.0.</i>",
        parse_mode="HTML",
    )


@router.message(Command("stats"))
async def cmd_stats(message: Message, command: CommandObject) -> None:
    arg = (command.args or "").strip()
    if not arg:
        await message.answer("⚠️ Укажи id подписки: /stats &lt;id&gt;", parse_mode="HTML")
        return
    logger.info("stats_stub", subscription_id=arg)
    await message.answer(
        f"📊 Статистика по подписке <code>{arg}</code>.\n"
        "<i>Появится в v1.0, когда заработает Price History Store.</i>",
        parse_mode="HTML",
    )
