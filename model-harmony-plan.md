## Model Harmony & Consistency Plan

This document tracks the plan to align win prediction, batting, bowling, fielding, extras, and any other performance models so that the final simulated scorecards are both **cricket‑legal** and **statistically coherent**.

We will mark items as we implement them: `[ ]` = pending, `[x]` = done.

---

## 1. Canonical match representation & constraints

- [x] **1.1 Define canonical match schema**
  - [x] Choose the primary resolution for the internal state (ball‑by‑ball vs over‑by‑over vs per‑innings) and encode it in a shared schema (e.g. `MatchState`, `InningsState`, `BallEvent` or equivalent).
  - [x] Ensure the schema can derive all downstream quantities: team totals, per‑batsman lines, per‑bowler figures, extras breakdown, and match result.

- [x] **1.2 Enumerate and formalize deterministic constraints**
  - [x] Write down all hard accounting identities, including:
    - [x] Total team score equals batsman runs plus extras, and equals runs conceded by bowlers (consistent with our definition of “conceded”).
    - [x] Total wickets fallen by the batting team equals total wickets taken by bowlers in that innings (capped at 10).
    - [x] Total legal balls faced by batsmen equals total legal balls bowled (subject to wides/no‑ball rules).
    - [x] Total extras decompose into wides, no‑balls, leg‑byes, byes, and penalty runs; wides/no‑balls behavior matches format rules.
    - [x] Innings termination conditions (all out, overs exhausted, or target reached in the second innings).
  - [x] Capture these constraints in a single reference document (e.g. `docs/match-schema.md`) so all components agree.
  - [x] Document additional structural constraints (player/team roles per ball, max overs per bowler by format, score monotonicity, and fielding vs dismissal relationships) that future reconciliation and simulation must respect.

---

## 2. Master source of truth & simulator

- [x] **2.1 Choose the master representation**
  - [x] Decide whether the master source of truth is a ball‑by‑ball simulator, an over‑by‑over state model, or a per‑innings generator, and document the choice and rationale.
  - [x] Make this master representation the only thing that directly produces full scorecards; all other projections should be derived from it.

- [x] **2.2 Define derived projections**
  - [x] Specify how per‑batsman projections are aggregated from the master state.
  - [x] Specify how per‑bowler projections are aggregated from the master state.
  - [x] Specify how extras (including wides and no‑balls) are aggregated from the master state.
  - [x] Specify how win probability is computed from the same underlying state (or how a separate win model consumes features from it).

---

## 3. Reconciliation layer between models and simulator

- [x] **3.1 Treat individual models as unconstrained forecasters**
  - [x] Define the exact outputs from each existing model (batting, bowling, extras, win, fielding, etc.) as “preferred” or “target” values (μ).
  - [x] Version and document these preferred outputs so the reconciliation layer can consume them in a stable format.

- [x] **3.2 Express cricket rules as constraints over variables**
  - [x] Define a vector of variables \(x\) that includes team totals, per‑player stats, and extras components.
  - [x] Encode cricket accounting rules as linear (or near‑linear) equality/inequality constraints \(C x = 0\) and bounds (e.g. no negative runs, balls, or wickets; wickets ≤ 10).

- [x] **3.3 Define a deviation cost from model suggestions**
  - [x] Define a quadratic (or absolute) penalty for deviations between reconciled values \(x\) and model suggestions \(μ\), with per‑component weights \(W\) that reflect confidence in each model.
  - [x] Decide how to weight different sub‑models (e.g. win vs batting vs bowling vs extras) when they disagree.

- [x] **3.4 Implement constrained optimization / heuristic solver**
  - [x] Implement a solver that minimizes \((x - μ)^T W (x - μ)\) subject to cricket constraints.
  - [x] Ensure the solver enforces integer constraints where needed (runs, wickets, balls) either via integer programming or robust rounding heuristics.
  - [x] Wrap the solver behind a clear API (e.g. `reconcile_innings(...)`) to produce a globally consistent scorecard.

