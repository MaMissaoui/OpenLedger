# OpenLedger — Architecture & Scaffolding Plan

## Context

OpenLedger is a web-based accounting application for **personal finance and small businesses**, reusing the proven **data model and accounting logic of GnuCash** (the mature desktop double-entry app). The goal is to keep GnuCash's correctness guarantees (true double-entry, exact rational arithmetic, multi-currency, lots) while delivering a modern multi-user web experience.

Key decisions:
- **Scope:** Personal **+ small business** (customers/vendors, invoices, bills, A/R & A/P, tax tables, payment terms) — built in phases starting with the personal-finance ledger core.
- **Stack:** **TypeScript / Node full-stack** (NestJS API + React frontend + PostgreSQL).
- **GnuCash compatibility:** Must **import and export GnuCash files**, so our schema mirrors GnuCash's SQL backend closely.

**Is it possible?** Yes. GnuCash's engine is a clean, well-specified double-entry kernel (`Book → Accounts → Transactions → Splits`, plus `Commodities`/`Prices`, `Lots`, `Scheduled Transactions`, and business objects). It already ships a relational SQL backend (SQLite/MySQL/PostgreSQL) whose table layout is public and stable, so the data model ports directly. The genuinely new work is the web layer: authentication, multi-user concurrency, an HTTP API, and a browser UI — none of which the single-user desktop app needed.

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
2. **Exact arithmetic, never floats.** GnuCash stores every monetary amount as a **rational number** — an integer numerator over an integer denominator (`value_num`/`value_denom`, `quantity_num`/`quantity_denom`). We mirror this. All money math uses integer/bignum or arbitrary-precision decimal; floating point is banned in the money path.
3. **Two amounts per split:** `value` is in the transaction currency; `quantity` is in the account's own commodity. For same-currency accounts they are equal. For foreign-currency / stock accounts they differ, and the ratio is an implied exchange rate or share price.
4. **Account-type sign semantics:** debit/credit normality is derived from account type. Reports and the UI must respect each type's natural balance sign.
5. **Multi-currency correctness:** when a transaction spans currencies it must still balance to zero in its transaction currency; GnuCash's optional **Trading Accounts** mechanism keeps the *whole book* balanced per-currency. We support trading accounts to remain export-compatible.
6. **Lots for cost basis:** sales of shares/securities are linked via lots to their purchase splits so realized gains compute correctly.

---

## 2. Data model & database schema

**Strategy:** mirror GnuCash's SQL backend table structure so import/export is a near-direct mapping, then layer our own tables (users, orgs, sessions, audit) *around* it rather than altering the GnuCash core tables.

- **Database:** PostgreSQL (GnuCash itself supports a Postgres backend, which keeps us close to its expectations).
- **ORM / migrations:** **Drizzle ORM** (typed SQL-first, predictable generated SQL — preferable to heavier abstractions when correctness of the emitted SQL matters). Alternative: Prisma if the team prefers its DX.
- **Money columns:** store GnuCash-style `*_num BIGINT` + `*_denom BIGINT` pairs in the GnuCash-mirroring tables (required for fidelity). Internally, wrap these in a `Money`/`GncNumeric` value object (numerator/denominator with GCD reduction) used everywhere in the domain layer. Never expose a JS `number` for money.
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
| Language | **TypeScript** (strict) end to end | One language, shared types between API and UI |
| API framework | **NestJS** | Structured DI/module system fits a layered domain (engine vs. controllers); good for the posting-service boundary |
| DB | **PostgreSQL** | GnuCash-compatible backend; transactional integrity for balanced posting |
| Data access | **Drizzle ORM** + SQL migrations | Predictable SQL, typed schema |
| Money | Custom `GncNumeric` value object (BigInt-based) | Exact rational arithmetic, GnuCash fidelity |
| Auth | JWT access + rotating refresh tokens; Argon2 password hashing | Standard, stateless API auth |
| Frontend | **React** + Vite + TanStack Query + a table/grid lib for the register | SPA register UX similar to GnuCash's ledger |
| Validation | **Zod** schemas shared client/server | Single source of truth for request shapes |
| Testing | **Vitest** (unit) + **Supertest** (API) + Playwright (e2e later) | Fast unit loop for the engine; HTTP contract tests |
| Monorepo | pnpm workspaces (`packages/engine`, `apps/api`, `apps/web`, `packages/shared`) | Share the engine + types across api and web |

---

## 4. Backend architecture (layered)

```
apps/web (React SPA)
        │  HTTPS / JSON
apps/api (NestJS)
   ├── controllers ......... HTTP, auth guards, request/response DTOs (Zod)
   ├── application services . use-cases: create ledger, post transaction, import file
   └── domain (packages/engine) ── the GnuCash kernel, framework-free
        ├── GncNumeric ........ exact rational money
        ├── Account/Tx/Split .. entities + invariants
        ├── PostingService .... validates balance, writes atomically
        ├── BalanceService .... account/period balances, sign by type
        ├── PriceService ...... rate lookup & currency conversion
        └── LotService ........ cost basis (phase 3+)
   └── infrastructure ....... Drizzle repositories, GnuCash import/export adapters
PostgreSQL
```

