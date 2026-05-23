from __future__ import annotations

from typing import Any

import pytest
from aiogram import Bot, Dispatcher

from tests.conftest import ALLOWED_USER_ID, DENIED_USER_ID


async def _feed(dispatcher: Dispatcher, bot: Bot, update: Any) -> None:
    await dispatcher.feed_update(bot, update)


@pytest.mark.asyncio
async def test_help_command_responds(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/help"))
    assert len(sent_messages) == 1
    assert "Air Tickets Warden" in sent_messages[0]["text"]


@pytest.mark.asyncio
async def test_health_command_db_ok(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/health"))
    assert len(sent_messages) == 1
    assert "DB: OK" in sent_messages[0]["text"]


@pytest.mark.asyncio
async def test_list_command_stub(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/list"))
    assert len(sent_messages) == 1
    assert "0 активных" in sent_messages[0]["text"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "command,action_word",
    [
        ("pause", "приостановка"),
        ("resume", "возобновление"),
        ("delete", "удаление"),
    ],
)
async def test_subscription_action_with_id(
    dispatcher: Dispatcher,
    bot: Bot,
    sent_messages: list[dict[str, Any]],
    make_update: Any,
    command: str,
    action_word: str,
) -> None:
    await _feed(dispatcher, bot, make_update(f"/{command} abc-123"))
    assert len(sent_messages) == 1
    assert "abc-123" in sent_messages[0]["text"]
    assert action_word in sent_messages[0]["text"]


@pytest.mark.asyncio
async def test_subscription_action_without_id(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/pause"))
    assert len(sent_messages) == 1
    assert "Укажи id" in sent_messages[0]["text"]


@pytest.mark.asyncio
async def test_search_and_stats(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/search xyz"))
    await _feed(dispatcher, bot, make_update("/stats xyz"))
    assert len(sent_messages) == 2
    assert "Ручной поиск" in sent_messages[0]["text"]
    assert "Статистика" in sent_messages[1]["text"]


@pytest.mark.asyncio
async def test_whitelist_blocks_unknown_user(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/help", user_id=DENIED_USER_ID))
    assert sent_messages == []


@pytest.mark.asyncio
async def test_new_subscription_fsm_full_flow(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    # Step 0: /new
    await _feed(dispatcher, bot, make_update("/new"))
    assert "Шаг 1/5" in sent_messages[-1]["text"]

    # Step 1: origin
    await _feed(dispatcher, bot, make_update("BEG"))
    assert "Шаг 2/5" in sent_messages[-1]["text"]

    # Step 2: destination
    await _feed(dispatcher, bot, make_update("BCN"))
    assert "Шаг 3/5" in sent_messages[-1]["text"]

    # Step 3: date_from
    await _feed(dispatcher, bot, make_update("2026-07-10"))
    assert "Шаг 4/5" in sent_messages[-1]["text"]

    # Step 4: date_to
    await _feed(dispatcher, bot, make_update("2026-07-20"))
    assert "Шаг 5/5" in sent_messages[-1]["text"]

    # Step 5: max_price
    await _feed(dispatcher, bot, make_update("120"))
    final = sent_messages[-1]["text"]
    assert "BEG → BCN" in final
    assert "v1.0" in final

    storage_state = await dispatcher.fsm.resolve_context(
        bot=bot, chat_id=ALLOWED_USER_ID, user_id=ALLOWED_USER_ID
    ).get_state()
    assert storage_state is None


@pytest.mark.asyncio
async def test_new_subscription_validates_iata(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/new"))
    await _feed(dispatcher, bot, make_update("not-iata"))
    assert "IATA" in sent_messages[-1]["text"]


@pytest.mark.asyncio
async def test_new_subscription_cancel(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/new"))
    await _feed(dispatcher, bot, make_update("/cancel"))
    assert "Отменено" in sent_messages[-1]["text"]
