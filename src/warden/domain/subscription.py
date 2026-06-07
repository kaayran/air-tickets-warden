from __future__ import annotations

from datetime import date, datetime
from typing import Self
from uuid import UUID

from pydantic import BaseModel, ConfigDict, model_validator


class SubscriptionView(BaseModel):
    """Read model for displaying a subscription to the user.

    Decoupled from the SQLAlchemy ORM so bot handlers never touch the DB layer.
    """

    model_config = ConfigDict(frozen=True)

    id: UUID
    origin: str
    destinations: list[str]
    date_from: datetime | None
    date_to: datetime | None
    max_price: float | None
    status: str
    created_at: datetime

    @property
    def short_id(self) -> str:
        """First 8 hex chars of the UUID — compact handle shown in /list."""
        return self.id.hex[:8]


class NewSubscriptionData(BaseModel):
    """Validated payload collected by the /new FSM dialog before persistence."""

    origin: str
    destination: str
    date_from: date
    date_to: date
    max_price: float | None = None

    @model_validator(mode="after")
    def _check_date_order(self) -> Self:
        if self.date_to < self.date_from:
            raise ValueError("date_to must be on or after date_from")
        return self
