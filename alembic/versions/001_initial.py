"""initial schema: subscriptions + scheduler_runs

Revision ID: 001
Revises:
Create Date: 2026-05-23 00:00:00

"""

from __future__ import annotations

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op

revision: str = "001"
down_revision: str | None = None
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "subscriptions",
        sa.Column("id", sa.Uuid(), primary_key=True),
        sa.Column("user_chat_id", sa.BigInteger(), nullable=False),
        sa.Column("origin", sa.String(length=8), nullable=False),
        sa.Column("origin_alternatives", sa.JSON(), nullable=False),
        sa.Column("destination", sa.JSON(), nullable=False),
        sa.Column("date_from", sa.DateTime(timezone=True), nullable=True),
        sa.Column("date_to", sa.DateTime(timezone=True), nullable=True),
        sa.Column("return_date_from", sa.DateTime(timezone=True), nullable=True),
        sa.Column("return_date_to", sa.DateTime(timezone=True), nullable=True),
        sa.Column("trip_length_min", sa.Integer(), nullable=True),
        sa.Column("trip_length_max", sa.Integer(), nullable=True),
        sa.Column("max_price", sa.Float(), nullable=True),
        sa.Column("max_stops", sa.Integer(), nullable=True),
        sa.Column("max_duration_minutes", sa.Integer(), nullable=True),
        sa.Column("airlines_whitelist", sa.JSON(), nullable=True),
        sa.Column("airlines_blacklist", sa.JSON(), nullable=True),
        sa.Column(
            "alert_strategy",
            sa.String(length=32),
            nullable=False,
            server_default="absolute_threshold",
        ),
        sa.Column("alert_params", sa.JSON(), nullable=False),
        sa.Column("cooldown_hours", sa.Integer(), nullable=False, server_default="6"),
        sa.Column("dry_run", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("status", sa.String(length=16), nullable=False, server_default="active"),
        sa.Column(
            "created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
        sa.Column(
            "updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False
        ),
    )
    op.create_index("ix_subscriptions_user_chat_id", "subscriptions", ["user_chat_id"])

    op.create_table(
        "scheduler_runs",
        sa.Column("id", sa.BigInteger(), primary_key=True, autoincrement=True),
        sa.Column(
            "subscription_id",
            sa.Uuid(),
            sa.ForeignKey("subscriptions.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("finished_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("trace_id", sa.Uuid(), nullable=False),
        sa.Column("flights_found", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("alerts_generated", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("status", sa.String(length=16), nullable=False),
        sa.Column("error", sa.String(length=2048), nullable=True),
    )
    op.create_index("ix_scheduler_runs_started_at", "scheduler_runs", ["started_at"])


def downgrade() -> None:
    op.drop_index("ix_scheduler_runs_started_at", table_name="scheduler_runs")
    op.drop_table("scheduler_runs")
    op.drop_index("ix_subscriptions_user_chat_id", table_name="subscriptions")
    op.drop_table("subscriptions")
