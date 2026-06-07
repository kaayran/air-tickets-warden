from __future__ import annotations

from aiogram.fsm.state import State, StatesGroup


class NewSubscription(StatesGroup):
    choosing_origin = State()
    choosing_destination = State()
    choosing_date_from = State()
    choosing_date_to = State()
    choosing_max_price = State()
