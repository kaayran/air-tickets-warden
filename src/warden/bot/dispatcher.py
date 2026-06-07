from __future__ import annotations

from aiogram import Bot, Dispatcher
from aiogram.client.default import DefaultBotProperties
from aiogram.fsm.storage.memory import MemoryStorage
from sqlalchemy.ext.asyncio import AsyncEngine

from warden.bot.handlers import common, search, subscriptions
from warden.bot.middlewares.logging import LoggingMiddleware
from warden.bot.middlewares.whitelist import WhitelistMiddleware
from warden.config import Settings
from warden.infrastructure.db import make_sessionmaker
from warden.services.subscription_manager import SubscriptionManager


def make_bot(settings: Settings) -> Bot:
    return Bot(
        token=settings.bot_token.get_secret_value(),
        default=DefaultBotProperties(parse_mode=None),
    )


def make_dispatcher(settings: Settings, engine: AsyncEngine) -> Dispatcher:
    dp = Dispatcher(storage=MemoryStorage())

    dp.update.outer_middleware(WhitelistMiddleware(settings.allowed_user_ids))
    dp.update.outer_middleware(LoggingMiddleware())

    dp.include_router(common.router)
    dp.include_router(subscriptions.router)
    dp.include_router(search.router)

    dp["engine"] = engine
    dp["settings"] = settings
    dp["subscriptions"] = SubscriptionManager(make_sessionmaker(engine))

    return dp
