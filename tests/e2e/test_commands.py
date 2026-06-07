from __future__ import annotations

from datetime import date, timedelta
from typing import Any

import pytest
from aiogram import Bot, Dispatcher

from tests.conftest import ALLOWED_USER_ID, DENIED_USER_ID
from warden.bot.keyboards import AirportCallback, PriceCallback, SubscriptionCallback
from warden.domain.subscription import NewSubscriptionData
from warden.services.subscription_manager import SubscriptionManager


async def _feed(dispatcher: Dispatcher, bot: Bot, update: Any) -> None:
    await dispatcher.feed_update(bot, update)


def _new_payload() -> NewSubscriptionData:
    return NewSubscriptionData(
        origin="BEG",
        destination="BCN",
        date_from=date(2026, 7, 10),
        date_to=date(2026, 7, 20),
        max_price=120.0,
    )


# --------------------------------------------------------------------------- #
# Unchanged stubs / infra commands.
# --------------------------------------------------------------------------- #


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
async def test_search_and_stats_still_stubs(
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


# --------------------------------------------------------------------------- #
# /list — real data.
# --------------------------------------------------------------------------- #


@pytest.mark.asyncio
async def test_list_empty(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/list"))
    assert len(sent_messages) == 1
    assert "нет подписок" in sent_messages[0]["text"]


@pytest.mark.asyncio
async def test_list_shows_created_subscription(
    dispatcher: Dispatcher,
    bot: Bot,
    sent_messages: list[dict[str, Any]],
    make_update: Any,
    manager: SubscriptionManager,
) -> None:
    await manager.create(ALLOWED_USER_ID, _new_payload())
    await _feed(dispatcher, bot, make_update("/list"))
    assert len(sent_messages) == 1
    assert "BEG → BCN" in sent_messages[0]["text"]


# --------------------------------------------------------------------------- #
# Management — text path with UUID.
# --------------------------------------------------------------------------- #


@pytest.mark.asyncio
async def test_pause_resume_via_text(
    dispatcher: Dispatcher,
    bot: Bot,
    texts: list[str],
    make_update: Any,
    manager: SubscriptionManager,
) -> None:
    view = await manager.create(ALLOWED_USER_ID, _new_payload())

    await _feed(dispatcher, bot, make_update(f"/pause {view.id}"))
    assert "приостановлена" in texts[-1]
    paused = await manager.get(view.id, ALLOWED_USER_ID)
    assert paused is not None and paused.status == "paused"

    await _feed(dispatcher, bot, make_update(f"/resume {view.id}"))
    assert "возобновлена" in texts[-1]
    resumed = await manager.get(view.id, ALLOWED_USER_ID)
    assert resumed is not None and resumed.status == "active"


@pytest.mark.asyncio
async def test_delete_via_text(
    dispatcher: Dispatcher,
    bot: Bot,
    texts: list[str],
    make_update: Any,
    manager: SubscriptionManager,
) -> None:
    view = await manager.create(ALLOWED_USER_ID, _new_payload())
    await _feed(dispatcher, bot, make_update(f"/delete {view.id}"))
    assert "удалена" in texts[-1]
    assert await manager.get(view.id, ALLOWED_USER_ID) is None


@pytest.mark.asyncio
async def test_pause_invalid_uuid(
    dispatcher: Dispatcher, bot: Bot, texts: list[str], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/pause not-a-uuid"))
    assert "корректный id" in texts[-1]


@pytest.mark.asyncio
async def test_delete_unknown_uuid(
    dispatcher: Dispatcher, bot: Bot, texts: list[str], make_update: Any
) -> None:
    from uuid import uuid4

    await _feed(dispatcher, bot, make_update(f"/delete {uuid4()}"))
    assert "не найдена" in texts[-1]


# --------------------------------------------------------------------------- #
# Management — inline-button callbacks.
# --------------------------------------------------------------------------- #


@pytest.mark.asyncio
async def test_pause_via_callback(
    dispatcher: Dispatcher,
    bot: Bot,
    make_callback: Any,
    manager: SubscriptionManager,
) -> None:
    view = await manager.create(ALLOWED_USER_ID, _new_payload())
    data = SubscriptionCallback(action="pause", id=str(view.id)).pack()
    await _feed(dispatcher, bot, make_callback(data))
    paused = await manager.get(view.id, ALLOWED_USER_ID)
    assert paused is not None and paused.status == "paused"


@pytest.mark.asyncio
async def test_delete_via_callback_with_confirm(
    dispatcher: Dispatcher,
    bot: Bot,
    make_callback: Any,
    manager: SubscriptionManager,
) -> None:
    view = await manager.create(ALLOWED_USER_ID, _new_payload())
    sid = str(view.id)
    # First tap shows the confirm keyboard, subscription still present.
    await _feed(
        dispatcher, bot, make_callback(SubscriptionCallback(action="delete", id=sid).pack())
    )
    assert await manager.get(view.id, ALLOWED_USER_ID) is not None
    # Confirm tap deletes it.
    await _feed(
        dispatcher,
        bot,
        make_callback(SubscriptionCallback(action="delete_confirm", id=sid).pack()),
    )
    assert await manager.get(view.id, ALLOWED_USER_ID) is None


# --------------------------------------------------------------------------- #
# /new — full interactive flow (airport buttons + calendar + price).
# --------------------------------------------------------------------------- #


@pytest.mark.asyncio
async def test_new_subscription_full_flow(
    dispatcher: Dispatcher,
    bot: Bot,
    texts: list[str],
    make_update: Any,
    make_callback: Any,
    manager: SubscriptionManager,
) -> None:
    from aiogram_calendar import SimpleCalendarCallback
    from aiogram_calendar.schemas import SimpleCalAct

    today = date.today()
    df = today + timedelta(days=40)
    dt = today + timedelta(days=50)

    # /new → frequent airports, step 1
    await _feed(dispatcher, bot, make_update("/new"))
    assert "Шаг 1/5" in texts[-1]

    # pick origin + destination via airport buttons
    await _feed(
        dispatcher,
        bot,
        make_callback(AirportCallback(step="origin", action="pick", iata="BEG", page=0).pack()),
    )
    assert "Шаг 2/5" in texts[-1]
    await _feed(
        dispatcher,
        bot,
        make_callback(AirportCallback(step="dest", action="pick", iata="BCN", page=0).pack()),
    )
    assert "Шаг 3/5" in texts[-1]

    # pick date_from + date_to via the calendar
    await _feed(
        dispatcher,
        bot,
        make_callback(
            SimpleCalendarCallback(
                act=SimpleCalAct.day, year=df.year, month=df.month, day=df.day
            ).pack()
        ),
    )
    assert "Шаг 4/5" in texts[-1]
    await _feed(
        dispatcher,
        bot,
        make_callback(
            SimpleCalendarCallback(
                act=SimpleCalAct.day, year=dt.year, month=dt.month, day=dt.day
            ).pack()
        ),
    )
    assert "Шаг 5/5" in texts[-1]

    # skip the price → subscription is persisted
    await _feed(dispatcher, bot, make_callback(PriceCallback(action="skip").pack()))
    assert "Подписка создана" in texts[-1]

    listed = await manager.list_for_user(ALLOWED_USER_ID)
    assert len(listed) == 1
    assert listed[0].origin == "BEG"
    assert listed[0].destinations == ["BCN"]
    assert listed[0].date_from is not None and listed[0].date_from.date() == df


@pytest.mark.asyncio
async def test_new_subscription_airport_text_search(
    dispatcher: Dispatcher, bot: Bot, texts: list[str], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/new"))
    await _feed(dispatcher, bot, make_update("Barcelona"))
    assert "Выбери аэропорт" in texts[-1]


@pytest.mark.asyncio
async def test_new_subscription_cancel(
    dispatcher: Dispatcher, bot: Bot, sent_messages: list[dict[str, Any]], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/new"))
    await _feed(dispatcher, bot, make_update("/cancel"))
    assert "Отменено" in sent_messages[-1]["text"]


@pytest.mark.asyncio
async def test_cancel_with_no_active_dialog(
    dispatcher: Dispatcher, bot: Bot, texts: list[str], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/cancel"))
    assert "Нечего отменять" in texts[-1]


@pytest.mark.asyncio
async def test_airport_search_no_results(
    dispatcher: Dispatcher, bot: Bot, texts: list[str], make_update: Any
) -> None:
    await _feed(dispatcher, bot, make_update("/new"))
    await _feed(dispatcher, bot, make_update("zzzzzzzz"))
    assert "Ничего не нашёл" in texts[-1]


@pytest.mark.asyncio
async def test_airport_search_pagination(
    dispatcher: Dispatcher, bot: Bot, make_update: Any, make_callback: Any
) -> None:
    # A broad query yields more than one page; the page callback must not crash
    # and re-runs the stored query to redraw the keyboard.
    await _feed(dispatcher, bot, make_update("/new"))
    await _feed(dispatcher, bot, make_update("international"))
    await _feed(
        dispatcher,
        bot,
        make_callback(AirportCallback(step="origin", action="page", iata="-", page=1).pack()),
    )  # no exception == pass


@pytest.mark.asyncio
async def test_delete_cancel_keeps_subscription(
    dispatcher: Dispatcher,
    bot: Bot,
    make_callback: Any,
    manager: SubscriptionManager,
) -> None:
    view = await manager.create(ALLOWED_USER_ID, _new_payload())
    sid = str(view.id)
    await _feed(
        dispatcher, bot, make_callback(SubscriptionCallback(action="delete", id=sid).pack())
    )
    await _feed(
        dispatcher,
        bot,
        make_callback(SubscriptionCallback(action="delete_cancel", id=sid).pack()),
    )
    assert await manager.get(view.id, ALLOWED_USER_ID) is not None


@pytest.mark.asyncio
async def test_pause_callback_unknown_subscription(
    dispatcher: Dispatcher, bot: Bot, make_callback: Any
) -> None:
    from uuid import uuid4

    # Should answer with an alert, not raise.
    await _feed(
        dispatcher,
        bot,
        make_callback(SubscriptionCallback(action="pause", id=str(uuid4())).pack()),
    )


@pytest.mark.asyncio
async def test_max_price_invalid_then_skip(
    dispatcher: Dispatcher,
    bot: Bot,
    texts: list[str],
    make_update: Any,
    make_callback: Any,
    manager: SubscriptionManager,
) -> None:
    from aiogram_calendar import SimpleCalendarCallback
    from aiogram_calendar.schemas import SimpleCalAct

    today = date.today()
    df = today + timedelta(days=40)
    dt = today + timedelta(days=50)

    await _feed(dispatcher, bot, make_update("/new"))
    await _feed(
        dispatcher,
        bot,
        make_callback(AirportCallback(step="origin", action="pick", iata="BEG", page=0).pack()),
    )
    await _feed(
        dispatcher,
        bot,
        make_callback(AirportCallback(step="dest", action="pick", iata="BCN", page=0).pack()),
    )
    await _feed(
        dispatcher,
        bot,
        make_callback(
            SimpleCalendarCallback(
                act=SimpleCalAct.day, year=df.year, month=df.month, day=df.day
            ).pack()
        ),
    )
    await _feed(
        dispatcher,
        bot,
        make_callback(
            SimpleCalendarCallback(
                act=SimpleCalAct.day, year=dt.year, month=dt.month, day=dt.day
            ).pack()
        ),
    )
    # invalid price → reprompt, no subscription yet
    await _feed(dispatcher, bot, make_update("not-a-number"))
    assert "не похоже на число" in texts[-1].lower()
    assert await manager.list_for_user(ALLOWED_USER_ID) == []

    # valid numeric price → persisted with the limit
    await _feed(dispatcher, bot, make_update("99,50"))
    listed = await manager.list_for_user(ALLOWED_USER_ID)
    assert len(listed) == 1
    assert listed[0].max_price == 99.5
