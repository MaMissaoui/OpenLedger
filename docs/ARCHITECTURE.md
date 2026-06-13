# OpenLedger — Architecture & Scaffolding Plan

## Context

OpenLedger is a web-based accounting application for **personal finance and small businesses**, reusing the proven **data model and accounting logic of GnuCash** (the mature desktop double-entry app). The goal is to keep GnuCash's correctness guarantees (true double-entry, exact rational arithmetic, multi-currency, lots) while delivering a modern multi-user web experience.

Key decisions:
- **Scope:** Personal **+ small business** (customers/vendors, invoices, bills, A/R & A/P, tax tables, payment terms) — built in phases starting with the personal-finance ledger core.
- **Stack:** **Go backend** (HTTP API with a `math/big.Rat`-based money kernel, `pgx` + `sqlc`) and a **React/TypeScript** frontend on **PostgreSQL**. API types are bridged to the frontend via OpenAPI codegen (no shared runtime, generated TS client).
- **GnuCash compatibility:** Must **import and export GnuCash files**, so our schema mirrors GnuCash's SQL backend closely.
- **Deployment:** Ships as **Docker images orchestrated by `docker compose`** (Postgres + Go API + web), with a tiny distroless API image.

**Is it possible?** Yes. GnuCash's engine is a clean, well-specified double-entry kernel (`Book → Accounts → Transactions → Splits`, plus `Commodities`/`Prices`, `Lots`, `Scheduled Transactions`, and business objects). It already ships a relational SQL backend (SQLite/MySQL/PostgreSQL) whose table layout is public and stable, so the data model ports directly. The genuinely new work is the web layer: authentication, multi-user concurrency, an HTTP API, and a browser UI — none of which the single-user desktop app needed.

**Why Go for the backend?** Money in GnuCash is exact rational arithmetic (numerator/denominator pairs). Go's standard-library `math/big.Rat` *is* a rational number type, so the money kernel wraps it thinly instead of hand-rolling BigInt math. Go also produces a single static binary → a ~15–25 MB distroless container with sub-second startup, which suits the Docker deployment goal. `sqlc` generates type-safe Go from hand-written SQL, pairing well with a schema that deliberately mirrors GnuCash. The one trade-off vs. an all-TypeScript stack — losing shared types with the frontend — is handled with OpenAPI-generated TS client types.

---

## 1. What we are porting from GnuCash

GnuCash's domain kernel, which we reuse conceptually and (for the schema) structurally:

| Concept | Role | GnuCash table |
|---|---|---|
| **Book** | Top-level container for one set of books | `books` |
| **Commodity** | A currency *or* a tradable (stock/fund). Has a `fraction` (smallest unit, e.g. 100 for USD cents) | `commodities` |
| **Account** | Node in a tree; typed (ASSET, BANK, EXPENSE, INCOME, EQUITY, LIABILITY, RECEIVABLE, PAYABLE, STOCK, TRADING, …); denominated in one commodity | `accounts` |
| **Transaction** | A dated economic event; denominated in one currency | `transactions` |
| **Split** | One leg of a transaction posted to one account. Carries **value** (in tx currency) and **quantity** (in account commodity) | `splits` |
| **Price** | Exchange rate / quote for a commodity on a date | `prices` |
| **Lot** | Groups splits for cost-basis / capital-gains tracking | `lots` |
| **Slot** | Key/value metadata attached to any object (KVP) | `slots` |
| **Scheduled Tx** | Template transaction + recurrence rule | `schedules`, `recurrences`, template txns |
| **Budget** | Per-account per-period planned amounts | `budgets`, `budget_amounts` |
| **Business** | Customers, vendors, employees, invoices, bills, entries, jobs, terms, tax tables | `customers`, `vendors`, `employees`, `invoices`, `bills`, `entries`, `jobs`, `billterms`, `taxtables`, `taxtable_entries` |

### Non-negotiable accounting invariants (the "logic" we must preserve)

1. **Balanced transactions:** for every transaction, the sum of its splits' `value` (in the transaction's currency) **equals exactly zero**. The posting service rejects any transaction that does not balance.
2. **Exact arithmetic, never floats.** GnuCash stores every monetary amount as a **rational number** — an integer numerator over an integer denominator (`value_num`/`value_denom`, `quantity_num`/`quantity_denom`). We mirror this with `math/big.Rat`. Floating point is banned in the money path.
3. **Two amounts per split:** `value` is in the transaction currency; `quantity` is in the account's own commodity. For same-currency accounts they are equal. For foreign-currency / stock accounts they differ, and the ratio is an implied exchange rate or share price.
4. **Account-type sign semantics:** debit/credit normality is derived from account type. Reports and the UI must respect each type's natural balance sign.
5. **Multi-currency correctness:** when a transaction spans currencies it must still balance to zero in its transaction currency; GnuCash's optional **Trading Accounts** mechanism keeps the *whole book* balanced per-currency. We support trading accounts to remain export-compatible.
6. **Lots for cost basis:** sales of shares/securities are linked via lots to their purchase splits so realized gains compute correctly.

