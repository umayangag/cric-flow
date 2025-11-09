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
- Derived feature stores (materialized tables) capturing rich sequence dynamics, windows, and reactions:
  - `batting_transition_features` (B after A by phase/format)
  - `bowling_sequence_features` (B after A by phase/format)
  - `player_window_features` (batter and bowler rolling windows at multiple horizons)
  - `entry_set_batter_features` (batter entry vulnerability and set-batter acceleration)
  - `partnership_features` (pair-level batting tendencies)
  - `pressure_state_features` (performance under match pressure states)
  - `event_reaction_features` (immediately-after specific prior events)
  - `dot_streak_features` (behavior after sequences of dots)
  - `extras_discipline_features` (wide/no-ball discipline)
  - `wicket_mode_features` (dismissal mode distributions)
  - `bowling_spell_features` (spell position effects)
  - `over_boundary_wicket_features` (start/end of over tendencies)
- Precompute job(s) to populate all derived tables incrementally for all formats.
- Exporter joins to include a compact, high-signal subset of these features.
- ML readers that transform these features appropriately and baseline experiments demonstrating impact.

---

## 5. Data Model (Approved core + new)

### 5.1 `ball_event` (core)
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

### 5.2 Transition features (core)

- `batting_transition_features` — performance of batter B after predecessor A by phase (as previously defined with latest-as-of indexes).
- `bowling_sequence_features` — over-by-over effect of B following A (as previously defined with latest-as-of indexes).

### 5.3 New feature stores (schemas)

For all below, columns include at minimum: `as_of_date DATE`, `format_id SMALLINT`, `scope VARCHAR(16) DEFAULT 'overall'`, `scope_id BIGINT NULL`, plus keys and aggregates listed. Primary keys follow the pattern `(as_of_date, format_id, scope, COALESCE(scope_id,0), <keys...>)`. Add latest-as-of indexes mirroring snapshot tables.

1) `player_window_features` — rolling windows for both batters and bowlers at multiple horizons (e.g., 6, 12, 24, 30 balls for batters; 12, 24, 30, 36 balls for bowlers)
- Keys: `player_id`, `role` in ('bat','bowl'), `phase`, `horizon`
- Aggs (bat): `balls`, `runs`, `fours`, `sixes`, `dots`, `dismissals`, `sr`, `boundary_rate`, `dot_rate`, `dismissal_hazard`
- Aggs (bowl): `balls`, `runs`, `wickets`, `dot_balls`, `boundaries_conceded`, `wide_nb`, `econ`, `dot_rate`, `wicket_rate`

2) `entry_set_batter_features` — batter entry and set-batter behavior
- Keys: `batter_id`, `phase`, `segment` in ('entry_1_6','entry_7_12','set_13_30','set_31_plus')
- Aggs: `balls`, `runs`, `sr`, `dismissals`, `boundary_rate`

3) `partnership_features` — pair-level batting synergy regardless of order (sorted pair)
- Keys: `batter1_id`, `batter2_id`, `phase`
- Aggs: `balls`, `runs`, `dismissals`, `sr`, `boundary_rate`, `avg_partnership_length`

4) `pressure_state_features` — performance under match pressure states (chasing or defending)
- Keys: `player_id`, `role`, `pressure_bucket`, `phase`
- Pressure buckets (chase): based on required RR (`<6`, `6-8`, `8-10`, `>10`) and wickets_in_hand (`<=3`, `4-6`, `>=7`)
- Pressure buckets (defend): based on current RR vs par, wickets_in_hand of opposition
- Aggs (bat): `balls`, `runs`, `sr`, `dismissals`; (bowl): `balls`, `runs`, `econ`, `wickets`

5) `event_reaction_features` — immediate next-ball performance conditioned on prior event
- Keys: `player_id`, `role`, `prev_event` in ('dot','1','2','3','4','6','wide','no_ball','wicket','bye','leg_bye'), `phase`
- Aggs (bat): `balls`, `runs`, `sr`, `dismissals`, `boundary_rate`; (bowl): `balls`, `runs`, `wickets`, `dot_rate`

6) `dot_streak_features` — outcomes after k consecutive dots
- Keys: `player_id`, `role`, `k` (0..6), `phase`
- Aggs: probability of `boundary`, `single`, `wicket`, `extra`, plus `avg_runs_next_ball`

7) `extras_discipline_features` — wides and no-balls discipline (bowlers)
- Keys: `bowler_id`, `phase`
- Aggs: `balls`, `wides`, `no_balls`, `wide_rate`, `no_ball_rate`, `penalty_runs`

