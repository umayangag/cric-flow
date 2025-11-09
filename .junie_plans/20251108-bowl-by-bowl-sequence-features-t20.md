# Plan ID: 1 — Bowl-by-Bowl Sequence Features (All formats; T20-first execution)

Status: Ready for implementation
Owner: Junie
Date: 2025-11-08
Active path: 1
Parent: —

---

## 1. Objective
Engineer bowl-by-bowl, sequence-aware features from Cricsheet deliveries for all formats (T20, ODI, Test). Execute implementation T20-first, then extend to ODI and Test within the same plan. Keep exports backward compatible (no dataset version bump) and integrate features into the ML pipeline.

---

## 2. Scope & Assumptions
- Formats in scope: T20, ODI, Test. Implementation order: T20 → ODI → Test.
- Data Source: Cricsheet v1.1 (already parsed by repo types).
- DB: PostgreSQL — migrations under `go-app/migrations/`.
- Existing `fielding_event` artifacts remain as-is.
- Not in production — exporter changes can be additive without a dataset version bump.

---

## 3. Evidence of Feasibility (Read-only Audit)
- `go-app/internal/cricsheet/cricsheet.go` defines `Innings → Over → Delivery` with `batter`, `non_striker`, `bowler`, `runs`, `extras`, `wickets`.
- `go-app/internal/cricsheet/ingest.go` iterates every delivery, computes legal-ball logic, tracks `overNo`, and already persists `fielding_event` keyed by `(match_id, innings, over, ball)`.

---

## 4. Deliverables
- Normalized `ball_event` table populated during imports/backfill (all formats).
- Derived sequence tables:
  - `batting_transition_features` (B after A by phase/format)
  - `bowling_sequence_features` (B after A by phase/format)
- Precompute job to populate derived tables incrementally for all formats.
- Exporter joins to include new features.
- ML readers that transform these features appropriately and baseline experiments demonstrating impact.

---

## 5. Data Model (Approved)

### 5.1 `ball_event`
One row per delivery (legal/illegal) with context.

```sql
CREATE TABLE IF NOT EXISTS ball_event (
  match_id       BIGINT NOT NULL,
  innings        SMALLINT NOT NULL,
  over           SMALLINT NOT NULL,
  ball           SMALLINT NOT NULL,
  ball_seq       INTEGER NOT NULL,         -- legal-delivery index within innings
  is_legal       BOOLEAN NOT NULL,
  phase          VARCHAR(16) NOT NULL,     -- powerplay, middle, death, all
  striker_id     BIGINT,
  non_striker_id BIGINT,
  bowler_id      BIGINT,
  runs_batter    SMALLINT NOT NULL DEFAULT 0,
  runs_extras    SMALLINT NOT NULL DEFAULT 0,
  runs_total     SMALLINT NOT NULL DEFAULT 0,
  extras_kind    VARCHAR(16),              -- wide, no_ball, bye, leg_bye, penalty
  wicket_kind    VARCHAR(24),              -- lbw, bowled, caught, run_out, stumped, etc.
  player_out_id  BIGINT,
  fielder_ids    BIGINT[],                 -- optional copy; source of truth is fielding_event
  PRIMARY KEY (match_id, innings, over, ball)
);

CREATE INDEX IF NOT EXISTS idx_ball_event_match_innings_seq
  ON ball_event (match_id, innings, ball_seq);
CREATE INDEX IF NOT EXISTS idx_ball_event_bowler_seq
  ON ball_event (bowler_id, match_id, innings, ball_seq) WHERE bowler_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ball_event_striker_seq
  ON ball_event (striker_id, match_id, innings, ball_seq) WHERE striker_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ball_event_phase
  ON ball_event (phase);
```

Notes: `ball_seq` advances only on legal balls. Illegal deliveries keep `is_legal=false`; ordering uses `(innings, over, ball)` and `ball_seq` for windowing.

### 5.2 `batting_transition_features`
Performance of batter B after predecessor A by phase.

