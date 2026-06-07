from __future__ import annotations

from datetime import UTC, date, datetime, time
from typing import Any
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from warden.domain.subscription import NewSubscriptionData, SubscriptionView
from warden.infrastructure.db import session_scope
from warden.infrastructure.models import Subscription

ACTIVE_STATUSES = ("active", "paused")


def _to_utc_midnight(d: date) -> datetime:
    """Map a calendar date to a TZ-aware UTC midnight (DB stores everything in UTC)."""
    return datetime.combine(d, time.min, tzinfo=UTC)


def _to_view(row: Subscription) -> SubscriptionView:
    return SubscriptionView(
        id=row.id,
        origin=row.origin,
        destinations=list(row.destination),
        date_from=row.date_from,
        date_to=row.date_to,
        max_price=row.max_price,
        status=row.status,
        created_at=row.created_at,
    )


class SubscriptionManager:
    """CRUD over monitoring rules. Every query is scoped by ``user_chat_id`` so a
    user can never read or mutate another user's subscription."""

    def __init__(self, sessionmaker: async_sessionmaker[AsyncSession]) -> None:
        self._sessionmaker = sessionmaker

    async def create(self, user_chat_id: int, data: NewSubscriptionData) -> SubscriptionView:
        if data.max_price is not None:
            strategy = "absolute_threshold"
            params: dict[str, Any] = {"max_price": data.max_price}
        else:
            strategy = "historical_minimum"
            params = {"window_days": 60}

        row = Subscription(
            user_chat_id=user_chat_id,
            origin=data.origin,
            origin_alternatives=[],
            destination=[data.destination],
            date_from=_to_utc_midnight(data.date_from),
            date_to=_to_utc_midnight(data.date_to),
            max_price=data.max_price,
            alert_strategy=strategy,
            alert_params=params,
        )
        async with session_scope(self._sessionmaker) as session:
            session.add(row)
            await session.commit()
            await session.refresh(row)
            return _to_view(row)

    async def list_for_user(self, user_chat_id: int) -> list[SubscriptionView]:
        stmt = (
            select(Subscription)
            .where(Subscription.user_chat_id == user_chat_id)
            .where(Subscription.status.in_(ACTIVE_STATUSES))
            .order_by(Subscription.created_at)
        )
        async with session_scope(self._sessionmaker) as session:
            result = await session.execute(stmt)
            return [_to_view(row) for row in result.scalars().all()]

    async def get(self, subscription_id: UUID, user_chat_id: int) -> SubscriptionView | None:
        async with session_scope(self._sessionmaker) as session:
            row = await self._get_owned(session, subscription_id, user_chat_id)
            return _to_view(row) if row is not None else None

    async def set_status(
        self, subscription_id: UUID, user_chat_id: int, status: str
    ) -> SubscriptionView | None:
        async with session_scope(self._sessionmaker) as session:
            row = await self._get_owned(session, subscription_id, user_chat_id)
            if row is None:
                return None
            row.status = status
            await session.commit()
            await session.refresh(row)
            return _to_view(row)

    async def delete(self, subscription_id: UUID, user_chat_id: int) -> bool:
        async with session_scope(self._sessionmaker) as session:
            row = await self._get_owned(session, subscription_id, user_chat_id)
            if row is None:
                return False
            await session.delete(row)
            await session.commit()
            return True

    @staticmethod
    async def _get_owned(
        session: AsyncSession, subscription_id: UUID, user_chat_id: int
    ) -> Subscription | None:
        stmt = (
            select(Subscription)
            .where(Subscription.id == subscription_id)
            .where(Subscription.user_chat_id == user_chat_id)
        )
        result = await session.execute(stmt)
        return result.scalar_one_or_none()
