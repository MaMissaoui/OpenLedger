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
| ~~**Employees**~~ — **Done.** `employees` table, domain, CRUD endpoints + web UI (entity only; expense vouchers still deferred) | **M** |
| ~~**Jobs** (group invoices/bills under a job)~~ — **Done.** `jobs` table, domain, CRUD endpoints + web UI (customer/vendor owner) | **S–M** |

## B. Deferred bank-import formats — DONE

OFX, QIF and CSV all import into an existing account (`POST /accounts/{id}/import-bank`,
offset to Imbalance-CUR, deduped by import ref).

| Gap | Effort |
|---|---|
| ~~**OFX** import~~ — **Done.** Tolerant SGML/XML parser; FITID-based dedup | M |
| ~~**QIF** import~~ — **Done.** Content-hash dedup (no per-line id in QIF) | M |
| ~~**CSV** import~~ — **Done.** Column-mapping wizard (server preview + mapping UI), `$€£¥`/parens/sign parsing | L |

## C. Online price quotes — DONE

~~`PriceService` stores only **manually-entered** quotes — no fetch.~~ **Done.**
A pluggable `app.QuoteProvider` + a Frankfurter (ECB FX) provider back
`POST /api/v1/prices/fetch`, recording fetched rates as exact-rational prices
via the normal price write path; a "Fetch online" button in the Commodities view
drives it. Optional scheduled auto-refresh is still deferred. **Effort: M**

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
4. ~~**Online price quotes** (C)~~ — **Done.** Frankfurter FX provider behind `POST /prices/fetch`.
5. ~~**OFX/QIF/CSV import** (B)~~ — **Done.** All three post into an account, offset to Imbalance; CSV via a column-mapping wizard.
6. ~~**Employees & jobs** (A)~~ — **Done.** Entity CRUD (table + domain + endpoints + web) for both; expense vouchers and job-attachment-to-invoice still deferred.

**Backlog complete** — every item in sections A–D is shipped or explicitly deferred (employee expense vouchers, scheduled price auto-refresh, job→invoice linking). Re-audit before picking up new roadmap work.

## E. Next roadmap — new requests (not yet started)

Fresh feature requests beyond the original gap audit. None are built yet.

| Gap | Notes | Effort |
|---|---|---|
| **Multi-language support (EN / FR / DE)** | i18n for the web SPA — extract UI strings into a message catalog, add a language switcher, and translate EN → FR + DE. Decide on a library (e.g. `react-i18next`/`lingui`), a default-locale fallback, and whether to persist the user's choice. Server-side text (error messages) is out of scope unless surfaced verbatim in the UI. | M |
| **Settings section — admin & system setup screens** | A dedicated Settings area: admin screens (org/book membership & role management, surfacing the existing `AuthzService` RBAC) plus system setup (default currency, locale, date/number formats, book preferences). Backend likely needs settings/preferences storage + endpoints; web needs a Settings nav section. | M–L |
| **Realistic SKR04 demo ledger (with account numbers)** | A seedable demo book using the German **SKR04** chart of accounts, including the SKR04 **account numbers** (the `accounts` schema needs a `code`/number field if not already present) and a realistic set of balanced transactions. Extends the existing `make seed` demo. Validates multi-language (DE) and account-number display end-to-end. | M |

These are **not sequenced yet** — confirm priority before starting. SKR04 + account numbers pairs naturally with the DE locale work; the Settings section is the natural home for a locale/default-currency switcher.
