from __future__ import annotations

import pytest

from warden.config import Settings, get_settings


def test_settings_loads_from_env(env_overrides: dict[str, str]) -> None:
    settings = get_settings()
    assert settings.bot_token.get_secret_value() == env_overrides["BOT_TOKEN"]
    assert settings.allowed_user_ids == [int(env_overrides["ALLOWED_USER_IDS"])]
    assert settings.environment == "test"
    assert settings.sentry_dsn is None


def test_settings_parses_csv_user_ids(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("BOT_TOKEN", "x")
    monkeypatch.setenv("ALLOWED_USER_IDS", "1,2 ,  3")
    get_settings.cache_clear()
    s = Settings()  # type: ignore[call-arg]
    assert s.allowed_user_ids == [1, 2, 3]


def test_settings_requires_allowed_users(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("BOT_TOKEN", "x")
    monkeypatch.setenv("ALLOWED_USER_IDS", "")
    get_settings.cache_clear()
    with pytest.raises(ValueError, match="ALLOWED_USER_IDS"):
        Settings()  # type: ignore[call-arg]


def test_settings_requires_bot_token(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("BOT_TOKEN", raising=False)
    monkeypatch.setenv("ALLOWED_USER_IDS", "1")
    get_settings.cache_clear()
    with pytest.raises(Exception):
        Settings()  # type: ignore[call-arg]


def test_log_level_validated(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("BOT_TOKEN", "x")
    monkeypatch.setenv("ALLOWED_USER_IDS", "1")
    monkeypatch.setenv("LOG_LEVEL", "verbose")
    get_settings.cache_clear()
    with pytest.raises(ValueError, match="LOG_LEVEL"):
        Settings()  # type: ignore[call-arg]
