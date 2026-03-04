## Model Harmony – Implementation & Evolution Plan

This document is a **concrete, implementation-focused companion** to `model-harmony-plan.md`. It lists:

- What is already implemented (with references),
- What remains to be done,
- How to evolve towards Stage 2 (consistency-aware training) and beyond,
- How to wire metrics and monitoring so future work stays on track.

Status key: `[ ]` pending, `[~]` in-progress, `[x]` done.

---

## 1. Current state summary (PR #91 baseline)

- [x] **Canonical match representation**
  - `ml.match_schema` defines `BallEvent`, `InningsState`, `MatchState` with derived properties (`total_runs`, `total_wickets`, `legal_balls`, extras breakdown).
  - `ml.match_aggregates` defines `BattingLine`, `BowlingLine` and aggregation helpers from `MatchState`.

- [x] **Reconciliation core & solver**
  - `ml.reconciliation_core`:
    - `VariableKind` (BAT_RUNS, BAT_BALLS, BOWL_RUNS, BOWL_BALLS, BOWL_WKTS).
    - `ProblemBuilder` builds `ReconciliationProblem` (variables, μ, weights, constraints).
    - Encodes **runs** and **wickets** constraints for both innings.
    - Encodes **balls** constraints for both innings when `preferred_legal_balls` is present:
      - Sum BAT_BALLS for batting team = preferred_legal_balls.
      - Sum BOWL_BALLS for bowling team = preferred_legal_balls.
  - `ml.reconciliation_solver`:
    - Solves the diagonal QP via Lagrange multipliers, enforcing all **hard** constraints.

- [x] **Reconciliation rounding & service**
  - `ml.reconciliation_service`:
    - `_sum_preserving_round` now robust for **positive and negative deltas**, clamping totals to the rounded target without negative stats.
    - `reconcile_match_players`:
      - Converts continuous solution into integer `ReconciledPlayerStats`.
      - Applies sum-preserving adjustments for runs, wickets, and balls per innings/team to match targets.

- [x] **Consistency checker & adjustment magnitude**
  - `ml.consistency_checker`:
    - `check_reconciled_scorecard_consistency` re-checks:
      - Team batting/bowling runs vs innings targets.
      - Team wickets vs innings targets.
      - Balls vs `preferred_legal_balls` where present.
    - `adjustment_magnitude(before, after)` returns:
      - `mean_abs_delta_runs`, `mean_abs_delta_wickets`,
      - `mean_abs_pct_delta_runs`, `mean_abs_pct_delta_wickets`,
      - `total_before_runs`, `total_before_wickets`.

- [x] **Prediction layer integration**
  - `app.prediction_service`:
    - Uses `INNINGS_MODELS` to predict innings totals when `match_context` is present.
    - Hybrid reconciliation path for non-share models:
      - Builds player-level predictions (`BacktestPlayerPred`).
      - Calls reconciliation adapter when available.
      - Logs a structured event `backtest_predict.reconciliation.applied` with innings totals and adjustment metrics.
  - `ml.reconciliation_adapter`:
    - `build_match_reconciliation_inputs_from_backtest_preds`:
      - Creates `MatchReconciliationInputs` from `BacktestPlayerPred`, team ids, innings targets, and format.
      - Uses `ml.config.get_prediction_defaults()` for `bowling_deliveries_by_format`.
    - `apply_constraint_reconciliation_from_backtest_preds`:
      - Calls `reconcile_match_players`.
      - Builds **before/after** `ReconciledPlayerStats`.
      - Uses `adjustment_magnitude` and `check_reconciled_scorecard_consistency` to return a rich `adjustment` dict.

- [x] **Win features & coherence helpers**
  - `ml.win_features_from_reconciled.build_win_features_standardized`:
    - Canonical mapping from scalar context and consistency/form sums → `WinFeatures`.
  - `ml.win_coherence_metrics`:
    - `implied_win_probability_from_margin(margin, scale)` maps reconciled score margin → implied \(P(\text{team1 wins})\) via a robust logistic.
    - `win_probability_coherence_from_margin(p_model_team1, margin, scale)` returns:
      - `p_model_team1`, `p_implied_team1`, and `abs_diff`.

