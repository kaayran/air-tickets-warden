from __future__ import annotations

import os
from collections.abc import AsyncIterator, Iterator
from datetime import UTC
from pathlib import Path
from typing import Any

import pytest
import pytest_asyncio
from aiogram import Bot, Dispatcher
from aiogram.client.default import DefaultBotProperties
from aiogram.client.session.base import BaseSession
from aiogram.methods import (
    AnswerCallbackQuery,
    DeleteMessage,
    EditMessageReplyMarkup,
    EditMessageText,
    SendMessage,
    TelegramMethod,
)
from aiogram.methods.base import Response, TelegramType
from aiogram.types import CallbackQuery, Chat, Message, Update, User
from alembic import command
from alembic.config import Config as AlembicConfig
from sqlalchemy.ext.asyncio import AsyncEngine

from warden.bot.dispatcher import make_dispatcher
from warden.bot.handlers import common, search, subscriptions
from warden.config import Settings, get_settings
from warden.infrastructure import db as db_module
from warden.services.subscription_manager import SubscriptionManager

PROJECT_ROOT = Path(__file__).resolve().parents[1]
ALEMBIC_INI = PROJECT_ROOT / "alembic.ini"

ALLOWED_USER_ID = 111
DENIED_USER_ID = 999


@pytest.fixture(autouse=True)
def _hermetic_config(monkeypatch: pytest.MonkeyPatch) -> None:
    """Ignore any developer-local .env in the project root for every test.

    Otherwise it leaks into Settings (e.g. an empty SENTRY_DSN= or a real
    BOT_TOKEN) and makes config tests non-deterministic across machines.
    """
    monkeypatch.setitem(Settings.model_config, "env_file", None)


