.PHONY: help run test lint fmt mypy migrate revision docker clean

help:
	@echo "Available targets:"
	@echo "  run       — start the bot locally"
	@echo "  test      — run the test suite with coverage"
	@echo "  lint      — ruff check + ruff format --check + mypy"
	@echo "  fmt       — format with ruff (also runs --fix)"
	@echo "  mypy      — strict type check"
	@echo "  migrate   — alembic upgrade head"
	@echo "  revision  — make m=\"message\" — autogenerate revision"
	@echo "  docker    — docker compose up --build"
	@echo "  clean     — remove caches and local DB"

run:
	uv run python -m warden.main

test:
	uv run pytest --cov=src/warden --cov-report=term-missing --cov-fail-under=80

lint:
	uv run ruff check .
	uv run ruff format --check .
	uv run mypy src/warden

fmt:
	uv run ruff format .
	uv run ruff check --fix .

mypy:
	uv run mypy src/warden

migrate:
	uv run alembic upgrade head

revision:
	uv run alembic revision --autogenerate -m "$(m)"

docker:
	docker compose -f docker/docker-compose.yml up --build

clean:
	rm -rf .pytest_cache .mypy_cache .ruff_cache .coverage htmlcov
	find . -type d -name __pycache__ -exec rm -rf {} +
