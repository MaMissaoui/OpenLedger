---
name: add-endpoint
description: Add a new HTTP API endpoint to apps/api the OpenLedger way — respecting the layered architecture, the money/GncNumeric rules, the posting-service write path, and keeping openapi/openapi.yaml in sync. Use when adding or changing any API route.
---

Add an endpoint to `apps/api` following the project's layering. Work from the inside out.

## 1. Domain (`internal/domain`) — only if new accounting logic is needed
- Pure Go: no DB or HTTP imports.
- Money is `GncNumeric`, never `float64`. New invariants get a unit test here first (`go test ./internal/domain`).

## 2. App service (`internal/app`)
- Use-cases orchestrate domain + repository ports. Define repository needs as an interface here; the pgx implementation lives in `internal/infra/pg`.
- **All transaction writes go through `PostingService`** — it enforces the balance invariant and writes tx + splits + `audit_log` in one `pgx.Tx`. Do not add a second write path for splits.

## 3. Infra (`internal/infra/pg/repository.go`)
- Hand-written pgx (sqlc is configured but not generated — don't assume a generated `db` package exists).
- Keep money columns as `*_num`/`*_denom`; convert with `GncNumeric.AtDenom(fraction)` so non-exact amounts are rejected, not rounded.

## 4. Transport (`internal/transport/httpapi`)
- Register the route in `server.go`'s `Routes()` using the stdlib mux (`METHOD /path/{id}`), not chi.
- DTOs carry money as `{num, denom}` (`numericDTO`) — never a JSON float.
- Map domain errors to status codes (e.g. `domain.ErrUnbalanced` → 422, not-found → 404). Add a handler test using the in-memory fake repo pattern in the existing `*_test.go` files.

## 5. Contract — required, same change
- Update `openapi/openapi.yaml`: path, params, and request/response schemas (reuse the `Numeric` schema for money). It is the source of truth for the generated web TS client.

## 6. Verify
- `make fmt && make vet && make test`.