@pytest.fixture
def env_overrides(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> dict[str, str]:
    db_file = tmp_path / "warden_test.db"
    overrides = {
        "BOT_TOKEN": "123456:test-token",
        "ALLOWED_USER_IDS": str(ALLOWED_USER_ID),
        "DATABASE_URL": f"sqlite+aiosqlite:///{db_file}",
        "WEB_HOST": "127.0.0.1",
        "WEB_PORT": "0",
        "LOG_LEVEL": "INFO",
        "LOG_JSON": "true",
        "ENVIRONMENT": "test",
    }
    for key, value in overrides.items():
        monkeypatch.setenv(key, value)
    monkeypatch.delenv("SENTRY_DSN", raising=False)
    get_settings.cache_clear()
    yield overrides
    get_settings.cache_clear()


@pytest.fixture
def settings(env_overrides: dict[str, str]) -> Settings:
    return get_settings()


@pytest_asyncio.fixture
async def engine(settings: Settings) -> AsyncIterator[AsyncEngine]:
    cfg = AlembicConfig(str(ALEMBIC_INI))
    cfg.set_main_option("sqlalchemy.url", settings.database_url)
    cfg.set_main_option("script_location", str(PROJECT_ROOT / "alembic"))
    os.chdir(PROJECT_ROOT)
    command.upgrade(cfg, "head")

    eng = db_module.make_engine(settings.database_url)
    try:
        yield eng
    finally:
        await eng.dispose()


class MockedSession(BaseSession):
    """Session double that captures outgoing TelegramMethod calls."""

    def __init__(self) -> None:
        super().__init__()
        self.calls: list[TelegramMethod[Any]] = []

    async def close(self) -> None:  # pragma: no cover - interface compliance
        return None

    async def make_request(
        self,
        bot: Bot,
        method: TelegramMethod[TelegramType],
        timeout: int | None = None,
    ) -> TelegramType:
        self.calls.append(method)
        if isinstance(method, SendMessage | EditMessageText):
            return Message.model_validate(  # type: ignore[return-value]
                {
                    "message_id": len(self.calls),
                    "date": 0,
                    "chat": {"id": method.chat_id or 0, "type": "private"},
                    "text": method.text,
                }
            )
        if isinstance(method, EditMessageReplyMarkup | AnswerCallbackQuery | DeleteMessage):
            return True  # type: ignore[return-value]
        # default empty success response object — good enough for stubs
        return Response[TelegramType](ok=True, result=None).result  # type: ignore[return-value]

    async def stream_content(
        self,
        url: str,
        headers: dict[str, Any] | None = None,
        timeout: int = 30,
        chunk_size: int = 65536,
        raise_for_status: bool = True,
    ) -> AsyncIterator[bytes]:  # pragma: no cover - interface compliance
        if False:  # pragma: no cover
            yield b""
        return


@pytest.fixture
def bot() -> Bot:
    session = MockedSession()
    b = Bot(
        token="123456:test-token",
        default=DefaultBotProperties(parse_mode=None),
        session=session,
    )
    return b


@pytest.fixture
def sent_messages(bot: Bot) -> list[dict[str, Any]]:
    """Expose captured outgoing messages as dicts for assertions."""
    session = bot.session
    assert isinstance(session, MockedSession)

    class _View(list[dict[str, Any]]):
        def __len__(self) -> int:
            return sum(1 for m in session.calls if isinstance(m, SendMessage))

        def __getitem__(self, index: int) -> dict[str, Any]:  # type: ignore[override]
            messages = [m for m in session.calls if isinstance(m, SendMessage)]
            m = messages[index]
            return {"chat_id": m.chat_id, "text": m.text, "parse_mode": m.parse_mode}

        def __iter__(self) -> Iterator[dict[str, Any]]:  # type: ignore[override]
            for m in session.calls:
                if isinstance(m, SendMessage):
                    yield {"chat_id": m.chat_id, "text": m.text, "parse_mode": m.parse_mode}

        def __eq__(self, other: object) -> bool:
            return list(iter(self)) == other

    return _View()


@pytest.fixture
def dispatcher(settings: Settings, engine: AsyncEngine) -> Dispatcher:
    # Routers are module-level singletons; detach any previous test's dispatcher first.
    for router in (common.router, subscriptions.router, search.router):
        router._parent_router = None
    return make_dispatcher(settings, engine)


@pytest.fixture
def make_update() -> Iterator[Any]:
    """Factory: build an Update wrapping a Message from a user."""
    counter = {"n": 0}

    def _factory(text: str, user_id: int = ALLOWED_USER_ID) -> Update:
        counter["n"] += 1
        n = counter["n"]
        user = User(id=user_id, is_bot=False, first_name="Test", username="test")
        chat = Chat(id=user_id, type="private")
        from datetime import datetime

        message = Message.model_validate(
            {
                "message_id": n,
                "date": datetime.now(tz=UTC),
                "chat": chat.model_dump(),
                "from": user.model_dump(),
                "text": text,
            }
        )
        return Update(update_id=n, message=message)

    yield _factory


@pytest.fixture
def make_callback() -> Iterator[Any]:
    """Factory: build an Update wrapping a CallbackQuery on a bot-authored message."""
    counter = {"n": 0}

    def _factory(data: str, user_id: int = ALLOWED_USER_ID) -> Update:
        counter["n"] += 1
        n = counter["n"]
        from datetime import datetime

        user = User(id=user_id, is_bot=False, first_name="Test", username="test")
        chat = Chat(id=user_id, type="private")
        bot_user = User(id=42, is_bot=True, first_name="Warden", username="warden_bot")
        message = Message.model_validate(
            {
                "message_id": n,
                "date": datetime.now(tz=UTC),
                "chat": chat.model_dump(),
                "from": bot_user.model_dump(),
                "text": "…",
            }
        )
        callback = CallbackQuery.model_validate(
            {
                "id": str(n),
                "from": user.model_dump(),
                "chat_instance": "test-instance",
                "message": message.model_dump(),
                "data": data,
            }
        )
        return Update(update_id=n, callback_query=callback)

    yield _factory


@pytest_asyncio.fixture
async def manager(engine: AsyncEngine) -> SubscriptionManager:
    """SubscriptionManager backed by the migrated test DB for direct service tests."""
    return SubscriptionManager(db_module.make_sessionmaker(engine))


@pytest.fixture
def texts(bot: Bot) -> list[str]:
    """All text-bearing outgoing calls (SendMessage + EditMessageText) in call order."""
    session = bot.session
    assert isinstance(session, MockedSession)

    class _View(list[str]):
        def _items(self) -> list[str]:
            return [
                m.text or "" for m in session.calls if isinstance(m, SendMessage | EditMessageText)
            ]

        def __len__(self) -> int:
            return len(self._items())

        def __getitem__(self, index: int) -> str:  # type: ignore[override]
            return self._items()[index]

        def __iter__(self) -> Iterator[str]:  # type: ignore[override]
            return iter(self._items())

    return _View()
