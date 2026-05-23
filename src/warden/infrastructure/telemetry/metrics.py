from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Gauge, Histogram

REGISTRY = CollectorRegistry()

bot_updates_total = Counter(
    "warden_bot_updates_total",
    "Total Telegram updates received and handled.",
    labelnames=("type", "command"),
    registry=REGISTRY,
)

bot_updates_dropped_total = Counter(
    "warden_bot_updates_dropped_total",
    "Updates dropped before reaching a handler (e.g. by whitelist).",
    labelnames=("reason",),
    registry=REGISTRY,
)

command_duration_seconds = Histogram(
    "warden_command_duration_seconds",
    "Wall-clock duration of bot command handlers.",
    labelnames=("command",),
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0),
    registry=REGISTRY,
)

db_ping_failures_total = Counter(
    "warden_db_ping_failures_total",
    "Failed DB pings (from /health and bot /health command).",
    registry=REGISTRY,
)

build_info = Gauge(
    "warden_build_info",
    "Static build/runtime information (set to 1, useful for label joins).",
    labelnames=("version", "environment"),
    registry=REGISTRY,
)


def set_build_info(*, version: str, environment: str) -> None:
    build_info.labels(version=version, environment=environment).set(1)
