from __future__ import annotations

from datetime import UTC, date, datetime
from uuid import UUID

import structlog
from aiogram import F, Router
from aiogram.filters import Command, CommandObject, StateFilter
from aiogram.fsm.context import FSMContext
from aiogram.types import CallbackQuery, Message
from aiogram_calendar import SimpleCalendar, SimpleCalendarCallback

from warden.bot.fsm.new_subscription import NewSubscription
from warden.bot.keyboards import (
    PAGE_SIZE,
    AirportCallback,
    AirportStep,
    PriceCallback,
    SubscriptionCallback,
    airports_kb,
    delete_confirm_kb,
    frequent_airports_kb,
    price_skip_kb,
    subscription_row_kb,
)
from warden.domain.subscription import NewSubscriptionData, SubscriptionView
from warden.services import airports
from warden.services.subscription_manager import SubscriptionManager

router = Router(name="subscriptions")
logger = structlog.get_logger(__name__)


# --------------------------------------------------------------------------- #
# Commands (registered first so /cancel etc. win over FSM text handlers).
# --------------------------------------------------------------------------- #


@router.message(Command("new"))
async def cmd_new(message: Message, state: FSMContext) -> None:
    await state.clear()
    await state.set_state(NewSubscription.choosing_origin)
    logger.info("new_subscription_started")
    await message.answer(
        "Создаём подписку. Шаг 1/5.\n"
        "Выбери аэропорт отправления из частых или начни вводить город / 3-буквенный код.\n"
        "Для отмены — /cancel",
        reply_markup=frequent_airports_kb("origin", airports.frequent()),
    )


@router.message(Command("cancel"))
async def cmd_cancel(message: Message, state: FSMContext) -> None:
    current = await state.get_state()
    if current is None:
        await message.answer("Нечего отменять.")
        return
    await state.clear()
    logger.info("new_subscription_cancelled", from_state=current)
    await message.answer("Отменено.")


@router.message(Command("list"))
async def cmd_list(message: Message, subscriptions: SubscriptionManager) -> None:
    user = message.from_user
    if user is None:
        return
    views = await subscriptions.list_for_user(user.id)
    if not views:
        await message.answer("У тебя пока нет подписок. Создай через /new.")
        return
    for view in views:
        await message.answer(
            _format_subscription(view),
            parse_mode="HTML",
            reply_markup=subscription_row_kb(view),
        )


@router.message(Command("pause"))
async def cmd_pause(
    message: Message, command: CommandObject, subscriptions: SubscriptionManager
) -> None:
    await _text_set_status(message, command, subscriptions, status="paused", word="приостановлена")


@router.message(Command("resume"))
async def cmd_resume(
    message: Message, command: CommandObject, subscriptions: SubscriptionManager
) -> None:
    await _text_set_status(message, command, subscriptions, status="active", word="возобновлена")


@router.message(Command("delete"))
async def cmd_delete(
    message: Message, command: CommandObject, subscriptions: SubscriptionManager
) -> None:
    user = message.from_user
    if user is None:
        return
    sid = _parse_uuid(command.args)
    if sid is None:
        await message.answer(
            "⚠️ Укажи корректный id подписки (UUID): /delete &lt;id&gt;", parse_mode="HTML"
        )
        return
    ok = await subscriptions.delete(sid, user.id)
    logger.info("subscription_deleted_via_command", subscription_id=str(sid), ok=ok)
    await message.answer("🗑 Подписка удалена." if ok else "Подписка не найдена.")


# --------------------------------------------------------------------------- #
# /new FSM — airport pickers.
# --------------------------------------------------------------------------- #


@router.message(StateFilter(NewSubscription.choosing_origin), F.text)
async def fsm_origin_query(message: Message, state: FSMContext) -> None:
    await _answer_airport_search(message, state, "origin", message.text or "")


@router.message(StateFilter(NewSubscription.choosing_destination), F.text)
async def fsm_destination_query(message: Message, state: FSMContext) -> None:
    await _answer_airport_search(message, state, "dest", message.text or "")