---

## 4. Hard vs soft constraints

- [x] **4.1 Classify constraints**
  - [x] Tag each constraint as **hard** (must never be violated) or **soft** (desirable but negotiable).
  - [x] Ensure all structural cricket rules (runs tally, wickets, balls, overs caps, innings termination) are treated as hard constraints.

- [x] **4.2 Incorporate soft constraints into the objective**
  - [x] Encode realism preferences (e.g. top order facing most balls, bowlers with realistic spell lengths) as penalties in the objective function rather than hard constraints.
  - [x] Parameterize the strength of these penalties so they can be tuned based on historical realism metrics.

---

## 5. Innings termination & partial batting

- [x] **5.1 Model which batsmen actually bat**
  - [x] Introduce variables to represent whether each batsman appears at the crease in an innings.
  - [x] Add constraints linking “played” flags to non‑zero runs/balls, and soft priors favoring top‑order batsmen to play more often.

- [x] **5.2 Encode innings end conditions (all out vs overs used)**
  - [x] Add variables/flags for all‑out vs overs‑completed endings.
  - [x] Enforce that:
    - [x] If all‑out, wickets = 10 and legal balls ≤ format maximum.
    - [x] If not all‑out, legal balls = format maximum and wickets ≤ 9.
  - [x] Ensure that the reconciled balls bowled by bowlers and balls faced by batsmen agree with the chosen termination condition.

---

## 6. Win prediction integration

- [x] **6.1 Standardize features for win prediction**
  - [x] Define the exact feature set for win prediction models, sourced from the reconciled match state (current or projected).
  - [x] Update the win prediction model (or its input pipeline) to consume only reconciled, constraint‑consistent features.

- [ ] **6.2 Align win probabilities with score distributions (optional)**
  - [ ] If we model score distributions or simulate matches, define how to compute implied win probabilities from those distributions.
  - [ ] Add a consistency metric or loss term that penalizes large discrepancies between implied win probabilities and the win model’s predictions.

---

## 7. Training‑time strategy

- [x] **7.1 Stage 1 – Inference‑only reconciliation**
  - [x] Keep current models as‑is, but run reconciliation only at inference time to produce consistent scorecards.
  - [x] Evaluate how much adjustment is applied to each component (batting, bowling, extras, win) to achieve consistency.

- [ ] **7.2 Stage 2 – Consistency‑aware training**
  - [ ] Introduce training losses that penalize large reconciliation adjustments, encouraging models to produce near‑consistent outputs by default.
  - [ ] Where feasible, enforce simple linear constraints directly in model outputs (e.g. normalized shares that sum to 1 for ball allocations).

- [ ] **7.3 Stage 3 – Hierarchical / joint models (optional)**
  - [ ] Design a hierarchical generative model that couples batting and bowling projections via shared latent variables (e.g. batting strength, bowling strength, pitch).
  - [ ] Prototype and compare this joint approach against the decoupled‑plus‑reconciliation pipeline before committing.

---

## 8. Implementation in current stack

- [x] **8.1 Implement reconciliation & simulator in `ml-service`**
  - [x] Add Python data models for the canonical match schema (using dataclasses or pydantic).
  - [x] Implement the constraint builder and optimization logic in the ML layer.
  - [x] Add unit tests to validate that reconciled outputs always satisfy cricket constraints on synthetic and historical cases.

- [ ] **8.2 Expose high‑level APIs to Go app**
  - [ ] Define a “generate innings/match” API that returns a fully reconciled scorecard and win probabilities.
  - [ ] Update the Go API layer to consume only reconciled outputs for any user‑facing scorecards or dashboards.

- [x] **8.3 Preserve low‑level model endpoints for research**
  - [x] Keep or expose per‑model debug endpoints (`batting`, `bowling`, `extras`, `win`) for experimentation.
  - [x] Document how to compare raw vs reconciled outputs to diagnose model biases.

