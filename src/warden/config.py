from __future__ import annotations

from functools import lru_cache
from typing import Annotated

from pydantic import Field, SecretStr, field_validator
from pydantic_settings import BaseSettings, NoDecode, SettingsConfigDict


class Settings(BaseSettings):
    bot_token: SecretStr
    allowed_user_ids: Annotated[list[int], NoDecode] = Field(default_factory=list)

    database_url: str = "sqlite+aiosqlite:///./warden.db"

    web_host: str = "0.0.0.0"
    web_port: int = 9090

    log_level: str = "INFO"
    log_json: bool = True
    sentry_dsn: SecretStr | None = None
    environment: str = "dev"

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
        case_sensitive=False,
    )

    @field_validator("allowed_user_ids", mode="before")
    @classmethod
    def _split_csv(cls, value: object) -> object:
        if isinstance(value, str):
            stripped = value.strip()
            if not stripped:
                return []
            return [int(part.strip()) for part in stripped.split(",") if part.strip()]
        return value

    @field_validator("allowed_user_ids")
    @classmethod
    def _require_at_least_one(cls, value: list[int]) -> list[int]:
        if not value:
            raise ValueError("ALLOWED_USER_IDS must contain at least one Telegram user id")
        return value

    @field_validator("log_level")
    @classmethod
    def _validate_log_level(cls, value: str) -> str:
        normalized = value.upper()
        valid = {"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}
        if normalized not in valid:
            raise ValueError(f"LOG_LEVEL must be one of {sorted(valid)}, got {value!r}")
        return normalized


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()