@router.callback_query(AirportCallback.filter(F.action == "page"))
async def cb_airport_page(
    callback: CallbackQuery, callback_data: AirportCallback, state: FSMContext
) -> None:
    data = await state.get_data()
    query: str = data.get("apt_query", "")
    hits = airports.search(query)
    page = callback_data.page
    start = page * PAGE_SIZE
    page_hits = hits[start : start + PAGE_SIZE]
    msg = _as_message(callback)
    if msg is not None and page_hits:
        await msg.edit_reply_markup(
            reply_markup=airports_kb(
                page_hits,
                callback_data.step,
                page=page,
                has_prev=page > 0,
                has_next=len(hits) > start + PAGE_SIZE,
            )
        )
    await callback.answer()


@router.callback_query(AirportCallback.filter(F.action == "pick"))
async def cb_airport_pick(
    callback: CallbackQuery, callback_data: AirportCallback, state: FSMContext
) -> None:
    msg = _as_message(callback)
    hit = airports.resolve(callback_data.iata)
    label = hit.label if hit is not None else callback_data.iata

    if callback_data.step == "origin":
        await state.update_data(origin=callback_data.iata)
        await state.set_state(NewSubscription.choosing_destination)
        logger.info("new_subscription_origin_set", origin=callback_data.iata)
        if msg is not None:
            await msg.edit_text(
                f"Отправление: <b>{label}</b>.\n"
                "Шаг 2/5. Выбери аэропорт назначения или введи город / код.",
                parse_mode="HTML",
                reply_markup=frequent_airports_kb("dest", airports.frequent()),
            )
    else:
        await state.update_data(destination=callback_data.iata)
        await state.set_state(NewSubscription.choosing_date_from)
        logger.info("new_subscription_destination_set", destination=callback_data.iata)
        if msg is not None:
            await msg.edit_text(f"Назначение: <b>{label}</b>.", parse_mode="HTML")
            await msg.answer(
                "Шаг 3/5. Выбери самую раннюю дату вылета:",
                reply_markup=await _calendar().start_calendar(),
            )
    await callback.answer()


# --------------------------------------------------------------------------- #
# /new FSM — calendar date pickers.
# --------------------------------------------------------------------------- #


@router.callback_query(
    SimpleCalendarCallback.filter(), StateFilter(NewSubscription.choosing_date_from)
)
async def fsm_date_from(
    callback: CallbackQuery, callback_data: SimpleCalendarCallback, state: FSMContext
) -> None:
    selected, picked = await _calendar().process_selection(callback, callback_data)
    if not selected or picked is None:
        return
    await state.update_data(date_from=picked.date().isoformat())
    await state.set_state(NewSubscription.choosing_date_to)
    logger.info("new_subscription_date_from_set", date_from=picked.date().isoformat())
    msg = _as_message(callback)
    if msg is not None:
        await msg.answer(
            "Шаг 4/5. Выбери самую позднюю дату вылета:",
            reply_markup=await _calendar(picked.date()).start_calendar(),
        )
    await callback.answer()


@router.callback_query(
    SimpleCalendarCallback.filter(), StateFilter(NewSubscription.choosing_date_to)
)
async def fsm_date_to(
    callback: CallbackQuery, callback_data: SimpleCalendarCallback, state: FSMContext
) -> None:
    data = await state.get_data()
    date_from = date.fromisoformat(data["date_from"])
    selected, picked = await _calendar(date_from).process_selection(callback, callback_data)
    if not selected or picked is None:
        return
    await state.update_data(date_to=picked.date().isoformat())
    await state.set_state(NewSubscription.choosing_max_price)
    logger.info("new_subscription_date_to_set", date_to=picked.date().isoformat())
    msg = _as_message(callback)
    if msg is not None:
        await msg.answer(
            "Шаг 5/5. Максимальная цена в EUR (число) или нажми «Пропустить».",
            reply_markup=price_skip_kb(),
        )
    await callback.answer()


