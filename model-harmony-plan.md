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