```sql
CREATE TABLE IF NOT EXISTS batting_transition_features (
  as_of_date     DATE NOT NULL,
  format_id      SMALLINT NOT NULL,
  scope          VARCHAR(16) NOT NULL DEFAULT 'overall',
  scope_id       BIGINT,
  prev_batter_id BIGINT NOT NULL,
  batter_id      BIGINT NOT NULL,
  phase          VARCHAR(16) NOT NULL,
  balls          INTEGER NOT NULL,
  runs           INTEGER NOT NULL,
  dismissals     INTEGER NOT NULL,
  fours          INTEGER NOT NULL,
  sixes          INTEGER NOT NULL,
  strike_rate    REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN runs::float*100/balls ELSE 0 END) STORED,
  out_rate       REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN dismissals::float/balls ELSE 0 END) STORED,
  PRIMARY KEY (as_of_date, format_id, scope, COALESCE(scope_id,0), prev_batter_id, batter_id, phase)
);

CREATE INDEX IF NOT EXISTS idx_bat_trans_latest
  ON batting_transition_features (batter_id, format_id, as_of_date DESC)
  INCLUDE (prev_batter_id, phase, balls, runs, strike_rate)
  WHERE scope='overall' AND scope_id IS NULL;
```

### 5.3 `bowling_sequence_features`
Effect of bowler B’s over when following A.

```sql
CREATE TABLE IF NOT EXISTS bowling_sequence_features (
  as_of_date     DATE NOT NULL,
  format_id      SMALLINT NOT NULL,
  scope          VARCHAR(16) NOT NULL DEFAULT 'overall',
  scope_id       BIGINT,
  prev_bowler_id BIGINT NOT NULL,
  bowler_id      BIGINT NOT NULL,
  phase          VARCHAR(16) NOT NULL,
  overs_pairs    INTEGER NOT NULL,
  balls          INTEGER NOT NULL,
  runs           INTEGER NOT NULL,
  wickets        INTEGER NOT NULL,
  dot_balls      INTEGER NOT NULL,
  wicket_rate    REAL GENERATED ALWAYS AS (CASE WHEN balls>0 THEN wickets::float/balls ELSE 0 END) STORED,
  econ           REAL,
  PRIMARY KEY (as_of_date, format_id, scope, COALESCE(scope_id,0), prev_bowler_id, bowler_id, phase)
);

CREATE INDEX IF NOT EXISTS idx_bowl_seq_latest
  ON bowling_sequence_features (bowler_id, format_id, as_of_date DESC)
  INCLUDE (prev_bowler_id, phase, balls, wickets, wicket_rate)
  WHERE scope='overall' AND scope_id IS NULL;
```

---

## 6. Phase Rules by Format (used in importer + precompute)
Implement a single `PhaseFor(format_id, ball_seq, innings_length)` helper with format-specific strategies.

- T20 (BallsPerOver=6 assumed):
  - Powerplay: first 36 legal balls (overs 1–6)
  - Death: last 30 legal balls (overs 16–20) — clamp to innings length
  - Middle: all legal balls between
- ODI (BallsPerOver=6):
  - PP1: first 60 legal balls (overs 1–10)
  - Death: last 60 legal balls (overs 41–50)
  - Middle: between PP1 and Death
- Test:
  - Phase: `all` (single bucket) to avoid arbitrary boundaries; sequence features computed without phase segmentation. We can later add optional `opening/middle/tail` if evidence suggests utility.
- Super over (if present in data): treated as a separate innings; same T20 phase logic but practically very short (likely `powerplay` classification for all deliveries).

Unit tests will cover boundaries and clamping behavior.

---

## 7. ETL Changes
- Add `InsertBallEvent` method and upsert logic in DB layer.
- Update Cricsheet importer to emit `ball_event` for each delivery across all formats:
  - Determine `is_legal` via wide/no_ball logic.
  - Maintain `ball_seq` per innings for legal deliveries.
  - Derive `phase` using `PhaseFor(format_id, ball_seq, innings_length)`.
  - Resolve player IDs via existing helpers (GetOrCreateByName).
