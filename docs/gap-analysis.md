# OpenLedger — Gap Analysis

_Generated 2026-06-20. Audit of the documented roadmap + ARCHITECTURE "Open
considerations" against the code. Snapshot — re-audit before relying on it._

## Roadmap status

Phases 0–3 are **essentially complete**, and phase 4 is **mostly built**.
Confirmed present and tested (so **not** gaps):

- GnuCash import/export in **both SQLite and XML** (gzip + roundtrip tests).
- Reconciliation, trading accounts, lots/capital-gains.
- Scheduled transactions, budgets.
- Securities/portfolio, capital-gains report.
- Full A/R **and** A/P: customer invoices + vendor bills, payments, aging reports.
- Web views for nearly every area.

The real gaps are narrow and specific.

## A. Phase-4 business objects — schema columns exist, backing tables don't

The `customers`/`vendors`/`invoices` tables already carry `terms_guid` and
`taxable`/tax columns, but the objects they point at were never built.

| Gap | Evidence | Effort |
|---|---|---|
| ~~**Bill/payment terms** (`billterms`)~~ — **Done.** `billterms` table, domain, CRUD endpoints + web UI; invoice due dates derive from terms | **M** |
| ~~**Tax tables** (`taxtables` + `taxtable_entries`)~~ — **Done.** Tax-rate table + entries, tax applied when posting invoices, web UI | **M–L** |
| **Employees** + expense vouchers | No `employees` table/handler | **M** |
| **Jobs** (group invoices/bills under a job) | No `jobs` table/handler | **S–M** |

## B. Deferred bank-import formats

Import only accepts GnuCash SQLite/XML. The deferred formats are absent:

| Gap | Effort |
|---|---|
| **OFX** import | M |
| **QIF** import | M |
| **CSV** import (column-mapping wizard — UI heavy) | L |

## C. Online price quotes

`PriceService` stores only **manually-entered** quotes — no fetch. ARCHITECTURE
calls for a "pluggable price-fetch service" to replace GnuCash's Finance::Quote.
Gap: a quote-provider interface + one provider + optional scheduled refresh.
**Effort: M**

## D. Web UI gaps (backend ahead of frontend) — DONE

- ~~**Prices/commodities management view**~~ — **Done.** A "Commodities" nav
  view (`CommoditiesView.tsx`) lists commodities, creates them (currency or
  security), and shows/adds per-commodity price history.
- ~~**Import UI**~~ — **Done.** An "Import GnuCash" sidebar button
  (`ImportDialog.tsx`) uploads a SQLite/XML file to `POST /imports/gnucash`
  via the new `api.importGnuCash` and reports the imported object counts.

## Recommended sequencing

1. ~~**Bill terms** (A)~~ — **Done.** Unblocked correct invoice due dates.
2. ~~**Tax tables** (A)~~ — **Done.** Completed invoicing correctness.
3. ~~**Prices/commodities UI + import UI** (D)~~ — **Done.**
4. **Online price quotes** (C) — self-contained.
5. **OFX/QIF import** (B) — then CSV last (heaviest).
6. **Employees & jobs** (A) — lowest value for a personal/small-business focus; defer unless chasing full GnuCash parity.