- [x] **Harmony & realism metrics**
  - `ml.harmony_metrics`:
    - `distribution_summary(x)` basic stats for any 1D sample.
    - `realism_metrics_vs_historical(reconciled, historical)` compares mean/std/median reconciliation vs historical.

- [x] **Stage 2 scaffolding (consistency-aware training)**
  - `ml.consistency_losses`:
    - `consistency_regularization_loss(before, after, weight_runs, weight_wickets)`:
      - Uses `adjustment_magnitude` to return a scalar penalty based on mean absolute percentage deltas in runs and wickets.
      - Designed for **offline training/tuning** (not yet wired into model fitting loops).

---

## 2. Stage 2 – Consistency-aware training

Goal: Teach models to be **easier to reconcile** by incorporating reconciliation-aware signals into training and model selection, while minimizing disruption to the existing scikit-learn based stack.

We split Stage 2 into three layers:

1. **Metrics-only**: record how “hard” each model is to reconcile on validation data.
2. **Model selection**: use a combined objective = (prediction error) + λ·(consistency penalty) to pick artifacts.
3. **True custom-loss training** (optional, more invasive): swap to estimators that expose loss hooks.

### 2.1 Metrics-only: compute and log consistency metrics per candidate model

- [ ] **2.1.1 Add a generic “evaluate-with-reconciliation” helper**
  - New module: `ml/consistency_eval.py`
    - Functions:
      - `build_before_after_stats_from_predictions(...)`:
        - Inputs:
          - Raw per-player predictions for a validation set (`BacktestPlayerPred`-like structures),
          - Team ids and innings targets (from held-out data or validation labels),
          - Format code.
        - Steps:
          - Build `MatchReconciliationInputs` via `ml.reconciliation_adapter`.
          - Run `reconcile_match_players` to get `after` stats.
          - Build `before` stats mirroring `after`’s structure from raw predictions.
        - Output:
          - `(before_stats: Mapping[int, ReconciledPlayerStats], after_stats: Mapping[int, ReconciledPlayerStats])`.
      - `compute_consistency_metrics(before_stats, after_stats)`:
        - Wraps `adjustment_magnitude` plus any additional summary we need.

- [ ] **2.1.2 Expose metrics from training_pipeline**
  - Extend `training_pipeline.TrainingPipeline` with:
    - An optional hook `evaluate_consistency_on_validation(...)`:
      - Takes validation slices `(X_val, Y_val)` and model/scaler.
      - Option 1 (low effort): approximate before/after just on **innings totals** using existing reconciliation logic, without full player ids.
      - Option 2 (richer; may require extra labels): use stored per-player validation predictions and team compositions to compute before/after stats properly.
    - Writes consistency metrics into:
      - Metadata JSON alongside artifacts (`*_metadata_*.json`),
      - Logs with a clear key, e.g. `training.consistency.metrics`.

- [ ] **2.1.3 Do this first for one model family**
  - Start with **innings model** or **batting model**:
    - Smaller dimensionality than full combination meta-model.
    - Good signal for how far reconciliation typically shifts team totals.
  - Once stable, replicate pattern for other models (bowling, extras, win).

### 2.2 Model selection: combined objective for auto-tuning / pipeline

- [ ] **2.2.1 Extend tuning to accept an extra scalar metric**
  - In `ml/auto_tune` (or equivalent tuning logic):
    - When training candidate models under different hyperparameters, compute:
      - Base score (e.g. MAE, RMSE) on validation set.
      - Consistency penalty via `consistency_regularization_loss(before, after, ...)`.
    - Define a combined score:
      - `score_combined = base_error + lambda_consistency * consistency_penalty`.
    - Use `score_combined` as the primary ranking metric for choosing best parameters.

