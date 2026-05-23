from __future__ import annotations

import structlog
from aiogram import F, Router
from aiogram.filters import Command, CommandObject
from aiogram.fsm.context import FSMContext
from aiogram.types import Message

from warden.bot.fsm.new_subscription import NewSubscription

router = Router(name="subscriptions")
logger = structlog.get_logger(__name__)


@router.message(Command("list"))
async def cmd_list(message: Message) -> None:
    await message.answer(
        "У вас 0 активных подписок.\n<i>Subscription Manager появится в v1.0.</i>",
        parse_mode="HTML",
    )


@router.message(Command("pause"))
async def cmd_pause(message: Message, command: CommandObject) -> None:
    await _stub_with_id(message, command, action="приостановка")


@router.message(Command("resume"))
async def cmd_resume(message: Message, command: CommandObject) -> None:
    await _stub_with_id(message, command, action="возобновление")


@router.message(Command("delete"))
async def cmd_delete(message: Message, command: CommandObject) -> None:
    await _stub_with_id(message, command, action="удаление")


async def _stub_with_id(message: Message, command: CommandObject, *, action: str) -> None:
    arg = (command.args or "").strip()
    if not arg:
        await message.answer(
            f"⚠️ Укажи id подписки: /{command.command} &lt;id&gt;", parse_mode="HTML"
        )
        return
    logger.info("subscription_action_stub", action=action, subscription_id=arg)
    await message.answer(
        f"🛠 Запрошено {action} подписки <code>{arg}</code>.\n"
        "<i>Subscription Manager появится в v1.0.</i>",
        parse_mode="HTML",
    )


@router.message(Command("new"))
async def cmd_new(message: Message, state: FSMContext) -> None:
    await state.clear()
    await state.set_state(NewSubscription.waiting_origin)
    logger.info("new_subscription_started")
    await message.answer(
        "Создаём подписку. Шаг 1/5.\n"
        "Введи IATA-код аэропорта отправления (например, <code>BEG</code>).\n"
        "Для отмены — /cancel",
        parse_mode="HTML",
    )


@router.message(Command("cancel"), F.text)
async def cmd_cancel(message: Message, state: FSMContext) -> None:
    current = await state.get_state()
    if current is None:
        await message.answer("Нечего отменять.")
        return
    await state.clear()
    logger.info("new_subscription_cancelled", from_state=current)
    await message.answer("Отменено.")


@router.message(NewSubscription.waiting_origin, F.text)
async def fsm_origin(message: Message, state: FSMContext) -> None:
    code = _normalize_iata(message.text)
    if code is None:
        await message.answer("⚠️ Это не похоже на IATA-код (3 буквы). Попробуй ещё раз.")
        return
    await state.update_data(origin=code)
    await state.set_state(NewSubscription.waiting_destination)
    logger.info("new_subscription_origin_set", origin=code)
    await message.answer(
        f"Отлично, отправление: <b>{code}</b>.\nШаг 2/5. Введи IATA-код аэропорта назначения.",
        parse_mode="HTML",
    )


@router.message(NewSubscription.waiting_destination, F.text)
async def fsm_destination(message: Message, state: FSMContext) -> None:
    code = _normalize_iata(message.text)
    if code is None:
        await message.answer("⚠️ Это не похоже на IATA-код (3 буквы). Попробуй ещё раз.")
        return
    await state.update_data(destination=code)
    await state.set_state(NewSubscription.waiting_date_from)
    logger.info("new_subscription_destination_set", destination=code)
    await message.answer(
        f"Назначение: <b>{code}</b>.\n"
        "Шаг 3/5. Самая ранняя дата вылета в формате <code>YYYY-MM-DD</code>.",
        parse_mode="HTML",
    )


@router.message(NewSubscription.waiting_date_from, F.text)
async def fsm_date_from(message: Message, state: FSMContext) -> None:
    text = message.text or ""
    if not _looks_like_iso_date(text):
        await message.answer("⚠️ Формат: YYYY-MM-DD. Попробуй ещё раз.")
        return
    await state.update_data(date_from=text.strip())
    await state.set_state(NewSubscription.waiting_date_to)
    await message.answer("Шаг 4/5. Самая поздняя дата вылета (YYYY-MM-DD).")


@router.message(NewSubscription.waiting_date_to, F.text)
async def fsm_date_to(message: Message, state: FSMContext) -> None:
    text = message.text or ""
    if not _looks_like_iso_date(text):
        await message.answer("⚠️ Формат: YYYY-MM-DD. Попробуй ещё раз.")
        return
    await state.update_data(date_to=text.strip())
    await state.set_state(NewSubscription.waiting_max_price)
    await message.answer(
        "Шаг 5/5. Максимальная цена в EUR (число), либо <code>-</code> чтобы пропустить.",
        parse_mode="HTML",
    )


@router.message(NewSubscription.waiting_max_price, F.text)
async def fsm_max_price(message: Message, state: FSMContext) -> None:
    raw = (message.text or "").strip()
    max_price: float | None = None
    if raw not in {"-", ""}:
        try:
            max_price = float(raw.replace(",", "."))
        except ValueError:
            await message.answer(
                "⚠️ Не похоже на число. Попробуй ещё раз или отправь <code>-</code>.",
                parse_mode="HTML",
            )
            return
    await state.update_data(max_price=max_price)
    data = await state.get_data()
    logger.info("new_subscription_completed_stub", **data)
    await state.clear()
    await message.answer(
        "✅ Получены данные подписки:\n"
        f"<b>{data['origin']} → {data['destination']}</b>\n"
        f"Даты: {data['date_from']} … {data['date_to']}\n"
        f"Лимит: {max_price if max_price is not None else '—'} EUR\n\n"
        "<i>Сохранение в БД и реальный мониторинг появятся в v1.0.</i>",
        parse_mode="HTML",
    )


def _normalize_iata(text: str | None) -> str | None:
    if not text:
        return None
    candidate = text.strip().upper()
    if len(candidate) == 3 and candidate.isalpha():
        return candidate
    return None


def _looks_like_iso_date(text: str | None) -> bool:
    if not text:
        return False
    value = text.strip()
    if len(value) != 10 or value[4] != "-" or value[7] != "-":
        return False
    try:
        from datetime import date

        date.fromisoformat(value)
    except ValueError:
        return False
    return True
