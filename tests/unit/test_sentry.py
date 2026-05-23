from __future__ import annotations

from unittest.mock import patch

from pydantic import SecretStr

from warden.config import Settings
from warden.infrastructure.telemetry import sentry as sentry_module


def test_sentry_noop_without_dsn(settings: Settings) -> None:
    assert settings.sentry_dsn is None
    with patch.object(sentry_module.sentry_sdk, "init") as mock_init:
        sentry_module.init_sentry(settings)
        mock_init.assert_not_called()


def test_sentry_initializes_when_dsn_set(settings: Settings) -> None:
    settings_with_dsn = settings.model_copy(
        update={"sentry_dsn": SecretStr("https://example@sentry.io/1")}
    )
    with patch.object(sentry_module.sentry_sdk, "init") as mock_init:
        sentry_module.init_sentry(settings_with_dsn)
        mock_init.assert_called_once()
        kwargs = mock_init.call_args.kwargs
        assert kwargs["environment"] == "test"
        assert kwargs["dsn"] == "https://example@sentry.io/1"