# --------------------------------------------------------------------------- #
# /new FSM — price + finalize.
# --------------------------------------------------------------------------- #


@router.message(StateFilter(NewSubscription.choosing_max_price), F.text)
async def fsm_max_price(
    message: Message, state: FSMContext, subscriptions: SubscriptionManager
) -> None:
    user = message.from_user
    if user is None:
        return
    raw = (message.text or "").strip()
    max_price: float | None = None
    if raw not in {"-", ""}:
        try:
            max_price = float(raw.replace(",", "."))
        except ValueError:
            await message.answer("⚠️ Не похоже на число. Введи число или нажми «Пропустить».")
            return
    await _finalize(message, state, subscriptions, user.id, max_price)


@router.callback_query(
    PriceCallback.filter(F.action == "skip"), StateFilter(NewSubscription.choosing_max_price)
)
async def cb_price_skip(
    callback: CallbackQuery, state: FSMContext, subscriptions: SubscriptionManager
) -> None:
    await callback.answer()
    msg = _as_message(callback)
    if msg is not None:
        await _finalize(msg, state, subscriptions, callback.from_user.id, None)


# --------------------------------------------------------------------------- #
# /list management callbacks.
# --------------------------------------------------------------------------- #


@router.callback_query(SubscriptionCallback.filter(F.action == "pause"))
async def cb_pause(
    callback: CallbackQuery, callback_data: SubscriptionCallback, subscriptions: SubscriptionManager
) -> None:
    await _callback_set_status(callback, callback_data, subscriptions, "paused")


@router.callback_query(SubscriptionCallback.filter(F.action == "resume"))
async def cb_resume(
    callback: CallbackQuery, callback_data: SubscriptionCallback, subscriptions: SubscriptionManager
) -> None:
    await _callback_set_status(callback, callback_data, subscriptions, "active")


@router.callback_query(SubscriptionCallback.filter(F.action == "delete"))
async def cb_delete(callback: CallbackQuery, callback_data: SubscriptionCallback) -> None:
    msg = _as_message(callback)
    if msg is not None:
        await msg.edit_reply_markup(reply_markup=delete_confirm_kb(callback_data.id))
    await callback.answer()


@router.callback_query(SubscriptionCallback.filter(F.action == "delete_cancel"))
async def cb_delete_cancel(
    callback: CallbackQuery, callback_data: SubscriptionCallback, subscriptions: SubscriptionManager
) -> None:
    sid = _parse_uuid(callback_data.id)
    view = await subscriptions.get(sid, callback.from_user.id) if sid is not None else None
    msg = _as_message(callback)
    if msg is not None and view is not None:
        await msg.edit_reply_markup(reply_markup=subscription_row_kb(view))
    await callback.answer("Отменено")


@router.callback_query(SubscriptionCallback.filter(F.action == "delete_confirm"))
async def cb_delete_confirm(
    callback: CallbackQuery, callback_data: SubscriptionCallback, subscriptions: SubscriptionManager
) -> None:
    sid = _parse_uuid(callback_data.id)
    ok = await subscriptions.delete(sid, callback.from_user.id) if sid is not None else False
    if not ok:
        await callback.answer("Подписка не найдена", show_alert=True)
        return
    logger.info("subscription_deleted_via_button", subscription_id=callback_data.id)
    msg = _as_message(callback)
    if msg is not None:
        await msg.edit_text("🗑 Подписка удалена.")
    await callback.answer("Удалено")


# --------------------------------------------------------------------------- #
# Helpers.
# --------------------------------------------------------------------------- #


def _as_message(callback: CallbackQuery) -> Message | None:
    """Return the callback's originating message if it is a regular (accessible) Message."""
    return callback.message if isinstance(callback.message, Message) else None


