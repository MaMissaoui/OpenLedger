.PHONY: test build run seed dev up down fmt vet lint web-install web-build start stop migrate

# --- Go API ---
test:
	cd apps/api && go test ./...

vet:
	cd apps/api && go vet ./...

lint:
	cd apps/api && golangci-lint run

fmt:
	cd apps/api && gofmt -w .

build:
	cd apps/api && go build ./...

run:
	cd apps/api && PORT=8090 go run ./cmd/server

seed:
	cd apps/api && go run ./cmd/seed

# --- Web ---
web-install:
	cd apps/web && pnpm install

web-build:
	cd apps/web && pnpm build

# --- Local dev (no auth infrastructure needed) ---
# Starts Postgres, runs migrations, builds and starts the API, then launches
# the Vite dev server in the foreground. Ctrl-C stops everything cleanly.
# Open http://localhost:5173 — auth is handled by the Vite proxy.
start:
	./scripts/dev.sh

# Stop a dev stack that was abandoned without Ctrl-C (e.g. terminal closed).
stop:
	@if [ -f .dev.pid ]; then \
		kill $$(cat .dev.pid) 2>/dev/null && echo "→ API stopped" || true; \
		rm -f .dev.pid; \
	fi
	@rm -f .dev-api
	docker compose -f docker-compose.dev.yml stop db

# Run database migrations against the local dev Postgres.
# Override DSN with: make migrate DEV_DSN="postgres://..."
DEV_DSN ?= postgres://openledger:openledger@localhost:5432/openledger?sslmode=disable
migrate:
	goose -dir apps/api/db/migrations postgres "$(DEV_DSN)" up

# --- Docker (full stack with Authelia) ---
up:
	docker compose up --build

down:
	docker compose down

# Legacy: Postgres + nginx auth shim (requires openledger.localhost in /etc/hosts).
dev:
	docker compose -f docker-compose.dev.yml up
