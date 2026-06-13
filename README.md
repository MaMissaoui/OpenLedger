# OpenLedger

Web-based double-entry accounting for **personal finance and small businesses**, modeled on the data model and accounting logic of [GnuCash](https://www.gnucash.org/).

OpenLedger reuses GnuCash's proven double-entry kernel — `Book → Accounts → Transactions → Splits`, plus Commodities/Prices, Lots, Scheduled Transactions, and business objects — while delivering a modern multi-user web experience. It is designed to **import and export GnuCash files**, so the schema mirrors GnuCash's SQL backend closely.

## Core principles

- **True double-entry.** Every transaction's splits balance to exactly zero in its currency; the posting service rejects anything that doesn't.
- **Exact rational money, never floats.** Amounts are stored as integer numerator/denominator pairs (GnuCash-style), wrapped in a `GncNumeric` value object backed by Go's `math/big.Rat`.
- **Multi-currency correct.** Per-split value (transaction currency) and quantity (account commodity), with optional trading accounts for whole-book balance.
- **GnuCash-compatible.** Import/export of GnuCash SQLite (and XML) files is a first-class feature.

## Stack

- **Backend:** Go (stdlib `net/http` 1.22 mux, `pgx`, `goose` migrations), `math/big.Rat` money kernel.
- **Auth:** handled at the proxy layer (Traefik + Authelia forward-auth from the `homelab-auth` repo); the API trusts the `Remote-User`/`Remote-Email` headers and has no `/auth/*` routes or JWT logic.
- **Frontend:** React + Vite + TanStack Query, consuming a TypeScript client generated from the OpenAPI contract.
- **Database:** PostgreSQL, with a schema that mirrors GnuCash's SQL backend.
- **Deployment:** Docker multi-stage builds (distroless API image) orchestrated with `docker compose`.

## Running locally

Targets are driven from the [`Makefile`](Makefile):

```bash
make dev    # Postgres + an nginx shim that injects fake auth headers (host-side dev)
make run    # run the API on the host (port 8090)
make seed   # populate a migrated DB with a demo book, chart of accounts, and transactions
make up     # full stack via docker compose (requires the homelab-auth stack running)
```

For `make dev`, add `127.0.0.1 openledger.localhost` to `/etc/hosts`. `make up` requires the
`homelab-auth` repo (Traefik + Authelia + lldap) to be running first — it provides the `proxy`
network the stack joins.

## Status

Phase 1 (personal ledger core) in progress. Working today: book/account creation, a chart of
accounts with per-account and subtree roll-up balances, balanced transaction posting (422 on
imbalance), the account register, commodities, and price quotes. GnuCash import/export and
business objects are later phases. See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the
full architecture and phased roadmap.
