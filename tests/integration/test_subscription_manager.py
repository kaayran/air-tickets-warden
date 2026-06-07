from __future__ import annotations

from datetime import date
from uuid import uuid4

import pytest

from warden.domain.subscription import NewSubscriptionData
from warden.services.subscription_manager import SubscriptionManager

USER_A = 111
USER_B = 222


def _payload(max_price: float | None = 120.0) -> NewSubscriptionData:
    return NewSubscriptionData(
        origin="BEG",
        destination="BCN",
        date_from=date(2026, 7, 10),
        date_to=date(2026, 7, 20),
        max_price=max_price,
    )


@pytest.mark.asyncio
async def test_create_then_list_and_get(manager: SubscriptionManager) -> None:
    view = await manager.create(USER_A, _payload())
    assert view.origin == "BEG"
    assert view.destinations == ["BCN"]
    assert view.max_price == 120.0
    assert view.status == "active"

    listed = await manager.list_for_user(USER_A)
    assert [v.id for v in listed] == [view.id]

    fetched = await manager.get(view.id, USER_A)
    assert fetched is not None
    assert fetched.id == view.id


@pytest.mark.asyncio
async def test_dates_stored_at_midnight(manager: SubscriptionManager) -> None:
    view = await manager.create(USER_A, _payload())
    assert view.date_from is not None and view.date_to is not None
    # Calendar dates map to midnight; the date component round-trips intact.
    assert (view.date_from.hour, view.date_from.minute) == (0, 0)
    assert view.date_from.date() == date(2026, 7, 10)
    assert view.date_to.date() == date(2026, 7, 20)


@pytest.mark.asyncio
async def test_strategy_depends_on_max_price(manager: SubscriptionManager) -> None:
    with_price = await manager.create(USER_A, _payload(max_price=99.0))
    without_price = await manager.create(USER_A, _payload(max_price=None))
    assert with_price.max_price == 99.0
    assert without_price.max_price is None


@pytest.mark.asyncio
async def test_ownership_scoping(manager: SubscriptionManager) -> None:
    view = await manager.create(USER_A, _payload())

    # User B cannot see, fetch, mutate, or delete user A's subscription.
    assert await manager.list_for_user(USER_B) == []
    assert await manager.get(view.id, USER_B) is None
    assert await manager.set_status(view.id, USER_B, "paused") is None
    assert await manager.delete(view.id, USER_B) is False

    # Untouched for the real owner.
    still = await manager.get(view.id, USER_A)
    assert still is not None and still.status == "active"


@pytest.mark.asyncio
async def test_set_status_pause_resume(manager: SubscriptionManager) -> None:
    view = await manager.create(USER_A, _payload())
    paused = await manager.set_status(view.id, USER_A, "paused")
    assert paused is not None and paused.status == "paused"
    resumed = await manager.set_status(view.id, USER_A, "active")
    assert resumed is not None and resumed.status == "active"


@pytest.mark.asyncio
async def test_delete(manager: SubscriptionManager) -> None:
    view = await manager.create(USER_A, _payload())
    assert await manager.delete(view.id, USER_A) is True
    assert await manager.get(view.id, USER_A) is None
    assert await manager.list_for_user(USER_A) == []


@pytest.mark.asyncio
async def test_get_missing_returns_none(manager: SubscriptionManager) -> None:
    assert await manager.get(uuid4(), USER_A) is None