---

## 2. Data model & database schema

**Strategy:** mirror GnuCash's SQL backend table structure so import/export is a near-direct mapping, then layer our own tables (users, orgs, sessions, audit) *around* it rather than altering the GnuCash core tables.

- **Database:** PostgreSQL (GnuCash itself supports a Postgres backend, which keeps us close to its expectations).
- **Data access:** **`pgx`** driver + **`sqlc`** — hand-written SQL compiled into type-safe Go. No ORM; the SQL stays explicit, which matters when the schema is a deliberate GnuCash mirror.
- **Migrations:** **`goose`** (or `golang-migrate`) with plain SQL migration files checked into the repo.
- **Money columns:** store GnuCash-style `*_num BIGINT` + `*_denom BIGINT` pairs in the GnuCash-mirroring tables (required for fidelity). Internally, wrap them in a `GncNumeric` value object backed by `math/big.Rat` (auto-reduced), used everywhere in the domain layer. Never expose a float for money.
- **Identifiers:** GnuCash uses 32-char hex GUIDs as primary keys across all core tables. We keep that format for the mirrored tables (so exported files are valid) and may use native UUIDs for our own web-only tables.
- **Multi-tenant:** each GnuCash **Book** maps to one "ledger/company." Our own `organizations` and `users` tables sit above books; a user has roles on one or more books. This is the main structural addition over desktop GnuCash, which is strictly single-book-per-file/single-user.

### Core tables (GnuCash-mirrored, phase 1)
`books`, `commodities`, `accounts`, `transactions`, `splits`, `prices`, `slots`, `versions`.

### Later tables (phase-gated)
`lots`, `schedules`/`recurrences` + template transactions, `budgets`/`budget_amounts` (phase 3); `customers`, `vendors`, `employees`, `invoices`, `bills`, `entries`, `jobs`, `billterms`, `taxtables`, `taxtable_entries` (phase 4).

### Web-only tables (new, not in GnuCash)
`organizations`, `users`, `memberships` (user↔book + role), `sessions`/`refresh_tokens`, `audit_log` (append-only record of every posting/edit for traceability), `import_jobs`.

---

## 3. Technology stack

| Layer | Choice | Why |
|---|---|---|
| Backend language | **Go** (1.22+) | Static binary, tiny container, goroutines for async import jobs; team experience |
| HTTP | **chi** router (or std `net/http` 1.22 routing) | Lightweight, idiomatic, no heavy framework |
| Money | `GncNumeric` wrapping **`math/big.Rat`** | Exact rational arithmetic, direct GnuCash fidelity |
| DB | **PostgreSQL** | GnuCash-compatible backend; transactional integrity for balanced posting |
| Data access | **`pgx`** + **`sqlc`** (type-safe SQL) | Explicit SQL over a GnuCash-mirrored schema |
| Migrations | **`goose`** (plain SQL) | Versioned, reviewable DDL |
| Auth | JWT access + rotating refresh tokens; **Argon2id** password hashing | Standard, stateless API auth |
| API contract | **OpenAPI** spec → generated **TS client** for the frontend | Replaces shared types lost by not using TS on the backend |
| Frontend | **React** + Vite + TanStack Query + a table/grid lib for the register | SPA register UX similar to GnuCash's ledger |
| Validation | Go: request structs + validator; Frontend: Zod against the generated client | Server is source of truth |
| Testing | Go: `go test` + `testify`; API: `httptest`; DB: testcontainers/Postgres; Frontend: Vitest; e2e: Playwright (later) | Fast unit loop for the engine; real-DB integration tests |
| Build/deploy | **Docker** multi-stage → distroless API image; **docker compose** | Small images, one-command local + prod-like stack |

---

## 4. Backend architecture (layered Go)

```
apps/web (React SPA)                      ← built static assets served by web container
        │  HTTPS / JSON (OpenAPI)
apps/api (Go)
   ├── transport/http ...... chi handlers, auth middleware, request/response DTOs
   ├── app (services) ...... use-cases: create ledger, post transaction, import file
   └── domain (the GnuCash kernel, pure Go, no DB/HTTP imports)
        ├── GncNumeric ...... exact rational money over math/big.Rat
        ├── Account/Tx/Split  entities + invariants
        ├── posting ......... validates balance, orchestrates atomic write
        ├── balance ......... account/period balances, sign by type
        ├── price ........... rate lookup & currency conversion
        └── lot ............. cost basis (phase 3+)
   └── infra ............... pgx/sqlc repositories, GnuCash import/export adapters
PostgreSQL
```