- [ ] **2.2.2 Configuration for λ and reporting**
  - Extend `ml.config` to support `ml.consistency_regularization`:
    - Fields:
      - `lambda_runs`, `lambda_wickets` (per-model defaults, e.g. per `ml.training.batting.consistency` section).
      - Flags to enable/disable consistency penalty per model.
  - Ensure tuning logs **both**:
    - Base error metrics (for interpretability).
    - Consistency penalty and combined score (for harmony tracking).

- [ ] **2.2.3 Backward-compatibility & rollout**
  - Make consistency-aware selection:
    - **Off by default** initially (config flag).
    - Enabled for experimental runs and specific formats.
  - Guard rails:
    - If consistency evaluation or reconciliation fails on a candidate:
      - Fallback to base-error ranking for that run.

### 2.3 Optional: true custom-loss training (beyond scikit-learn RF)

> This is intentionally future work; the design should be captured now, but implementation can follow once we are comfortable changing model families.

- [ ] **2.3.1 Choose differentiable model families**
  - Evaluate options:
    - Gradient-boosted trees with custom loss (e.g. XGBoost/LightGBM with custom objective where feasible).
    - Neural networks in PyTorch or similar, where the loss can include:
      - Standard prediction error term,
      - Differentiable approximation of consistency penalty (e.g. L2 between implied team totals and constraints).
  - Decide per model family:
    - For example, start with **innings model** or **win model** as the first candidate.

- [ ] **2.3.2 Design differentiable consistency penalties**
  - Possible approaches:
    - Use soft versions of reconciliation constraints (e.g. penalty on difference between predicted aggregates and known team totals) directly in the loss.
    - Approximate the effect of reconciliation via a differentiable surrogate, rather than calling the full QP in the inner loop.

- [ ] **2.3.3 Integrate into training_pipeline**
  - Likely via a parallel path:
    - `TrainingPipeline` gains a “backend” switch (sklearn vs custom).
    - Custom backend handles its own loss and optimization loop, but still produces artifacts conforming to the inference API (models, scalers, metadata).

---

## 3. Harmony, realism, and win coherence metrics (Monitoring)

Goal: Turn reconciliation into a **first-class signal** for model and system health, wired into offline analysis and eventually dashboards.

### 3.1 Adjustment magnitude & per-model bias (already scaffolded)

- [x] **3.1.1 Adjustment magnitude** via `ml.consistency_checker.adjustment_magnitude`
  - Already used in `ml.reconciliation_adapter.apply_constraint_reconciliation_from_backtest_preds`.
  - Exposed to `app.prediction_service` logs via `backtest_predict.reconciliation.applied`.

- [ ] **3.1.2 Aggregate adjustment metrics offline**
  - Build an offline job/script (e.g. `ml/analyze_reconciliation_adjustments.py`) that:
    - Reads reconciliation logs (raw or via whatever log store you use).
    - Aggregates metrics per:
      - Model family (batting, bowling, innings, win),
      - Format (T20, ODI, etc.),
      - Time window (weekly, monthly).
    - Produces summaries:
      - Average adjustment magnitude,
      - Histograms of per-player deltas,
      - Persistent biases (e.g. models systematically over-predict batting runs).

### 3.2 Realism metrics vs historical distributions

- [x] **3.2.1 Generic distribution helpers**
  - `ml.harmony_metrics`:
    - `distribution_summary(x)`.
    - `realism_metrics_vs_historical(reconciled, historical)`.

- [ ] **3.2.2 Define realism metrics per stat**
  - For each of:
    - Runs per innings,
    - Wickets per innings,
    - Extras per innings,
    - Runs per wicket, wickets per innings (derived stats),
  - Define:
    - Allowed band for `delta_mean`, `delta_std`, `delta_p50` relative to historical.

