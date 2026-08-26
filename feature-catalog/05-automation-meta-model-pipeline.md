# Plan 05: Automate Pipeline and Meta-Model (Priority 5)

**Goal:** Automate production of the combination meta-model from backtest evaluate results; add optional pipeline steps for evaluation and one-command retrain+evaluate. Weather is out of scope.

---

## 1. Objective

- **Meta-model automation:** After backtest evaluate (or a batch of evaluates), produce the contributions CSV (bat_score, bowl_score, field_score, is_keeper, format, target) and run `train_combination_meta` to write combination_meta.json; document how to set selection.meta_model_path.
- **Pipeline:** Optional step “train_combination_meta” that can be run after train_win (or after evaluate). Optionally an “evaluate” step that runs a batch evaluate and stores metrics.
- **One-command retrain and evaluate:** A Make target or script that runs: precompute → export → train all → (optional) run combination meta training → (optional) run a small evaluation and report MAE/winner accuracy.

---

## 2. Current State

| Component | Location | Behavior |
|-----------|----------|----------|
| Pipeline steps | `go-app/internal/server/ops_status_pipeline.go` | import, precompute, export, train_batting, train_bowling, train_fielding, train_extras, train_win, auto_tune. No train_combination_meta. |
| Pipeline run | `go-app/internal/server/pipeline_handlers.go` | Runs step via tracking and for train_* calls ML service /admin/train/{step}. |
| train_combination_meta | `ml-service/ml/train_combination_meta.py` | Expects CSV with bat_score, bowl_score, field_score, is_keeper, format, target. Outputs JSON. |
| Backtest evaluate | `go-app/internal/server/backtest_handlers.go` | doEvaluateWork produces backtestEvaluateResponse with Players (predicted, actual), MatchAggregates. No export of “contribution” rows for meta-model. |

To produce the meta-model CSV we need: for each player in each evaluate result, (bat_score, bowl_score, field_score, is_keeper, format, target). bat/bowl/field scores are the normalized scores used in selection (from predicted runs, wickets, economy, catches, run_outs). target = actual contribution (e.g. actual runs / bat_divisor, or a composite). So we need to either: (1) export from evaluate response when we have it, or (2) run a batch job that runs evaluate on N matches and accumulates rows, then writes CSV.

---

## 3. Acceptance Criteria

> **Status (2026-08): all four are met.** Verified against `main`:
> `POST /api/backtest/export-contributions` (+ `-status`) in `router.go`;
> the `train_combination_meta` pipeline step delegating to ml-service's
> `POST /admin/train/combination-meta` (C5-2 — it previously returned 501);
> `make full-pipeline` (the `retrain-and-evaluate` name below was the
> alternative that was not taken); and `meta_model_path` in
> `docs/ml-and-training.md`. Kept for the design reasoning.


- [x] **Export contributions from evaluate:** When backtest evaluate runs, optionally write one row per player to a contributions file (or accumulate in memory for a batch). Row: bat_score, bowl_score, field_score, is_keeper (0/1), format, target. Target = actual runs / bat_divisor (or similar composite from actuals). This may be a new API or a side effect of evaluate when a query param or config is set (e.g. export_contributions=true and path).
- [x] **Pipeline step train_combination_meta:** New step that (1) expects the contributions CSV to exist at a known path (e.g. from a previous “evaluate” batch that wrote it), or (2) triggers a batch evaluate that writes the CSV, then (3) calls the ML service or runs `make train-combination-meta` to produce combination_meta.json. Document that operators must run evaluate batch first or provide the CSV.
- [x] **One-command retrain + evaluate:** Make target (e.g. `make full-pipeline` or `make retrain-and-evaluate`) that: runs precompute, export, train_batting, train_bowling, train_fielding, train_extras, train_win; optionally runs train_combination_meta if CSV exists; optionally runs a fixed set of backtest evaluates and reports metrics. Does not run import (assume data already imported).
- [x] **Documentation:** How to generate the contributions CSV, run train_combination_meta, and set selection.meta_model_path in config.

---

## 4. Implementation Details

### 4.1 Contributions CSV from evaluate

**Option A (simpler):** New endpoint or query param that returns contributions in CSV format.

- **File:** `go-app/internal/server/backtest_handlers.go` or a new handler.
- After doEvaluateWork, if request has e.g. `?export_contributions=1` or a dedicated endpoint `GET /api/backtest/contributions?format=...&team1=...&team2=...&match_ids=...`, run evaluate for each match (or use existing evaluate), and for each player build a row: bat_score = normalized predicted batting (from predicted runs / bat_divisor), bowl_score = normalized bowling, field_score = normalized fielding, is_keeper from pool, format, target = actual_runs/bat_divisor (or composite). Append to response or write to file.
- **Option B:** Batch job: `POST /api/backtest/export-contributions` with body { format, team1, team2, match_ids[] }. Server runs evaluate for each match_id, collects rows, writes CSV to a configured path (e.g. output_dir/backtest_contributions.csv), returns path. Then operator runs train_combination_meta with that CSV.

