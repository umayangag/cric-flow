## Optimum team from Monte Carlo – TODO

This doc tracks the work to make team selection directly optimize for Monte Carlo win probability and expose richer distributions in the UI.

### Goals

- **Decision objective**: define and implement a clear optimization target for team selection based on Monte Carlo simulation (e.g. maximize simulated win probability, or expected utility that trades off win prob vs risk).
- **Backend support**: extend `predictteam` so it can (optionally) choose XIs using the Monte Carlo objective instead of the current score-weights objective.
- **Frontend support**: let users toggle between “score-based best XI” and “Monte Carlo‑optimized XI”, and surface the simulation details used for the decision.

### Open design questions

- **Optimization target**
  - Should “best XI” be: highest simulated win probability, highest median innings total, or a risk‑adjusted function (e.g. maximize P(win) subject to P(total < T) ≤ ε)?
  - Do we optimize over:
    - A single XI per team (current mental model), or
    - A *portfolio* of XIs (e.g. top‑k ranked by simulation) with tie‑breaking by score weights?
- **Constraints and fairness**
  - Same constraints as today (min bowlers, keeper, etc.), or new ones (e.g. max players from a league/team)?
  - How to keep runtime reasonable as pool size grows (top‑k per team, max matchups, early stopping)?
- **Explainability**
  - What minimal set of numbers/visuals should the UI show so users understand *why* a Monte Carlo‑optimized XI is preferred (e.g. “+6% win prob vs score‑only XI, similar median total, tighter tail”)?

### Proposed backend tasks

- **T1 – Factor out simulation core for reuse**
  - Extract a pure function from `PredictTeamsWithSimulation` that, given:
    - a fixed team selection function,
    - player predictions and extras,
  - returns per‑XI metrics (simulated win prob, mean/percentile totals) without coupling to HTTP or current “top‑k” selection.

- **T2 – Per‑XI metrics**
  - For each candidate XI in `topK1` / `topK2`, compute:
    - `p_win`, `p_loss`, `p_draw` vs a representative set of opponent XIs,
    - `innings_total_p10/p50/p90`.
  - Add a (configurable) cap on:
    - number of XIs per team,
    - number of matchups and samples.

- **T3 – Monte Carlo–optimized XI**
  - Implement a new selector that:
    - starts from the current score‑based top‑k XIs per team,
    - runs simulation for those XIs,
    - picks the XI that maximizes the chosen objective (e.g. `p_win`), breaking ties with existing score weights.
  - Expose this via a new option on the team‑selection API, e.g. `selection_mode=score|monte_carlo`.

- **T4 – API and DTOs**
  - Extend `POST /api/predict/team-selection` to:
    - accept `selection_mode`,
    - when `selection_mode=monte_carlo`, return:
      - the Monte Carlo–optimized XI,
      - both score‑based and Monte Carlo metrics (so users can compare),
      - optional per‑XI summary (e.g. “this XI: 62% win prob; baseline: 55%”).

### Proposed frontend tasks

- **F1 – Upcoming Match tab: selection mode**
  - Add a toggle or select for `Best XI based on: Score | Monte Carlo win probability`.
  - When Monte Carlo mode is selected:
    - call the updated API with `selection_mode=monte_carlo` and `simulate=true`,
    - clearly label the returned XI as “Monte Carlo‑optimized”.

- **F2 – Simulation visualization**
  - For the selected XI, show:
    - win prob vs baseline XI,
    - innings total distribution (P10/P50/P90) for both teams.
  - Optionally add a compact “risk profile” indicator (e.g. low/medium/high downside risk vs baseline).

- **F3 – Evaluate DB: real distributions**
  - Replace the current assumed‑CV distribution in the Evaluate DB players table with:
    - real simulation‑based percentiles when available,
    - a clear “simulated” vs “assumed” indicator.

### Non‑goals (for now)

- Jointly re‑tuning the win model and simulation parameters in one loop.
- Enumerating *all* possible XIs for large pools (keep top‑k + caps).
- Adding full per‑player Monte Carlo distributions to the API (focus first on team‑level metrics).

