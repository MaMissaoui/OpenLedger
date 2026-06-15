#!/usr/bin/env bash
# scripts/dev.sh — Start the full OpenLedger development stack on localhost.
#
# Auth is handled by the Vite dev-server proxy which injects Remote-User / Remote-Email
# headers directly to the API, so no Authelia, Traefik, or /etc/hosts entry is needed.
#
# What runs:
#   • Postgres  — Docker (docker-compose.dev.yml db service), port 5432
#   • API       — compiled Go binary, port 8090 (or $PORT)
#   • Web       — Vite dev server, port 5173 (foreground; Ctrl-C stops everything)
#
# Usage:  make start   (or ./scripts/dev.sh directly from the repo root)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/docker-compose.dev.yml"
MIGRATIONS_DIR="$REPO_ROOT/apps/api/db/migrations"
API_DIR="$REPO_ROOT/apps/api"
WEB_DIR="$REPO_ROOT/apps/web"
API_BIN="$REPO_ROOT/.dev-api"
PID_FILE="$REPO_ROOT/.dev.pid"

DEV_DSN="${DATABASE_URL:-postgres://openledger:openledger@localhost:5432/openledger?sslmode=disable}"
API_PORT="${PORT:-8090}"

# ── Cleanup on exit (covers Ctrl-C, errors, and normal exit) ──────────────────
cleanup() {
  echo ""
  echo "→ Stopping…"
  if [[ -f "$PID_FILE" ]]; then
    kill "$(cat "$PID_FILE")" 2>/dev/null || true
    rm -f "$PID_FILE"
  fi
  rm -f "$API_BIN"
  docker compose -f "$COMPOSE_FILE" stop db 2>/dev/null || true
  echo "→ Done."
}
trap cleanup EXIT

# ── Prerequisites ──────────────────────────────────────────────────────────────
need() {
  command -v "$1" &>/dev/null || { echo "✗ '$1' not found — please install it and retry"; exit 1; }
}
need docker
need goose
need go
need pnpm

# ── 1. Postgres ────────────────────────────────────────────────────────────────
echo "→ Starting Postgres…"
docker compose -f "$COMPOSE_FILE" up -d db

echo -n "  Waiting for Postgres"
READY=0
for i in $(seq 1 30); do
  if docker compose -f "$COMPOSE_FILE" exec -T db \
       pg_isready -U openledger -d openledger -q 2>/dev/null; then
    READY=1; echo " ready"; break
  fi
  echo -n "."; sleep 1
done
[[ $READY -eq 1 ]] || { echo ""; echo "✗ Postgres not ready after 30 s"; exit 1; }

# ── 2. Migrations ──────────────────────────────────────────────────────────────
echo "→ Running migrations…"
goose -dir "$MIGRATIONS_DIR" postgres "$DEV_DSN" up

# ── 3. API ─────────────────────────────────────────────────────────────────────
echo "→ Building API…"
(cd "$API_DIR" && go build -o "$API_BIN" ./cmd/server)

echo "→ Starting API on :${API_PORT}…"
PORT="$API_PORT" DATABASE_URL="$DEV_DSN" "$API_BIN" &
echo $! > "$PID_FILE"

echo -n "  Waiting for API"
READY=0
for i in $(seq 1 20); do
  if curl -sf "http://localhost:${API_PORT}/healthz" &>/dev/null; then
    READY=1; echo " ready"; break
  fi
  echo -n "."; sleep 1
done
[[ $READY -eq 1 ]] || { echo ""; echo "✗ API did not start after 20 s — check output above"; exit 1; }

# ── 4. Web dependencies ────────────────────────────────────────────────────────
echo "→ Checking web dependencies…"
(cd "$WEB_DIR" && pnpm install)

# ── 5. Vite dev server (foreground — Ctrl-C triggers cleanup above) ────────────
echo ""
echo "  OpenLedger running"
echo "  App  →  http://localhost:5173"
echo "  API  →  http://localhost:${API_PORT}"
echo "  Auth →  Vite proxy (Remote-User: dev, no Authelia needed)"
echo "  Stop →  Ctrl-C"
echo ""

(cd "$WEB_DIR" && pnpm dev)