Prefer Option B for clarity: a dedicated “export contributions” job that (1) takes a list of match_ids (or “last N matches” for format/teams), (2) runs evaluate for each, (3) builds rows with (bat_score, bowl_score, field_score, is_keeper, format, target), (4) writes CSV to output dir, (5) returns path. Then pipeline step “train_combination_meta” runs after that, reading from that path.

### 4.2 Building contribution rows

- For each player in the evaluate result we have: Predicted (runs, wickets, economy, catches, run_outs), Actual (same). We need normalized scores as in predict_team: bat_score = min(1, runs/bat_divisor), bowl_score from wickets and economy, field_score from catches and run_outs. Use the same config divisors (EffectiveScoreNormParams). is_keeper: from DB or from squad — we need to know if the player is a keeper; get from pool or match squad. format: from request. target: use actual contribution, e.g. actual_runs/bat_divisor + actual_wickets/wicket_divisor (or a single composite). Keep it simple: target = actual_runs / bat_divisor so the meta-model learns to weight batting score by how well it predicted runs.
- So each row: bat_score (from predicted), bowl_score (from predicted), field_score (from predicted), is_keeper (0/1), format, target (actual_runs/bat_divisor or similar).

### 4.3 Pipeline step train_combination_meta

**File:** `go-app/internal/server/ops_status_pipeline.go`

- Add step "train_combination_meta" with previous step "train_win" (or "export_contributions" if we add that). So order: ... train_win, train_combination_meta. train_combination_meta is runnable when train_win has completed and (optionally) when contributions CSV exists.
- **File:** `go-app/internal/server/pipeline_handlers.go`
  - Add case for "train_combination_meta": call ML service or shell out to `make train-combination-meta CSV=<path> OUT=<path>`. Path for CSV from config (e.g. output_dir/backtest_contributions.csv) or env. Path for OUT from config (e.g. output_dir/ml-service/combination_meta.json).
- **File:** `go-app/internal/server/pipeline_progress.go` — add step label for train_combination_meta.

### 4.4 Export contributions endpoint

**File:** `go-app/internal/server/backtest_handlers.go` or new file

- `POST /api/backtest/export-contributions` (or GET with query params). Request: format, team1, team2, match_ids (or limit). For each match_id, run doEvaluateWork, collect players; for each player compute bat_score, bowl_score, field_score (from predicted), is_keeper (need to load squad and get keeper flag), format, target (from actual). Write CSV to configured path. Return { "path": "...", "rows": N }.
- Need to get is_keeper per player: from DB player table or from match squad. If match squad doesn’t have keeper flag, use 0 for all (or skip is_keeper column and let meta-model use 0). Prefer: look up player in pool for that match/format and get IsWicketKeeper from ListPlayerPoolByTeam or similar.

### 4.5 Make target

**File:** `Makefile` (repo root or go-app)

- Add target `full-pipeline` or `retrain-and-evaluate`:
  - Depends: precompute-all, export-dataset, train-batting, train-bowling, train-fielding, train-extras, train-win (in order or in parallel where safe).
  - Optional: if backtest_contributions.csv exists, run make train-combination-meta.
  - Optional: run a small evaluate (e.g. one match per format) and echo metrics. Can be a separate target `evaluate-sample` that curls the API.
- Document in README or docs/ml-and-training.md.

### 4.6 Documentation

**File:** `docs/ml-and-training.md`

- Add section: “Combination meta-model automation”. Steps: (1) Run backtest export-contributions with desired match list to generate CSV. (2) Run pipeline step train_combination_meta or `make train-combination-meta CSV=... OUT=...`. (3) Set selection.meta_model_path in config to the output JSON path. (4) Restart or reload config so team selection uses learned weights.

---

## 5. File Checklist

| File | Action |
|------|--------|
| `go-app/internal/server/backtest_handlers.go` or new | Add export-contributions handler: run evaluate for given matches, build CSV rows, write to file. |
| `go-app/internal/server/ops_status_pipeline.go` | Add train_combination_meta step; previous = train_win. |
| `go-app/internal/server/pipeline_handlers.go` | Add handler for train_combination_meta (call ML or Make). |
| `go-app/internal/server/pipeline_progress.go` | Add label for train_combination_meta. |
| `Makefile` | Add full-pipeline or retrain-and-evaluate target. |
| `docs/ml-and-training.md` | Document export-contributions, train_combination_meta, meta_model_path. |

---

## 6. Weather

No weather in this plan.