**Design rules:**
- The **engine package is framework-free** and has no DB or HTTP imports — it's pure domain logic, exhaustively unit-tested. This is where GnuCash's "logic" lives and where correctness is cheapest to guarantee.
- **PostingService is the only write path for transactions.** It (a) loads/constructs splits, (b) asserts the balance invariant in the transaction currency, (c) writes the transaction + all splits inside a single DB transaction, (d) appends an `audit_log` row. No controller writes splits directly.
- **Concurrency:** use DB transactions with row versioning (a `versions`-style or `updated_at`/optimistic-lock column) so two web users editing the same transaction can't silently clobber each other — a concern desktop GnuCash never had.

---

## 5. API design (REST, versioned `/api/v1`)

Representative endpoints (phase 1–2):
- `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`
- `GET /books`, `POST /books` (create ledger), `GET /books/:id`
- `GET /books/:id/accounts` (tree), `POST /accounts`, `PATCH /accounts/:id`
- `GET /accounts/:id/register` (paginated splits with running balance)
- `POST /transactions` (body = transaction + splits; **server validates balance, rejects 422 if unbalanced**), `PATCH /transactions/:id`, `DELETE /transactions/:id`
- `GET /commodities`, `POST /prices`, `GET /prices?commodity=…`
- `POST /imports/gnucash` (upload), `GET /imports/:id` (status), `GET /books/:id/export/gnucash`
- Reports: `GET /reports/balance-sheet`, `/reports/income-statement`, `/reports/account-balance` with date-range params.

All money fields cross the wire as `{num, denom}` (or a decimal string) — never a JSON float.

---

## 6. GnuCash import / export

This constrains the schema, so it's first-class, not an afterthought.

- **Two GnuCash on-disk formats:**
  1. **SQLite/SQL backend** — tables map 1:1 to our mirrored schema; importer is a table-by-table copy with GUID preservation. Easiest, highest fidelity.
  2. **XML format** (`.gnucash`, often gzipped) — parse with a streaming XML reader into the same entities.
- **Importer** lives in `infrastructure`, runs as an async `import_jobs` task, validates every transaction balances on the way in, and reports rejected rows rather than silently dropping them.
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

---

## 8. Phased roadmap

1. **Phase 0 — Scaffolding** (build first): monorepo, DB, migrations for core tables, auth, `GncNumeric`, health-check, one end-to-end vertical slice (create book → create accounts → post a balanced 2-split transaction → read register).
2. **Phase 1 — Personal ledger core:** full account tree, register UI, transactions/splits, commodities + currencies, prices, balances, basic reports.
3. **Phase 2 — GnuCash import/export** (SQLite first, then XML), reconciliation, multi-currency + trading accounts.
4. **Phase 3 — Scheduled transactions, budgets, lots/capital-gains, investment accounts.**
5. **Phase 4 — Small-business:** customers/vendors, invoices/bills, A/R & A/P, tax tables, payment terms, business reports.

---

## 9. Scaffolding plan (the concrete first build)

Repository layout (pnpm monorepo):
```
/ (repo root)
  package.json, pnpm-workspace.yaml, tsconfig.base.json
  docker-compose.yml            # postgres for local dev
  packages/
    shared/                     # Zod DTOs + shared types
    engine/                     # framework-free domain: GncNumeric, entities, PostingService, BalanceService
  apps/
    api/                        # NestJS: controllers, app services, Drizzle repos, migrations
    web/                        # React + Vite SPA
```

Concrete first tasks:
1. Init pnpm workspace, base tsconfig, ESLint/Prettier, `docker-compose.yml` with Postgres.
2. `packages/engine`: implement and unit-test `GncNumeric` (add/sub/mul, GCD reduction, denom alignment, compare, to/from decimal string) and the **balance invariant** check — this is the riskiest correctness code, so it gets tests first.
3. `apps/api`: Drizzle schema + migration for phase-1 core tables (`books`, `commodities`, `accounts`, `transactions`, `splits`, `prices`, `slots`, `versions`) **plus** web tables (`organizations`, `users`, `memberships`, `sessions`, `audit_log`).
4. Auth module (register/login/refresh, Argon2, JWT guard).
5. `PostingService` + `transactions` controller enforcing the balance rule (422 on imbalance) inside a DB transaction, with an `audit_log` write.
6. Accounts + register endpoints.
7. `apps/web`: minimal account tree + register + a "new transaction" form that posts a balanced split pair.
8. Seed script: one demo book with a small chart of accounts and a few balanced transactions.

---

## 10. Verification

- **Engine unit tests (Vitest):** `GncNumeric` arithmetic identities and rounding; multi-currency split construction; the balance invariant accepts balanced and rejects unbalanced transactions.
- **API contract tests (Supertest):** posting a balanced transaction succeeds; an unbalanced one returns 422; register endpoint returns correct running balances; auth guards reject unauthenticated requests.
- **End-to-end smoke (manual or Playwright):** create book → create Asset:Checking and Expense:Groceries → post a $50 transaction → confirm both account balances and the register update.
- **Import/export round-trip test (phase 2):** import a sample `.gnucash` SQLite file, export it, re-import, and assert account/transaction/split counts and balances match.
- **Local run:** `docker compose up` (Postgres), `pnpm --filter api migrate && pnpm --filter api dev`, `pnpm --filter web dev`, then exercise the vertical slice in the browser.

---

## Open considerations (flag, not blocking)
- **OFX/QIF/CSV bank import** is valuable but deferred to post-phase-2.
- **Online price quotes** (GnuCash uses Finance::Quote) — replace with a pluggable price-fetch service later.
- **Reporting engine** — GnuCash's reports are Scheme/Guile; we reimplement the handful of core reports natively rather than porting Scheme.
