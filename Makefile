.PHONY: test build run seed dev up down fmt vet lint web-install web-build

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
	cd apps/api && go run ./cmd/server

seed:
	cd apps/api && go run ./cmd/seed

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
