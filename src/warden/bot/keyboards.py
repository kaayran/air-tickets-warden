from __future__ import annotations

from typing import Literal

from aiogram.filters.callback_data import CallbackData
from aiogram.types import InlineKeyboardMarkup
from aiogram.utils.keyboard import InlineKeyboardBuilder

from warden.domain.subscription import SubscriptionView
from warden.services.airports import AirportHit

AirportStep = Literal["origin", "dest"]

PAGE_SIZE = 8


class AirportCallback(CallbackData, prefix="apt"):
    step: AirportStep
    action: Literal["pick", "page"]
    iata: str
    page: int


class SubscriptionCallback(CallbackData, prefix="sub"):
    action: Literal["pause", "resume", "delete", "delete_confirm", "delete_cancel"]
    id: str


class PriceCallback(CallbackData, prefix="price"):
    action: Literal["skip"]


def airports_kb(
    page_hits: list[AirportHit],
    step: AirportStep,
    page: int,
    *,
    has_prev: bool,
    has_next: bool,
) -> InlineKeyboardMarkup:
    """Inline "dropdown" of airport candidates plus optional pagination row."""
    builder = InlineKeyboardBuilder()
    for hit in page_hits:
        builder.button(
            text=hit.label,
            callback_data=AirportCallback(step=step, action="pick", iata=hit.iata, page=page),
        )
    builder.adjust(1)

    nav: list[tuple[str, int]] = []
    if has_prev:
        nav.append(("◀", page - 1))
    if has_next:
        nav.append(("▶", page + 1))
    if nav:
        nav_builder = InlineKeyboardBuilder()
        for text, target in nav:
            nav_builder.button(
                text=text,
                callback_data=AirportCallback(step=step, action="page", iata="-", page=target),
            )
        builder.attach(nav_builder)
    return builder.as_markup()


def subscription_row_kb(view: SubscriptionView) -> InlineKeyboardMarkup:
    """Pause/Resume + Delete buttons attached to a /list row."""
    builder = InlineKeyboardBuilder()
    sid = str(view.id)
    if view.status == "active":
        builder.button(text="⏸ Пауза", callback_data=SubscriptionCallback(action="pause", id=sid))
    else:
        builder.button(
            text="▶️ Возобновить", callback_data=SubscriptionCallback(action="resume", id=sid)
        )
    builder.button(text="🗑 Удалить", callback_data=SubscriptionCallback(action="delete", id=sid))
    builder.adjust(2)
    return builder.as_markup()


def delete_confirm_kb(sid: str) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.button(
        text="❗ Точно удалить", callback_data=SubscriptionCallback(action="delete_confirm", id=sid)
    )
    builder.button(
        text="Отмена", callback_data=SubscriptionCallback(action="delete_cancel", id=sid)
    )
    builder.adjust(2)
    return builder.as_markup()


def price_skip_kb() -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    builder.button(text="Пропустить", callback_data=PriceCallback(action="skip"))
    return builder.as_markup()


def frequent_airports_kb(step: AirportStep, hits: list[AirportHit]) -> InlineKeyboardMarkup:
    builder = InlineKeyboardBuilder()
    for hit in hits:
        builder.button(
            text=hit.label,
            callback_data=AirportCallback(step=step, action="pick", iata=hit.iata, page=0),
        )
    builder.adjust(1)
    return builder.as_markup()