---

## 9. Test coverage, CI gates, and training CLIs

- [x] **9.1 Component‑level coverage gates**
  - [x] Treat coverage thresholds as **per‑component quality bars**:
    - [x] Frontend: thresholds live in `frontend/vite.config.ts` under `test.coverage` (lines, branches, functions, statements). Raise them to the floor of current coverage when we improve tests.
    - [x] Go app: thresholds are `COV_MIN` in `go-app/Makefile` and `COV_MIN_GO` in the root `Makefile`, plus the `COV_MIN` env in `.github/workflows/go-app-ci.yml`. Only raise, never lower.
    - [x] ML service: threshold is `COV_MIN` in `ml-service/Makefile`, mirrored as `COV_MIN_ML` in the root `Makefile` and the `COV_MIN` env in `.github/workflows/ml-service-ci.yml`. Keep all three in sync.

- [x] **9.2 What counts toward ML‑service coverage**
  - [x] Coverage gates for ML service focus on **inference‑time and service code paths** (`app.*`, `ml.*` that power prediction, reconciliation, and APIs).
  - [x] Long‑running offline training and tuning entrypoints are **excluded from coverage accounting** via `pyproject.toml`:
    - [x] `ml/train_*.py` (per‑model training CLIs),
    - [x] `ml/training_pipeline.py`,
    - [x] `ml/tuning/*.py`,
    - [x] `ml/walk_forward.py`,
    - [x] `ml/validate_exports.py`.
  - [x] These scripts are still tested via focused unit tests and integration/e2e runs, but they do not block the core ML‑service coverage gate.

- [x] **9.3 Node 25 / Vitest behavior**
  - [x] To ensure Vitest’s `jsdom` environment works correctly with Node ≥25, two changes are made:
    - [x] Test scripts in `frontend/package.json` set `NODE_OPTIONS=--no-webstorage` to disable Node’s native Web Storage API, preventing conflicts with JSDOM.
    - [x] To support parallel test execution, `frontend/vite.config.ts` is updated to provide each test worker with a unique local storage file via the `--localstorage-file` argument.

---

## 9. Monitoring, diagnostics & realism metrics

- [x] **9.1 Automated consistency checker**
  - [x] Implement a checker that re‑evaluates all constraints on any produced scorecard and flags violations (pre‑ and post‑reconciliation).
  - [x] Log magnitude and direction of reconciliation adjustments for each component to a central store.

- [ ] **9.2 Harmony & realism metrics**
  - [ ] Define metrics for adjustment magnitude (e.g. mean absolute percentage changes per player/stat).
  - [ ] Define realism metrics by comparing distributions of reconciled outputs to historical data (e.g. runs per wicket, wickets per innings, extras per innings).
  - [ ] Define a metric for coherence between win probabilities and required‑runs/balls states.

- [ ] **9.3 Use metrics to guide model improvements**
  - [ ] Add dashboards or reports that highlight where reconciliation consistently pulls a given model in one direction.
  - [ ] Feed these insights back into model calibration and feature engineering.

---

## 10. Rollout & iteration plan

- [ ] **10.1 Prototype & offline evaluation**
  - [ ] Build a prototype reconciliation pipeline and run it against historical matches and current model outputs.
  - [ ] Compare raw vs reconciled vs ground‑truth scorecards for realism and consistency.

- [ ] **10.2 Shadow mode in production**
  - [ ] Run the reconciled pipeline alongside the existing pipeline without changing user‑visible outputs.
  - [ ] Collect logs and expert feedback (cricket experts / internal users) on differences between the two.

- [ ] **10.3 Gradual adoption**
  - [ ] Enable reconciled outputs for a subset of formats or users while keeping a fallback to the current behavior.
  - [ ] Once stable, make reconciled outputs the default while retaining a kill switch and continuing to monitor metrics.
-
---