**Design rules:**
- The **`domain` package is pure Go** with no DB or HTTP imports — it's where GnuCash's "logic" lives and where correctness is cheapest to guarantee with `go test`.
- **The posting service is the only write path for transactions.** It (a) loads/constructs splits, (b) asserts the balance invariant in the transaction currency, (c) writes the transaction + all splits inside a single DB transaction (`pgx.Tx`), (d) appends an `audit_log` row. No handler writes splits directly.
- **Concurrency:** DB transactions with an optimistic-lock column (a `versions`-style counter or `updated_at`) so two web users editing the same transaction can't silently clobber each other — a concern desktop GnuCash never had.

---

## 5. API design (REST, versioned `/api/v1`, described by OpenAPI)

Representative endpoints (phase 1–2):
- `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`
- `GET /books`, `POST /books` (create ledger), `GET /books/:id`
- `GET /books/:id/accounts` (tree), `POST /accounts`, `PATCH /accounts/:id`
- `GET /accounts/:id/register` (paginated splits with running balance)
- `POST /transactions` (body = transaction + splits; **server validates balance, rejects 422 if unbalanced**), `PATCH /transactions/:id`, `DELETE /transactions/:id`
- `GET /commodities`, `POST /prices`, `GET /prices?commodity=…`
- `POST /imports/gnucash` (upload), `GET /imports/:id` (status), `GET /books/:id/export/gnucash`
- Reports: `GET /reports/balance-sheet`, `/reports/income-statement`, `/reports/account-balance` with date-range params.

All money fields cross the wire as `{num, denom}` (or a decimal string) — never a JSON float. The OpenAPI spec is the source of truth; the frontend consumes a generated TypeScript client so request/response types stay in sync without sharing a runtime.

---

## 6. GnuCash import / export

This constrains the schema, so it's first-class, not an afterthought.

- **Two GnuCash on-disk formats:**
  1. **SQLite/SQL backend** — tables map 1:1 to our mirrored schema; importer is a table-by-table copy with GUID preservation. Easiest, highest fidelity. Read with pure-Go `modernc.org/sqlite`.
  2. **XML format** (`.gnucash`, often gzipped) — parse with Go's streaming `encoding/xml` into the same entities.
- **Importer** lives in `infra`, runs as an async `import_jobs` task (goroutine + status row), validates every transaction balances on the way in, and reports rejected rows rather than silently dropping them.
- **Exporter** writes our mirrored tables back out to GnuCash SQLite (primary target) and optionally XML. We keep a round-trip test: import a known GnuCash file, export it, re-import, assert account/transaction/split equivalence.
- **`slots` (KVP):** preserve unknown/extra metadata verbatim on import so re-export is lossless even for fields our UI doesn't surface yet.

---

## 7. Frontend (React SPA)

- **Account tree** sidebar (collapsible, with running balances by type).
- **Register view** — the heart of GnuCash: an editable, keyboard-driven split grid per account, with running balance, basic/transaction-journal modes, and inline transfer-account autocomplete.
- **Transaction editor** that enforces balance live (shows the imbalance and the auto-balancing split, like GnuCash).
- **Reports**: balance sheet, income/expense, net worth over time, account pie/bar charts.
- **Import wizard** for GnuCash files (and later OFX/QIF/CSV).
- Business UI (phase 4): customer/vendor lists, invoice/bill editors, A/R & A/P aging.
- Talks to the API through the **generated TypeScript client** + TanStack Query.

---

## 8. Phased roadmap

1. **Phase 0 — Scaffolding** (build first): Go module + frontend workspace, DB, SQL migrations for core tables, auth, `GncNumeric`, health-check, Docker/compose, one end-to-end vertical slice (create book → create accounts → post a balanced 2-split transaction → read register).
2. **Phase 1 — Personal ledger core:** full account tree, register UI, transactions/splits, commodities + currencies, prices, balances, basic reports.
3. **Phase 2 — GnuCash import/export** (SQLite first, then XML), reconciliation, multi-currency + trading accounts.
4. **Phase 3 — Scheduled transactions, budgets, lots/capital-gains, investment accounts.**
5. **Phase 4 — Small-business:** customers/vendors, invoices/bills, A/R & A/P, tax tables, payment terms, business reports.

---

## 9. Scaffolding plan (the concrete first build)

Repository layout:
```
/ (repo root)
  docker-compose.yml             # full stack: db + api + web
  docker-compose.dev.yml         # postgres-only for local dev
  .env.example
  apps/
    api/                         # Go module
      cmd/server/main.go         # entrypoint
      internal/
        domain/                  # GncNumeric, entities, posting, balance (pure Go)
        app/                     # use-case services
        transport/http/          # chi handlers, middleware
        infra/                   # pgx/sqlc repos, gnucash import/export
      db/
        migrations/              # goose SQL migrations
        queries/                 # sqlc .sql sources
      sqlc.yaml
      Dockerfile                 # multi-stage → distroless
    web/                         # React + Vite SPA
      Dockerfile                 # build → static assets (nginx/caddy or served by api)
  openapi/openapi.yaml           # API contract → generates web TS client
```