- [ ] **3.2.3 Batch job to compute realism metrics**
  - New script: `ml/compute_harmony_realism_metrics.py`:
    - Input:
      - A table of reconciled outputs on historical matches or simulated outputs,
      - Reference historical distributions.
    - Uses `realism_metrics_vs_historical` to compute per-stat metrics.
    - Writes a JSON/CSV summary for dashboards and alarms.

### 3.3 Win coherence metrics (6.2)

- [x] **3.3.1 Coherence for final score margins**
  - `ml.win_coherence_metrics`:
    - `implied_win_probability_from_margin(margin, scale)`.
    - `win_probability_coherence_from_margin(p_model_team1, margin, scale)`.

- [ ] **3.3.2 Integrate with win model evaluation**
  - Extend win-model evaluation scripts to:
    - For held-out matches with known outcomes and reconciled scores:
      - Compute final margin.
      - Compute model win probability at “start of match” and at key checkpoints.
      - Compute coherence metrics and include them in evaluation reports.

- [ ] **3.3.3 Optional: feature for online monitoring**
  - For live or near-live predictions:
    - When a simulated distribution of outcomes is available, compute:
      - Empirical win probability from simulations.
      - Model win probability.
      - Coherence metric.
    - Log these alongside standard metrics for future dashboards.

---

## 4. Stage 3 – Joint / hierarchical modelling (high-level)

Goal: Move beyond independent models + reconciliation to **hierarchical models** where batting, bowling, and win predictions share latent structure (e.g. batting strength, bowling strength, pitch).

- [ ] **4.1 Define latent structure**
  - Decide on latent variables (e.g. team batting strength, team bowling strength, venue factor, pitch factor).
  - Map current features/stats to these latent factors.

- [ ] **4.2 Prototype generative model**
  - Start with a small format (e.g. T20):
    - Sample latent variables.
    - Generate ball-by-ball or over-by-over outcomes.
    - Derive per-player lines and team totals.
    - Compare realism vs current pipeline + reconciliation.

- [ ] **4.3 Evaluate vs decoupled + reconciliation**
  - Offline comparison across:
    - Predictive accuracy,
    - Realism metrics,
    - Reconciliation “effort” (how much fudge is needed).

---

## 5. API & rollout considerations

### 5.1 High-level APIs to Go app (Plan 8.2)

- [ ] **5.1.1 Define “generate match” ml-service API**
  - Endpoint: e.g. `POST /api/ml/generate-match`:
    - Input: match context + teams + format.
    - Output:
      - Reconciled scorecards (per-player stats, per-innings totals).
      - Win probability trajectory or at least start-of-match win probability.

- [ ] **5.1.2 Wire Go app to use reconciled outputs**
  - Update Go app:
    - To call the new high-level endpoint.
    - To display reconciled scorecards and win probabilities everywhere user-visible scores appear.
    - Preserve low-level per-model endpoints for research only.

### 5.2 Rollout strategy (Plan 10.x)

- [ ] **5.2.1 Offline prototype runs**
  - Run full reconciliation pipeline and harmony metrics on:
    - Historical matches with known outcomes.
    - Synthetic matches that stress corner cases (low scores, collapses, massive totals).

- [ ] **5.2.2 Shadow mode**
  - Run reconciled pipeline alongside current user-visible pipeline:
    - Log differences in scores, wins, and metrics.
    - Collect expert feedback.

- [ ] **5.2.3 Gradual adoption**
  - Enable reconciled outputs:
    - First on internal dashboards and/or limited segments.
    - Then roll out by format subset.
  - Keep:
    - Fallback path to old behavior.
    - Kill switch for reconciliation if constraints or performance regress unexpectedly.

---

## 6. How to use this plan

When implementing new work:

- Treat this document as the **implementation backlog** for model harmony.
- For each new PR:
  - Reference the relevant subsection(s) here.
  - Mark items as done in both this file and `model-harmony-plan.md` (where applicable).
- Prefer:
  - Adding focused, well-tested helpers (metrics, adapters, loss functions),
  - Then wiring them into existing flows (training_pipeline, auto_tune, evaluation scripts) with clear config flags and logging.

