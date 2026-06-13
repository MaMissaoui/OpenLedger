# OpenLedger

Web-based double-entry accounting for **personal finance and small businesses**, modeled on the data model and accounting logic of [GnuCash](https://www.gnucash.org/).

OpenLedger reuses GnuCash's proven double-entry kernel — `Book → Accounts → Transactions → Splits`, plus Commodities/Prices, Lots, Scheduled Transactions, and business objects — while delivering a modern multi-user web experience. It is designed to **import and export GnuCash files**, so the schema mirrors GnuCash's SQL backend closely.

## Core principles

- **True double-entry.** Every transaction's splits balance to exactly zero in its currency; the posting service rejects anything that doesn't.
- **Exact rational money, never floats.** Amounts are stored as integer numerator/denominator pairs (GnuCash-style), wrapped in a `GncNumeric` value object.
- **Multi-currency correct.** Per-split value (transaction currency) and quantity (account commodity), with optional trading accounts for whole-book balance.
- **GnuCash-compatible.** Import/export of GnuCash SQLite (and XML) files is a first-class feature.

## Planned stack

TypeScript end to end — NestJS API, React + Vite frontend, PostgreSQL, Drizzle ORM, in a pnpm monorepo (`packages/engine`, `packages/shared`, `apps/api`, `apps/web`).

## Status

Pre-implementation. See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the full architecture and phased roadmap.