Concrete first tasks:
1. Init Go module + Vite frontend; set up `sqlc.yaml`, `goose`, linters; `docker-compose.dev.yml` with Postgres.
2. `internal/domain`: implement and unit-test `GncNumeric` over `math/big.Rat` (add/sub/mul, reduction, denom alignment, compare, to/from decimal string) and the **balance invariant** check — the riskiest correctness code, so it gets tests first.
3. SQL migrations for phase-1 core tables (`books`, `commodities`, `accounts`, `transactions`, `splits`, `prices`, `slots`, `versions`) **plus** web tables (`organizations`, `users`, `memberships`, `sessions`, `audit_log`); generate accessors with `sqlc`.
4. Auth (register/login/refresh, Argon2id, JWT middleware).
5. Posting service + `POST /transactions` handler enforcing the balance rule (422 on imbalance) inside a `pgx.Tx`, with an `audit_log` write.
6. Accounts + register endpoints; publish the OpenAPI spec and generate the web TS client.
7. `apps/web`: minimal account tree + register + a "new transaction" form that posts a balanced split pair.
8. Seed script: one demo book with a small chart of accounts and a few balanced transactions.
9. Multi-stage Dockerfiles + full `docker-compose.yml`; verify `docker compose up` brings up the whole stack.

---

## 10. Deployment (Docker & docker-compose)

The app ships as containers; `docker compose up` is the supported way to run it locally and as a prod-like single-host deployment.

- **API image** — multi-stage `Dockerfile`: a `golang` builder stage compiles a static binary, then a **distroless/static** (or `scratch`) final stage copies just the binary and migrations. Result ≈ 15–25 MB, non-root, no shell.
- **Web image** — Vite build stage produces static assets served by a small **nginx/caddy** stage (or, for simplicity, embedded and served by the Go binary so there's one fewer container).
- **`docker-compose.yml`** services:
  - `db` — `postgres:16`, named volume for data, `POSTGRES_*` from `.env`, `healthcheck` (`pg_isready`).
  - `api` — built from `apps/api`, `depends_on: db` (condition: healthy), reads `DATABASE_URL`/`JWT_SECRET` from env, runs migrations on startup (or a one-shot `migrate` service), exposes `:8080`, has its own healthcheck endpoint `/healthz`.
  - `web` — built from `apps/web`, serves the SPA and reverse-proxies `/api` to `api` (or the SPA calls the API directly via configured base URL).
- **`docker-compose.dev.yml`** — Postgres only, so you can run `api`/`web` with hot reload on the host during development.
- **Config** — twelve-factor via environment variables; `.env.example` documents every key (DB DSN, JWT secret, log level, port). No secrets committed.
- **Migrations on deploy** — `goose up` runs before the API serves traffic (init/one-shot service or entrypoint step) so the schema is always current.

---

## 11. Verification

- **Engine unit tests (`go test`):** `GncNumeric` arithmetic identities and rounding; multi-currency split construction; the balance invariant accepts balanced and rejects unbalanced transactions.
- **API/integration tests:** `httptest` handlers + a real Postgres via testcontainers — posting a balanced transaction succeeds; an unbalanced one returns 422; the register endpoint returns correct running balances; auth middleware rejects unauthenticated requests.
- **End-to-end smoke (manual or Playwright):** create book → create Asset:Checking and Expense:Groceries → post a $50 transaction → confirm both account balances and the register update.
- **Import/export round-trip test (phase 2):** import a sample `.gnucash` SQLite file, export it, re-import, and assert account/transaction/split counts and balances match.
- **Container smoke test:** `docker compose up --build` brings up `db` + `api` + `web`; `GET /healthz` returns 200; the seed demo book is reachable in the browser.
- **Local dev run:** `docker compose -f docker-compose.dev.yml up` (Postgres), `goose up`, `go run ./apps/api/cmd/server`, `pnpm --filter web dev`.

---

## Open considerations (flag, not blocking)
- **Frontend type-sharing:** with a Go backend, keep the OpenAPI spec authoritative and regenerate the TS client in CI so drift is caught early.
- **OFX/QIF/CSV bank import** is valuable but deferred to post-phase-2.
- **Online price quotes** (GnuCash uses Finance::Quote) — replace with a pluggable price-fetch service later.
- **Reporting engine** — GnuCash's reports are Scheme/Guile; we reimplement the handful of core reports natively rather than porting Scheme.