- Add `cmd/backfill-ball-events` to populate `ball_event` for existing matches across all formats.
- Add `cmd/precompute-sequence-features` to compute and upsert into sequence tables (all formats):
  - Batting transitions: predecessor at innings start and after dismissals; aggregate next-N legal balls or until next wicket, partitioned by `phase` (except Test where `phase='all'`).
  - Bowling sequences: pair each over by B with the immediately preceding over by A for the same side; aggregate outcomes in B’s over with `phase`.

---

## 8. Exporter Integration (no dataset v2)
- Extend exporter SQL to join latest-as-of rows from `batting_transition_features` and `bowling_sequence_features` for each player at match time `(as_of_date <= match_date)` and `format_id`.
- Initial added columns (narrow set):
  - Batting: `bat_trans_prev_id`, `bat_trans_phase`, `bat_trans_balls`, `bat_trans_sr`, `bat_trans_out_rate`.
  - Bowling: `bowl_seq_prev_id`, `bowl_seq_phase`, `bowl_seq_balls`, `bowl_seq_wkt_rate`, `bowl_seq_econ`.
- Keep existing columns untouched.

---

## 9. ML Feature Transformation (All formats)
- Readers (ml-service):
  - Parse the added exporter columns.
  - Encodings:
    - `prev_batter_id`, `prev_bowler_id`: keep as numeric ids only when paired with their aggregated stats. Use Bayesian smoothing on rates: `rate_smooth = (sum + m*prior) / (n + m)`, with `prior` from global mean per format, `m = 50` (tunable). Include `log(n+1)` as a confidence feature.
    - `phase`: one-hot per format. For Test, `phase='all'` → a single indicator (or omit and treat missing as zero).
    - `format_id`: include as categorical/one-hot to capture format effects directly.
  - Leakage control: ensure exporter enforces `as_of_date <= match_date`; use temporal splits by match date per format.
- Baseline experiments (per format): current model → +batting transitions → +bowling sequences; report lift.
- Metrics: classification AUC/PR-AUC for wicket- or dismissal-related targets; regression RMSE/MAE for runs/wickets.

---

## 10. Tooling, CI, and Standards
- Follow existing style and repo structure; Go 1.25+, Python 3.10+.
- Use `make migrate`, `go test ./...`, `pytest -q`.
- Lint/format according to defaults (`gofmt -s`, `go vet`, `black`, `ruff` if configured).
- Secrets: none added; env vars documented if needed.

---

## 11. Verification Commands (Acceptance)
```sh
# DB schema
make migrate

# Import sample(s) and populate ball_event
# Provide at least one sample per format under data/sample/ (t20, odi, test)
go run ./go-app/cmd/cricsheet-importer -data ./data/sample/t20
psql -c "SELECT match_id, innings, over, ball, ball_seq, phase, runs_total, wicket_kind FROM ball_event ORDER BY match_id, innings, ball_seq LIMIT 40;"

go run ./go-app/cmd/cricsheet-importer -data ./data/sample/odi
psql -c "SELECT DISTINCT phase FROM ball_event WHERE match_id IN (SELECT match_id FROM ball_event ORDER BY match_id DESC LIMIT 1);"

go run ./go-app/cmd/cricsheet-importer -data ./data/sample/test
psql -c "SELECT COUNT(*) FROM ball_event WHERE phase='all';"

# Precompute sequence features (all formats)
go run ./go-app/cmd/precompute-sequence-features
psql -c "SELECT * FROM batting_transition_features ORDER BY as_of_date DESC LIMIT 10;"
psql -c "SELECT * FROM bowling_sequence_features ORDER BY as_of_date DESC LIMIT 10;"

# Export dataset (no version bump)
make -C go-app export-dataset OUTPUT=./output/dataset_seq.csv

# Tests
make -C go-app test
make -C ml-service test
```

---

## 12. Branching & PR Strategy
- Feature branch: `feat/seq-features-t20` (never commit to main).
- Small PRs (1–3 files plus tests), Conventional Commits, open PR per phase.

---

## 13. Plan Hierarchy & Sub‑Plans

Active path: 1

