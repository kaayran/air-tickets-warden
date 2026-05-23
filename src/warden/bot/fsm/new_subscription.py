from __future__ import annotations

from aiogram.fsm.state import State, StatesGroup


class NewSubscription(StatesGroup):
    waiting_origin = State()
    waiting_destination = State()
    waiting_date_from = State()
    waiting_date_to = State()
    waiting_max_price = State()
    waiting_confirm = State()
