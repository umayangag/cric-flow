# Plan ID 1.4.1.4.2.a — Export‑dataset Phase 4 (cont.): Implement exportqueries for Batting

Active path: 1 → 4 → 4.1 → 4.1.4 → 4.1.4.2 → 4.1.4.2.a
Parent: 1 → 4 → 4.1 → 4.1.4 → 4.1.4.2
Branch: feat/exportdataset-phase4

---

## Goal
Implement the read‑only batting query helpers in `internal/db/exportqueries` by extracting SQL from `cmd/export-dataset/main.go`, returning `[][]string` suitable for CSV writing by `internal/services/exportdataset`.

---

## Scope
- Implement:
  - `BattingUnifiedRows(ctx)`
  - `BattingLegacyRows(ctx)`
  - `BattingInferenceRows(ctx, format)`
- Use existing DB connect helpers (pgx via `internal/db` package).
- Map query results to `[][]string` including header row as the first element (to match current CSV outputs).
- Robust error handling; do not log in this layer.
- Keep functions small and single‑purpose.

Out of scope:
- Bowling queries (handled in a follow‑up sub‑plan 4.1.4.2.b).
- Changing CSV schemas or behavior.

---

## Files to Modify
- `go-app/internal/db/exportqueries/batting.go`

## Implementation Notes
- Copy SQL verbatim from:
  - `exportBattingUnified`
  - `exportBattingLegacy` (wrapper over `exportBatting`)
  - `exportBattingFormatInference`
- Use `db.Connect(ctx)` to obtain a connection.
- Use `pgx` row iteration to collect `[][]string` in the same column order as CSV writing in `main.go`.
- Include the same CSV header row that `main.go` emits.

---

## Acceptance Criteria
- `exportqueries/batting.go` implemented; compiles.
- Services (`internal/services/exportdataset`) continue to pass tests.
- No behavior change in field order or formatting.

---

## Verification Commands
```bash
cd go-app
make test
make coverage && make coverage-func
# Smoke (after wiring step completes):
make export-dataset OUT=../output/go-app
```

---

## Steps
1. Extract SQL and implement `BattingUnifiedRows` → return `[][]string` with headers.
2. Implement `BattingLegacyRows` using the legacy (combined) query.
3. Implement `BattingInferenceRows(format)` using the per‑format inference query.
4. Build and run unit tests; ensure compilation of adapter and services.
5. Proceed to sub‑plan 4.1.4.2.b for bowling.