### 1.1 PR1 — ball_event migration + DB layer
Parent: 1
- Objectives: Create `ball_event` schema and DB insert/upsert helper.
- Files:
  - `go-app/migrations/0014_ball_event.sql`
  - `go-app/internal/db/ball_event.go`
  - Tests: DB migration and insert idempotency tests
- Acceptance:
  - `make migrate` succeeds; unit tests pass.
- Verification:
  - `psql -c "\d+ ball_event"`
- Rollback: drop migration or run `down` if maintained; remove helper.

### 1.2 PR2 — Importer emits ball_event (phase + ball_seq)
Parent: 1
- Objectives: Persist deliveries during import with correct legality and sequencing (all formats supported by a single strategy function; tests start with T20).
- Files:
  - `go-app/internal/cricsheet/ingest.go`
  - Tests under `go-app/internal/cricsheet/` using deterministic Cricsheet fixtures in `data/sample/{t20,odi,test}/`.
- Acceptance:
  - Import samples; `SELECT` returns expected rows by `(innings, ball_seq)` and phases per format.
- Verification:
  - Run importer and SQL queries listed in §11.
- Rollback: feature flag to disable writes; revert changes.

### 1.3 PR3 — Backfill CLI
Parent: 1
- Objectives: Populate `ball_event` for existing matches from JSON (all formats).
- Files:
  - `go-app/cmd/backfill-ball-events/main.go`
  - `Makefile` target `backfill-ball-events`
  - Tests: CLI dry-run; verify counts
- Acceptance:
  - Backfill runs idempotently on samples; progress logs; no duplicates
- Verification:
  - Row counts stable across reruns
- Rollback: remove CLI and target.

### 1.4 PR4 — Precompute sequence features (schemas + job)
Parent: 1
- Objectives: Create sequence feature tables and computation job (all formats).
- Files:
  - `go-app/migrations/0015_sequence_features.sql`
  - `go-app/cmd/precompute-sequence-features/main.go`
  - Tests: unit tests for pairing/window logic; integration on samples across formats
- Acceptance:
  - Tables populated; expected sample rows visible across formats
- Verification:
  - SQL queries in §11
- Rollback: drop tables + job code.

### 1.5 PR5 — Exporter integration (additive)
Parent: 1
- Objectives: Join latest-as-of sequence features into exports (no v2). Ensure queries are format-aware.
- Files:
  - `go-app/cmd/export-dataset/*`
  - Tests: schema presence and latest-as-of correctness across formats
- Acceptance:
  - Export completes and includes new columns; existing columns unchanged
- Verification:
  - Inspect CSV header and sample rows
- Rollback: behind flag; revert joins.

### 1.6 PR6 — ML readers + baseline experiments
Parent: 1
- Objectives: Read new columns and quantify lift per format.
- Files:
  - `ml-service/` readers
  - `tests/` for reader contract
- Acceptance:
  - `pytest -q` passes; training completes on samples
- Verification:
  - Report baseline vs enhanced metrics per format
- Rollback: keep old readers; guard new readers behind config.

### 1.7 PR7 — Performance & polish
Parent: 1
- Objectives: Index tuning; documentation; finalize ODI/Test nuances (e.g., rain-shortened games, follow-on).
- Files: migrations (if needed), code tweaks, docs updates
- Acceptance: Same tests pass across formats; performance acceptable
- Verification: Timing logs; EXPLAIN ANALYZE on key queries
- Rollback: keep stable indexes only.

---

## 14. Risks & Mitigations
- Table growth: compact schema and targeted indexes; only necessary INCLUDEs.
- Rule variations across eras/formats: centralized `PhaseFor` with config; easy to adjust and backfill if needed.
- Edge cases: super overs, multiple wickets on one ball, rain-shortened innings, follow-on — covered with tests and documented assumptions.

---

## 15. Close Conditions
Plan 1 closes when PRs 1.1 through 1.6 are merged and verified across all formats and 1.7 is completed or explicitly deferred. Active path is updated in each PR and status note; parent/child progress reconciled after each merge.
