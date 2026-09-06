# P1-2 resume notes (delete before the PR)

Every number already measured, so a resumed session re-runs nothing it does not have to.

## Stack under measurement

The shared containers were left as they were found. The branch runs as host processes
against the same Postgres and the same served run:

- ml-service: `uvicorn app.main:app --port 8001`, `MODELS_DIR=<repo>/output/ml-service`
- go-app: `go run ./cmd/api` on `:8081`, `ML_SERVICE_URL=http://127.0.0.1:8001`,
  `RUN_MIGRATIONS_AT_STARTUP=0`
- frontend: `VITE_API_URL=http://localhost:8081 npm run dev -- --port 5199`
- served run `20260906T083819Z-36689f80`, ratings through 2026-09-02 (the container's run)

Probe: `scripts/probes/p1_2_play_mode.py`, run with `PYTHONPATH=ml-service`.

## Measured, 2026-09-06

**Path parity** (pinning the searched eleven reproduces Optimise's own numbers): gap
**0.0000000000** in all four formats, headline source `display` in all four.

**Swap monotonicity, dominating upgrades (the gate):** 0 falls out of 2 (T20I), 2 (ODI),
8 (T20), 8 (TEST). TEST and the TEST diagnostic used the all-time pool: the recency window
offered no unambiguous upgrade.

**Selector-rank swaps (diagnostic, not the gate):** falls 0/8 T20I, 1/8 ODI (worst
-0.0326), 4/8 T20 (worst -0.0171), 6/8 TEST (worst -0.0805). A player the rating
composite ranks higher is not better on every axis the display model reads, so this is not
B-7's guarantee failing. Recorded as B-8 in docs/BUG_BACKLOG.md.

**Latency, 30 timed re-scores per format, client to client (ms):**

| format | median | p95 | min | predict-win | forecast call | draws |
|---|---:|---:|---:|---:|---:|---:|
| T20I | 370.5 | 738.6 | 345.5 | 8.2 | 321.7 (/simulate) | 2000 |
| ODI | 388.0 | 532.8 | 372.8 | 7.6 | 334.0 (/simulate) | 2000 |
| T20 | 399.0 | 516.4 | 378.0 | 5.9 | 358.1 (/simulate) | 2000 |
| TEST | 354.9 | 471.0 | 340.3 | 8.1 | 300.2 (/performance/predict) | — |

**Where the time goes** (one T20I `/simulate`, 20 repeats in-process):

| step | ms |
|---|---:|
| assemble the fixture's rows | 0.8 |
| performance model -> per-player forecasts | 295.6 |
| `simulate_match` at 2000 draws | **9.4** |
| summarize the draws | 5.5 |
| `display_probability` | 6.8 |

The roadmap's ~10 ms is the draw loop alone (9.4 ms here, 9.5-9.8 in the harness's
`ms_per_fixture_at_default_samples`), and excludes the forecast that feeds it.

**Environment caveat:** the same `/simulate` payload takes 317.2 ms median on the host
process and **145.4 ms** against the shared container serving the same run, so a
containerised re-score would land near 200 ms. Either way the ~10 ms claim is out by a
factor of 20-40.

## Done

- go-app pinned path, refusals, constraint report; ml-service app-layer constraint check;
  frontend Play board, chips, delta; all tests (commit `e351420f`).
- Frontend coverage ratcheted to 80/80/78/71. go-app 76.8 % and ml-service 93.62 % both
  round down to their existing thresholds, so those did not move.

## Left to do

- Browser walk-through of the Play board on the dev stack.
- Docs: § 1 and § 3 latency claim, § 3.2 P1-2 marked shipped, § 10 P1-2 row, B-8.
- `make check-all`; verify cricket_data still reads 22,818 / 11,539,808 / 13,662.
