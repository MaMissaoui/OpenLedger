# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

OpenLedger is web-based double-entry accounting that ports GnuCash's data model and accounting logic to a Go + React stack. Full design: @docs/ARCHITECTURE.md

## Layout

Monorepo. `apps/api` is a Go module (`github.com/openledger/openledger/apps/api`, Go 1.25); `apps/web` is a React 19 + Vite SPA managed with **pnpm**. `openapi/openapi.yaml` is the API contract.

## Commands

Run from the repo root via the Makefile (the targets `cd` into the right app):

- `make test` — `go test ./...`
- `make vet` / `make fmt` — `go vet` / `gofmt -w`
- `make lint` — `golangci-lint run` (config in `apps/api/.golangci.yml`; install the binary separately)
- `make build` / `make run` — build / run the API
- `make seed` — populate a (migrated) DB with a demo book, chart of accounts, and balanced transactions
- `make up` — full stack (`docker compose up --build`; requires homelab-auth running); `make dev` — Postgres + nginx shim for host-side dev
- `make web-install` / `make web-build` — pnpm install / build (web uses pnpm, not npm)

## Accounting invariants (the rules that must not break)

- **Money is never a float.** Use `domain.GncNumeric` (backed by `math/big.Rat`) everywhere in the money path. On the wire and in the DB, amounts are exact integer `{num, denom}` pairs (`value_num`/`value_denom`, `quantity_num`/`quantity_denom`). Never introduce `float64` for money.
- **Transactions must balance.** The sum of a transaction's split values equals exactly zero in the transaction currency; an unbalanced post returns HTTP 422.
- **Two amounts per split.** `value` is in the transaction currency; `quantity` is in the account's own commodity. They're equal only for same-currency accounts.

## Architecture rules

- `internal/domain` is **pure Go** — no DB or HTTP imports. The accounting kernel lives here so it's testable in isolation. Don't add infrastructure dependencies to it.
- **`PostingService` (`internal/app`) is the only write path for transactions.** It validates the balance invariant and writes the transaction, its splits, and an `audit_log` row inside one `pgx.Tx`. Handlers must not write splits directly.
- **Trading accounts keep the book balanced per-commodity.** When `PostingService` has a `TradingBalancer` (`WithTrading`, wired in `main.go`), a multi-currency transaction gets GnuCash-style `TRADING` splits added *after* its value balance is validated (so they never mask a real imbalance), zeroing each commodity's net quantity and value. The pure engine is `domain.ComputeTradingSplits`; `app.TradingService` resolves account commodities and find-or-creates the `Trading:NAMESPACE:MNEMONIC` accounts. Single-currency posts are unaffected.
- The DB schema (`apps/api/db/migrations`) **deliberately mirrors GnuCash's SQL backend**: 32-char hex GUID primary keys, `*_num`/`*_denom` money columns, GnuCash `account_type` tokens. This is required for import/export fidelity — do not "modernize" the mirrored core tables.
- `openapi/openapi.yaml` is the source of truth for request/response shapes and generates the web TS client. Update it in the same change as any API change.
- **Auth is at the proxy layer (homelab-auth repo → Traefik + Authelia + lldap).** The Go API has no `/auth/*` routes and no JWT logic. `requireAuth` in `transport/httpapi` simply reads the `Remote-User` and `Remote-Email` headers that Traefik injects after Authelia validates the session. On first login, `ProvisionService.ProvisionUser` (in `internal/app`) creates an org + `users` row keyed by `ldap_uid` (idempotent find-or-create). There is no Argon2id hashing or JWT issuing in the HTTP path. The `internal/auth` package remains for the primitives but is no longer used by the server.
- **`make dev` starts only Postgres + a nginx auth shim** (`docker-compose.dev.yml`). The shim listens on port 80 at `openledger.localhost` and injects fake `Remote-User: dev` / `Remote-Email: dev@localhost` headers so `requireAuth` passes without a real Authelia stack. Add `127.0.0.1 openledger.localhost` to `/etc/hosts`. The API runs on port 8090 on the host (`PORT=8090`).
- **Production** (`docker compose up --build`) requires the `homelab-auth` repo to be running first — it provides the `proxy` Docker network, Traefik, Authelia, and lldap. Add `openledger.<DOMAIN>` to Authelia's `access_control` rules in the homelab-auth repo.
- **Book-scoped routes enforce per-book membership and role** via `AuthzService` (`internal/app`): handlers call `authorizeBook`/`authorizeAccount`/`authorizeAccounts` with an `app.Access` level (`AccessRead` for GETs, `AccessWrite` for POSTs). Roles rank `viewer < editor < admin < owner`; reads need `viewer`+, writes need `editor`+. Returns 403 (no membership → `ErrForbidden`; role too low → `ErrInsufficientRole`), 404 if the book/account is unknown. A book's owner membership is written when the book is created. Commodities and prices are shared reference data and are not book-scoped (their routes require auth only, no `authorizeBook`).

## Gotchas (docs vs. code)

- The HTTP layer uses the **stdlib `net/http` 1.22 mux** (`http.NewServeMux` with method patterns), **not chi**.
- **`sqlc` is configured (`sqlc.yaml`) but not yet generated.** The pg repository (`internal/infra/pg/repository.go`) is hand-written pgx; there is no generated `internal/infra/db` package yet.

## Agent skills

### Issue tracker

Issues are tracked in this repo's **GitHub Issues** (via the `gh` CLI). External PRs are **not** a triage surface. See `docs/agents/issue-tracker.md`.

### Triage labels

Canonical label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`), used verbatim. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