8) `wicket_mode_features` — mode-of-dismissal distributions
- Keys: `player_id`, `role` in ('bat','bowl'), `phase`, `wicket_kind`
- Aggs: `count`, `rate` (per ball for bowlers, per dismissal for batters), smoothing-ready

9) `bowling_spell_features` — spell position effects (first over of spell vs later)
- Keys: `bowler_id`, `phase`, `spell_pos` in ('first_over','second_over','later')
- Aggs: `overs`, `runs`, `wickets`, `dot_balls`, `econ`, `wicket_rate`

10) `over_boundary_wicket_features` — start/end-of-over tendencies
- Keys: `player_id`, `role`, `phase`, `over_ball_pos` in ('ball1','ball6')
- Aggs (bat): `sr`, `boundary_rate`; (bowl): `wicket_rate`, `dot_rate`, `econ`

All tables include generated columns where helpful (e.g., `sr`, `econ`, rates) and latest-as-of indexes similar to `0013_feature_snapshot_indexes.sql`.

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
  - Phase: `all` (single bucket) to avoid arbitrary boundaries; sequence features computed without phase segmentation. Optionally later: `opening/middle/tail`.
- Super over: treated as a separate innings; same T20 phase logic but practically very short (nearly all `powerplay`).

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
- Add `cmd/precompute-sequence-features` to compute and upsert into all sequence tables (all formats):
  - Reconstruct state (chasing/defending, runs_needed, balls_remaining, wickets_in_hand) from score progression in `match_details`/`ball_event`.
  - Compute transitions, windows, pressure buckets, dots streaks, event reactions, spell changes, and over positions.
  - Ensure `as_of_date = match_date` and idempotent writes.

---

## 8. Exporter Integration (no dataset v2)
Export only a compact, high-signal subset initially to keep columns manageable; allow flags to expand later.

- Batting additions (examples):
  - `bat_prev_batter_id`, `bat_prev_phase`, `bat_prev_sr`, `bat_prev_out_rate`
  - `bat_window_sr_12_pp`, `bat_window_boundary_rate_12_pp`, `bat_entry_sr_1_6`, `bat_set_sr_13_30`
  - `bat_pressure_sr_rr_gt8_wih_le3`, `bat_react_after_dot_sr`, `bat_after_k_dots_boundary_p(k=2)`
  - `bat_partnership_sr_top_prev_partner` (optional)
- Bowling additions (examples):
  - `bowl_prev_bowler_id`, `bowl_prev_phase`, `bowl_prev_wkt_rate`
  - `bowl_window_econ_24_death`, `bowl_window_wkt_rate_24_death`, `bowl_extras_wide_rate_pp`
  - `bowl_react_after_boundary_wkt_rate_next`, `bowl_spell_first_over_wkt_rate`
  - `bowl_over_ball1_wkt_rate`, `bowl_over_ball6_dot_rate`

Joins use latest-as-of by `(player_id, format_id, as_of_date <= match_date)` and provided indexes.

---

## 9. ML Feature Transformation (All formats)
- Readers (ml-service):
  - Parse added exporter columns; keep a YAML/JSON config mapping to feature groups for easy A/B toggling.
  - Encodings:
    - Rates with Bayesian smoothing: `rate_smooth = (sum + m*prior) / (n + m)`, with `prior` per-format global mean; include `log(n+1)` confidence.
    - `phase` one-hot per format; for Test, treat `all` as single-hot.
    - `format_id` one-hot.
  - Normalization: winsorize extreme rates; standardize continuous features.
  - Leakage control: exporter enforces `as_of_date <= match_date`; temporal split by match date.
- Baselines (per format):
  - Baseline A: current snapshot features only.
  - Baseline B: A + transitions (bat & bowl).
  - Baseline C: B + windows + pressure.
  - Baseline D: C + reaction & streaks + extras discipline.
- Metrics: AUC/PR-AUC for wicket-related, RMSE/MAE for runs/wickets.
- Reproducibility: fixed seeds; deterministic test fixtures.

---

## 10. Tooling, CI, and Standards
- Follow existing style and repo structure; Go 1.25+, Python 3.10+.
- Use `make migrate`, `go test ./...`, `pytest -q`.
- Lint/format according to defaults (`gofmt -s`, `go vet`, `black`, `ruff` if configured).

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
psql -c "SELECT * FROM player_window_features ORDER BY as_of_date DESC LIMIT 10;"
psql -c "SELECT * FROM pressure_state_features ORDER BY as_of_date DESC LIMIT 10;"

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

