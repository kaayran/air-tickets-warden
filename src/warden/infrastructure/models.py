from __future__ import annotations

import uuid
from datetime import datetime
from typing import Any

from sqlalchemy import JSON, BigInteger, Boolean, DateTime, ForeignKey, Integer, String, Uuid, func
from sqlalchemy.orm import Mapped, mapped_column

from warden.infrastructure.db import Base


class Subscription(Base):
    __tablename__ = "subscriptions"

    id: Mapped[uuid.UUID] = mapped_column(Uuid, primary_key=True, default=uuid.uuid4)
    user_chat_id: Mapped[int] = mapped_column(BigInteger, index=True, nullable=False)

    origin: Mapped[str] = mapped_column(String(8), nullable=False)
    origin_alternatives: Mapped[list[str]] = mapped_column(JSON, default=list, nullable=False)
    destination: Mapped[list[str]] = mapped_column(JSON, nullable=False)

    date_from: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    date_to: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    return_date_from: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    return_date_to: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))

    trip_length_min: Mapped[int | None] = mapped_column(Integer)
    trip_length_max: Mapped[int | None] = mapped_column(Integer)

    max_price: Mapped[float | None] = mapped_column()
    max_stops: Mapped[int | None] = mapped_column(Integer)
    max_duration_minutes: Mapped[int | None] = mapped_column(Integer)

    airlines_whitelist: Mapped[list[str] | None] = mapped_column(JSON)
    airlines_blacklist: Mapped[list[str] | None] = mapped_column(JSON)

    alert_strategy: Mapped[str] = mapped_column(String(32), default="absolute_threshold")
    alert_params: Mapped[dict[str, Any]] = mapped_column(JSON, default=dict, nullable=False)
    cooldown_hours: Mapped[int] = mapped_column(Integer, default=6, nullable=False)
    dry_run: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    status: Mapped[str] = mapped_column(String(16), default="active", nullable=False)

    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), nullable=False
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        server_default=func.now(),
        onupdate=func.now(),
        nullable=False,
    )


class SchedulerRun(Base):
    __tablename__ = "scheduler_runs"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True, autoincrement=True)
    subscription_id: Mapped[uuid.UUID | None] = mapped_column(
        Uuid, ForeignKey("subscriptions.id", ondelete="SET NULL")
    )
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    finished_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    trace_id: Mapped[uuid.UUID] = mapped_column(Uuid, nullable=False)
    flights_found: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    alerts_generated: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    status: Mapped[str] = mapped_column(String(16), nullable=False)
    error: Mapped[str | None] = mapped_column(String(2048))
