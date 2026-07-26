.DEFAULT_GOAL := help

# Single source of truth for the sqlc version — CI runs `make sqlc`, so the
# drift check can never disagree with local generation.
SQLC_VERSION := v1.31.1

.PHONY: help run dev test lint fmt migrate sqlc web-build docker tunnel db-up db-down

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

run: ## Run the service (needs Postgres up and PUBLIC_URL set)
	go run ./cmd/warden run

dev: ## One command: Postgres + cloudflared tunnel + app (for the phone test)
	bash scripts/dev.sh

test: ## Run Go + web tests
	go test -short ./...
	cd web && npm test

lint: ## Lint Go + web
	golangci-lint run
	cd web && npm run lint

fmt: ## Format Go and tidy modules
	gofmt -w .
	go mod tidy

migrate: ## Apply DB migrations
	go run ./cmd/warden migrate

sqlc: ## Regenerate typed queries from db/queries
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

web-build: ## Build the Mini App into web/dist (embedded by the binary)
	cd web && npm ci && npm run build

docker: ## Build the production image
	docker build -t air-tickets-warden .

tunnel: ## Start a cloudflared quick tunnel to the local app (:8080)
	cloudflared tunnel --url http://localhost:8080

db-up: ## Start the local Postgres container
	docker compose up -d postgres

db-down: ## Stop the local stack
	docker compose down