### 1.2 PR2 — Importer emits ball_event (phase + ball_seq)
Parent: 1
- Objectives: Persist deliveries during import with correct legality and sequencing (all formats supported by a single strategy function; tests start with T20).
- Files:
  - `go-app/internal/cricsheet/ingest.go`
  - Tests under `go-app/internal/cricsheet/` using deterministic Cricsheet fixtures in `data/sample/{t20,odi,test}/`.
- Acceptance:
  - Import samples; `SELECT` returns expected rows by `(innings, ball_seq)` and phases per format.

### 1.3 PR3 — Backfill CLI
Parent: 1
- Objectives: Populate `ball_event` for existing matches from JSON (all formats).
- Files:
  - `go-app/cmd/backfill-ball-events/main.go`
  - `Makefile` target `backfill-ball-events`
  - Tests: CLI dry-run; verify counts
- Acceptance:
  - Backfill runs idempotently on samples; progress logs; no duplicates

### 1.4 PR4 — Core transitions (schemas + job)
Parent: 1
- Objectives: Create `0015_sequence_features.sql`; implement `precompute-sequence-features` for batting/bowling transitions.
- Files:
  - `go-app/migrations/0015_sequence_features.sql`
  - `go-app/cmd/precompute-sequence-features/main.go`
  - Tests: unit tests; integration on samples
- Acceptance:
  - Tables populated; expected sample rows visible across formats

### 1.5 PR5 — Windows & Entry/Set (schemas + job)
Parent: 1
- Objectives: Add `player_window_features` and `entry_set_batter_features` with computations.
- Files:
  - `go-app/migrations/0016_player_windows.sql`
  - `go-app/cmd/precompute-sequence-features/windows.go`
  - Tests: unit tests; integration on samples
- Acceptance: rows populated; sanity on window math.

### 1.6 PR6 — Pressure & Reaction & Dot-streaks
Parent: 1
- Objectives: Add `pressure_state_features`, `event_reaction_features`, `dot_streak_features` with computations.
- Files:
  - `go-app/migrations/0017_pressure_reaction_streaks.sql`
  - `go-app/cmd/precompute-sequence-features/pressure_reaction.go`
  - Tests: unit tests; integration on samples
- Acceptance: rows populated; buckets correct.

### 1.7 PR7 — Extras discipline, wicket modes, spell & over-pos
Parent: 1
- Objectives: Add `extras_discipline_features`, `wicket_mode_features`, `bowling_spell_features`, `over_boundary_wicket_features` with computations.
- Files:
  - `go-app/migrations/0018_extras_modes_spells_overpos.sql`
  - `go-app/cmd/precompute-sequence-features/discipline_modes_spells.go`
  - Tests: unit tests; integration on samples
- Acceptance: rows populated; modes and rates consistent.

### 1.8 PR8 — Exporter integration (additive)
Parent: 1
- Objectives: Join a compact subset of high-signal features (bat + bowl) into exports (no v2). Ensure queries are format-aware and latest-as-of.
- Files:
  - `go-app/cmd/export-dataset/*`
  - Tests: schema presence and correctness across formats
- Acceptance: Export completes with new columns; existing columns unchanged

### 1.9 PR9 — ML readers + staged baselines
Parent: 1
- Objectives: Read new columns and run staged baselines (A→D) per format.
- Files:
  - `ml-service/` readers and configs
  - `tests/` for reader contract
- Acceptance: `pytest -q` passes; baseline reports produced.

### 1.10 PR10 — Performance & polish
Parent: 1
- Objectives: Index tuning; documentation; finalize ODI/Test nuances (rain-shortened, follow-on).
- Files: migrations (if needed), code tweaks, docs updates
- Acceptance: Same tests pass across formats; performance acceptable

---

## 14. Risks & Mitigations
- Table growth: compact schemas and targeted indexes; only necessary INCLUDEs; consider partitioning by match_date if needed later.
- Rule variations across eras/formats: centralized `PhaseFor` with config; easy to adjust and backfill.
- Edge cases: super overs, multiple wickets on one ball, rain-shortened innings, follow-on — covered with tests and documented assumptions.

---

## 15. Close Conditions
Plan 1 closes when PRs 1.1 through 1.9 are merged and verified across all formats and 1.10 is completed or explicitly deferred. Active path is updated in each PR and status note; parent/child progress reconciled after each merge.
