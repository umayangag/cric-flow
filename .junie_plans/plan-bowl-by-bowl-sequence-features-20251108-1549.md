# Plan: Bowl-by-Bowl Sequence Features (T20-first)

Status: Proposed
Owner: Junie
Date: 2025-11-08

## Goals
- Move from snapshot aggregates to ball-by-ball, sequence-aware features.
- Start with T20; extend to ODI/Test if straightforward.
- Keep current pipelines backward-compatible (no dataset version bump needed since not in production).
- Deliver tested, maintainable code in small PRs.

## High-level Design
1) Normalize deliveries into `ball_event` table during Cricsheet import.
2) Compute sequence-derived aggregates:
   - Batting transitions: performance of batter B immediately after batter A (start-of-innings and post-dismissal) by phase.
   - Bowling sequences: effect of bowler B’s over when following bowler A for the same team/innings and phase.
   - Momentum windows: recent balls/overs streaks for batters and bowlers (optional in later PR).
3) Integrate into dataset export (no dataset versioning) and add readers in `ml-service`.

## Scope & Assumptions
- Formats: T20 first; generalize with the same code paths if simple.
- Data source: Cricsheet v1.1 JSON (already parsed in repo).
- DB: Postgres (migrations under `go-app/migrations/`).
- Existing artifacts like `fielding_event` remain unchanged.

## Schemas (Approved)

### 1. ball_event
- Purpose: One row per delivery (legal and illegal) with core context.

