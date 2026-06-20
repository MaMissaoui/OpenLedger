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
| **Bill/payment terms** (`billterms`) | `terms_guid` columns exist everywhere; no `billterms` table, domain, or endpoint — due dates set manually | **M** |
| **Tax tables** (`taxtables` + `taxtable_entries`) | `entries.taxable BOOLEAN` exists but no tax-rate table or tax calc on totals | **M–L** |
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

## D. Web UI gaps (backend ahead of frontend)

- **Prices/commodities management view** — `createPrice`/`listPrices` exist in
  `api.ts` but no component renders them; prices are only read inside
  Portfolio/Invoice. **S**
- **Import UI** — an *export* URL helper exists in `api.ts` but there's **no
  import call and no upload surface**; GnuCash import is API-only. **S–M**

## Recommended sequencing

1. **Bill terms** (A) — small, unblocks correct invoice due dates, referenced everywhere already.
2. **Tax tables** (A) — completes invoicing correctness.
3. **Prices/commodities UI + import UI** (D) — cheap, high visibility, no schema work.
4. **Online price quotes** (C) — self-contained.
5. **OFX/QIF import** (B) — then CSV last (heaviest).
6. **Employees & jobs** (A) — lowest value for a personal/small-business focus; defer unless chasing full GnuCash parity.