def _calendar(min_date: date | None = None) -> SimpleCalendar:
    cal = SimpleCalendar()
    today = datetime.now(tz=UTC).date()
    low = max(min_date, today) if min_date is not None else today
    cal.set_dates_range(
        datetime(low.year, low.month, low.day),
        datetime(today.year + 2, 12, 31),
    )
    return cal


def _parse_uuid(value: str | None) -> UUID | None:
    if not value:
        return None
    try:
        return UUID(value.strip())
    except ValueError:
        return None


def _format_subscription(view: SubscriptionView) -> str:
    if view.date_from is not None and view.date_to is not None:
        dates = f"{view.date_from.date()} … {view.date_to.date()}"
    else:
        dates = "—"
    price = f"≤ {view.max_price:g} EUR" if view.max_price is not None else "без лимита цены"
    status = {"active": "🟢 active", "paused": "⏸ paused"}.get(view.status, view.status)
    return (
        f"<b>{view.origin} → {', '.join(view.destinations)}</b>\n"
        f"🗓 {dates}\n"
        f"💰 {price}\n"
        f"{status} · id <code>{view.short_id}</code>"
    )


async def _answer_airport_search(
    message: Message, state: FSMContext, step: AirportStep, query: str
) -> None:
    hits = airports.search(query)
    if not hits:
        await message.answer("Ничего не нашёл. Попробуй другой город или 3-буквенный код.")
        return
    await state.update_data(apt_query=query)
    page_hits = hits[:PAGE_SIZE]
    await message.answer(
        "Выбери аэропорт:",
        reply_markup=airports_kb(
            page_hits, step, page=0, has_prev=False, has_next=len(hits) > PAGE_SIZE
        ),
    )


async def _callback_set_status(
    callback: CallbackQuery,
    callback_data: SubscriptionCallback,
    manager: SubscriptionManager,
    status: str,
) -> None:
    sid = _parse_uuid(callback_data.id)
    view = await manager.set_status(sid, callback.from_user.id, status) if sid is not None else None
    if view is None:
        await callback.answer("Подписка не найдена", show_alert=True)
        return
    logger.info("subscription_status_changed", subscription_id=callback_data.id, status=status)
    msg = _as_message(callback)
    if msg is not None:
        await msg.edit_text(
            _format_subscription(view), parse_mode="HTML", reply_markup=subscription_row_kb(view)
        )
    await callback.answer("Готово")


async def _text_set_status(
    message: Message,
    command: CommandObject,
    manager: SubscriptionManager,
    *,
    status: str,
    word: str,
) -> None:
    user = message.from_user
    if user is None:
        return
    sid = _parse_uuid(command.args)
    if sid is None:
        await message.answer(
            f"⚠️ Укажи корректный id подписки (UUID): /{command.command} &lt;id&gt;",
            parse_mode="HTML",
        )
        return
    view = await manager.set_status(sid, user.id, status)
    await message.answer(f"Подписка {word}." if view is not None else "Подписка не найдена.")


async def _finalize(
    message: Message,
    state: FSMContext,
    manager: SubscriptionManager,
    user_chat_id: int,
    max_price: float | None,
) -> None:
    data = await state.get_data()
    payload = NewSubscriptionData(
        origin=data["origin"],
        destination=data["destination"],
        date_from=date.fromisoformat(data["date_from"]),
        date_to=date.fromisoformat(data["date_to"]),
        max_price=max_price,
    )
    view = await manager.create(user_chat_id, payload)
    await state.clear()
    logger.info(
        "subscription_created",
        subscription_id=str(view.id),
        origin=payload.origin,
        destination=payload.destination,
    )
    limit = f"{max_price:g} EUR" if max_price is not None else "—"
    await message.answer(
        f"✅ Подписка создана: <b>{view.origin} → {', '.join(view.destinations)}</b>\n"
        f"🗓 {payload.date_from} … {payload.date_to}\n"
        f"💰 Лимит: {limit}\n"
        f"id: <code>{view.short_id}</code>\n\n"
        "Управлять — через /list.",
        parse_mode="HTML",
    )