## 11. Technical debt remediation plan (hierarchical)

This section tracks the implementation-focused work to harden the reconciliation stack and its integration with the existing prediction pipeline. Items here are more granular and code-oriented than the conceptual plan above.

- [ ] **11.1 Reconciliation core robustness (TD2, TD3)**
  - [ ] **11.1.1 Robust rounding in `ml.reconciliation_service` (TD2)**
    - [ ] Generalize `_sum_preserving_round` to handle both positive and negative deltas (target sums above or below current totals) without assertions.
    - [ ] Ensure `_adjust_team_stat` uses `_sum_preserving_round` only as a small, numerically stable correction step, not as a substitute for missing constraints.
    - [ ] Add targeted unit tests for `_sum_preserving_round` covering:
      - [ ] Positive delta (sum of values below target).
      - [ ] Negative delta (sum of values above target).
      - [ ] Edge cases with zeros and single-player teams.
  - [ ] **11.1.2 Move balls constraints into QP (`ml.reconciliation_core`) (TD3)**
    - [ ] Extend `ProblemBuilder` to encode preferred legal balls as hard constraints:
      - [ ] Batting balls for the batting team per innings sum to `preferred_legal_balls`.
      - [ ] Bowling balls for the bowling team per innings sum to `preferred_legal_balls`.
    - [ ] Keep `_sum_preserving_round` as a safety net only, verifying that deltas are typically small when constraints are present.
    - [ ] Update reconciliation tests to validate balls constraints at both continuous (QP) and integer (rounded) levels.

- [ ] **11.2 Prediction service layering & configuration (TD1, TD4, TD5, TD6)**
  - [ ] **11.2.1 Centralize prediction defaults in `ml.config` (TD4)**
    - [ ] Define a structured config for reconciliation-related defaults (e.g. `default_economy`, `bowling_deliveries_by_format`).
    - [ ] Expose a single `get_prediction_defaults()` accessor with validation and optional memoization.
    - [ ] Remove magic numbers (e.g. default overs) from `app.prediction_service` and route them through `ml.config`.
  - [ ] **11.2.2 Introduce a reconciliation adapter in `ml.*` (TD1/TD6)**
    - [ ] Move construction of `MatchReconciliationInputs` from `BacktestPlayerPred` plus innings totals into a dedicated adapter module.
    - [ ] Have the adapter call `reconcile_match_players`, `check_reconciled_scorecard_consistency`, and adjustment-magnitude helpers in one place.
    - [ ] Return reconciled stats and a structured metrics payload that `prediction_service` can log or surface.
  - [ ] **11.2.3 Thin `app.prediction_service` orchestration layer (TD1, TD5)**
    - [ ] Refactor `predict_players_with_features` to delegate reconciliation work to the adapter.
    - [ ] Standardize reconciliation logging fields (e.g. `applied`, mean deltas, violations) for downstream dashboards.
    - [ ] Clarify and centralize behavior when reconciliation modules are not available (ImportError-based fallbacks).

- [ ] **11.3 Future enhancements and win-model alignment (Plan §6–7, §9)**
  - [ ] **11.3.1 Extend constraints beyond runs/wickets/balls**
    - [ ] Design and gradually add constraints for extras breakdown, per-bowler over caps, and innings termination flags, ensuring they are expressed in `ProblemBuilder` rather than ad-hoc corrections.
    - [ ] Keep these constraints versioned and documented in `docs/match-schema.md`.
  - [ ] **11.3.2 Align win prediction with reconciled match state**
    - [ ] Define a canonical helper to transform reconciled `MatchState` into `WinFeatures`.
    - [ ] Add tests comparing implied and model win probabilities where simulators or score distributions are available.
  - [ ] **11.3.3 Metrics and observability**
    - [ ] Wire adjustment magnitude and violation summaries into structured logs.
    - [ ] Use these metrics for harmony/realism dashboards and to guide future model calibration work.