```sql
CREATE TABLE IF NOT EXISTS ball_event (
  match_id       BIGINT NOT NULL,
  innings        SMALLINT NOT NULL,
  over           SMALLINT NOT NULL,
  ball           SMALLINT NOT NULL,
  ball_seq       INTEGER NOT NULL,         -- legal-delivery index within innings
  is_legal       BOOLEAN NOT NULL,
  phase          VARCHAR(16) NOT NULL,     -- powerplay, middle, death
  striker_id     BIGINT,
  non_striker_id BIGINT,
  bowler_id      BIGINT,
  runs_batter    SMALLINT NOT NULL DEFAULT 0,
  runs_extras    SMALLINT NOT NULL DEFAULT 0,
  runs_total     SMALLINT NOT NULL DEFAULT 0,
  extras_kind    VARCHAR(16),              -- wide, no_ball, bye, leg_bye, penalty
  wicket_kind    VARCHAR(24),              -- lbw, bowled, caught, run_out, stumped, etc.
  player_out_id  BIGINT,
  fielder_ids    BIGINT[],                 -- optional copy; primary source is fielding_event
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

Notes:
- `ball_seq` counts legal deliveries; wides/noballs will have `is_legal=false` and share the same `ball_seq` as the next legal ball for ordering context (or we leave `ball_seq` as last legal index and rely on `(over,ball)` for precise ordering; implementation detail documented in code and tests).

### 2. batting_transition_features
- Purpose: Performance of batter after a specific predecessor by phase.

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

### 3. bowling_sequence_features
- Purpose: Effect of bowler B’s over when following bowler A.

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

## ETL Changes
- Add `InsertBallEvent` DB method and upsert logic.
- Update Cricsheet importer to emit `ball_event` for each delivery:
  - Track `legal` via existing wide/no_ball logic.
  - Maintain `ball_seq` for legal deliveries.
  - Derive `phase` (Powerplay/Middle/Death) per format using `BallsPerOver` and rules.
  - Resolve/ensure player IDs (reusing existing helpers).
- Add `cmd/backfill-ball-events` to backfill existing matches.
- Add `cmd/precompute-sequence-features` to aggregate transitions:
  - Batting: identify predecessor at innings start and post-wicket; accumulate next-N-balls (e.g., N=12/18 configurable) and/or until next dismissal.
  - Bowling: for each completed over by B, pair with immediately preceding over by A for same fielding side; aggregate outcomes in B’s over.

## Dataset Export Integration (no v2)
- Extend existing exporter joins to include latest-as-of records from `batting_transition_features` and `bowling_sequence_features` by `(player_id, format_id, as_of_date <= match_date)` using the provided indexes.
- Add columns with a narrow initial set to limit scope (e.g., `bat_trans_prev_id`, `bat_trans_phase`, `bat_trans_sr`, `bowl_seq_prev_id`, `bowl_seq_phase`, `bowl_seq_wkt_rate`).
- Keep existing columns intact.

## ML Feature Transformation Plan
- Reader updates in `ml-service` to ingest new columns.
- Encodings:
  - `prev_batter_id`, `prev_bowler_id`: use target statistics (mean SR / wicket_rate) with Bayesian smoothing or leave as numeric IDs only where paired with their aggregated metrics to avoid high-cardinality one-hot.
  - `phase`: ordinal or one-hot (3 categories for T20).
- Numerical features:
  - `batting`: strike_rate, out_rate, balls (sample size), optionally winsorized; add log(balls+1) as confidence weight.
  - `bowling`: wicket_rate, econ, dot_rate, balls.
- Regularization and leakage controls:
  - Compute features with `as_of_date <= match_date` (enforced by exporter query).
  - Time-based train/val/test split (by match date) to respect temporal leakage.
- Baselines:
  - Start with current snapshot-only model.
  - Add sequence features and measure lift (AUC/MAE/RMSE depending on target).
- Evaluation:
  - For classification (e.g., wicket next over/ball): AUC/PR-AUC.
  - For regression (player runs/wickets): MAE/RMSE.
- Reproducibility:
  - Fixed seeds; small deterministic fixtures in `tests/fixtures/`.

## Phases & PR Breakdown
Each PR targets 1–3 files + tests, with Conventional Commits.

- PR1: Migrations for `ball_event` (and helper indexes) + DB layer
  - Files:
    - `go-app/migrations/0014_ball_event.sql`
    - `go-app/internal/db/ball_event.go` (insert/upsert)
    - Tests: DB migration and insert idempotency tests
  - Accept: `make migrate` succeeds; unit tests pass.

- PR2: Importer writes `ball_event` (+ phase + ball_seq)
  - Files:
    - `go-app/internal/cricsheet/ingest.go` (emit `ball_event`)
    - `go-app/internal/cricsheet/` tests for legality, sequencing, and phase derivation using small fixture under `data/sample/`
  - Accept: Import sample; `SELECT` returns expected rows ordered by `(innings, ball_seq)`.

- PR3: Backfill CLI
  - Files:
    - `go-app/cmd/backfill-ball-events/main.go`
    - Makefile target `backfill-ball-events`
    - Tests: CLI dry-run; verify counts with sample data
  - Accept: Runs on sample directory; idempotent.

- PR4: Precompute job for sequence features
  - Files:
    - `go-app/migrations/0015_sequence_features.sql`
    - `go-app/cmd/precompute-sequence-features/main.go`
    - Tests: unit tests for pairing/window logic; integration on sample
  - Accept: Tables populated; sample queries return expected rows.

- PR5: Exporter integration (no dataset version bump)
  - Files:
    - `go-app/cmd/export-dataset/*` (joins to new feature tables)
    - Tests: ensure columns present and latest-as-of logic correct
  - Accept: Export completes; schema includes new columns; existing columns unchanged.

- PR6: ml-service readers + baseline experiment
  - Files:
    - `ml-service/` readers to parse added columns
    - `tests/` for reader contract
    - Optional: simple baseline model to quantify lift
  - Accept: `pytest -q` passes; training completes on sample.

- PR7: Performance & generalization (optional)
  - Add ODI/Test phase rules if straightforward; minor index tuning; documentation updates.

## Makefile targets (to add or confirm)
- `make migrate`
- `make backfill-ball-events`
- `make precompute-sequence-features`
- `make export-dataset`
- `make test` (top-level), `make -C go-app test`, `make -C ml-service test`

## Acceptance Criteria (Overall)
- Migrations apply cleanly and are reversible.
- Importer populates `ball_event` deterministically on sample fixture.
- Precompute job writes expected rows for both sequence feature tables.
- Exporter adds new columns without breaking existing ones.
- ml-service can read and train with the new features on sample data.

## Verification Commands
```sh
# DB schema
make migrate

# Import sample and populate ball_event
go run ./go-app/cmd/cricsheet-importer -data ./data/sample
psql -c "SELECT match_id, innings, over, ball, ball_seq, phase, runs_total, wicket_kind FROM ball_event ORDER BY match_id, innings, ball_seq LIMIT 40;"

# Precompute sequence features
go run ./go-app/cmd/precompute-sequence-features
psql -c "SELECT * FROM batting_transition_features ORDER BY as_of_date DESC LIMIT 10;"
psql -c "SELECT * FROM bowling_sequence_features ORDER BY as_of_date DESC LIMIT 10;"

# Export dataset (no version bump)
make -C go-app export-dataset OUTPUT=./output/dataset_seq.csv

# Tests
make -C go-app test
make -C ml-service test
```

## Branching & PR Process
- Create feature branch: `feat/seq-features-t20`.
- Open small, focused PRs per phase above.
- Conventional commits (e.g., `feat: add ball_event schema` / `feat: importer emits ball_event`).

## Risks & Mitigations
- Volume growth: `ball_event` grows linearly; mitigate with compact schema and narrow indexes.
- Rule differences across formats: encapsulate phase rules in a strategy function and config.
- Sequence edge cases (super overs, penalty runs): cover with tests and documented assumptions.

## Out-of-Scope (initially)
- Live commentary ingestion.
- Predictive online inference endpoints (batch-only for now).

## Next Actions
- Approve this plan.
- Create feature branch and execute PR1.
