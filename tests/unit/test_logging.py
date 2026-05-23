from __future__ import annotations

import json
import logging

import structlog

from warden.infrastructure.telemetry.logging import setup_logging


def test_logging_emits_json(capsys: object) -> None:
    setup_logging(log_level="INFO", json=True)
    logger = structlog.get_logger("test")
    structlog.contextvars.clear_contextvars()
    structlog.contextvars.bind_contextvars(user_id=42, command="help")

    logger.info("update_handled", duration_ms=12.3)

    captured = capsys.readouterr()  # type: ignore[attr-defined]
    line = captured.err.strip().splitlines()[-1]
    payload = json.loads(line)
    assert payload["event"] == "update_handled"
    assert payload["user_id"] == 42
    assert payload["command"] == "help"
    assert payload["duration_ms"] == 12.3
    assert payload["level"] == "info"
    structlog.contextvars.clear_contextvars()
    logging.getLogger().handlers.clear()


def test_logging_console_mode_runs(capsys: object) -> None:
    setup_logging(log_level="DEBUG", json=False)
    logger = structlog.get_logger("test")
    logger.info("hello", foo="bar")
    captured = capsys.readouterr()  # type: ignore[attr-defined]
    assert "hello" in captured.err
    logging.getLogger().handlers.clear()
