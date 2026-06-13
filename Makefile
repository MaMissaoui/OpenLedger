.PHONY: test build run dev up down fmt vet web-install web-build

# --- Go API ---
test:
	cd apps/api && go test ./...

vet:
	cd apps/api && go vet ./...

fmt:
	cd apps/api && gofmt -w .

build:
	cd apps/api && go build ./...

run:
	cd apps/api && go run ./cmd/server

# --- Web ---
web-install:
	cd apps/web && pnpm install

web-build:
	cd apps/web && pnpm build

# --- Docker ---
up:
	docker compose up --build

down:
	docker compose down

dev:
	docker compose -f docker-compose.dev.yml up
