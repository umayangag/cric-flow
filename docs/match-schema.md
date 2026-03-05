## Canonical match schema & deterministic constraints

This document defines the **canonical internal representation** of a cricket match and the **deterministic accounting rules** that all models and reconciliation logic must satisfy.

It is aligned with the Python types in `ml-service/ml/match_schema.py`:

- `BallEvent` — single delivery (legal or not)
- `InningsState` — one batting innings (aggregates a list of `BallEvent`s)
- `MatchState` — full match (one or more innings)

Downstream models (batting, bowling, fielding, extras, win, and innings) are free to make probabilistic predictions, but the **final reconciled scorecard** must obey the constraints in this document.

---

## 1. Canonical objects

### 1.1 `BallEvent`

Logical fields (matching `BALL_COLS` from `ml.ball_by_ball_loader`):

- **Identifiers**
  - `match_id: int`
  - `innings: int` — 1, 2 (or higher in multi-innings formats)
  - `over: int`
  - `ball: int`
  - `ball_seq: int` — monotonically increasing sequence within an innings
- **State**
  - `is_legal: bool` — `True` if this ball consumes one of the innings’ legal deliveries
  - `phase: str` — e.g. powerplay / middle / death
  - `striker_id: int`
  - `non_striker_id: int`
  - `bowler_id: int`
- **Runs and extras**
  - `runs_batter: int` — runs credited to the batter’s scorecard
  - `runs_extras: int` — runs credited as extras (wides, no-balls, leg-byes, byes, penalties)
  - `runs_total: int` — runs added to the team total on this ball
  - `extras_kind: Optional[str]` — raw extras type, mapped to `ExtrasKind` enum where possible
- **Dismissals**
  - `wicket_kind: Optional[str]` — raw dismissal type, mapped to `WicketKind`
  - `player_out_id: Optional[int]` — batter dismissed on this ball, if any

### 1.2 `InningsState`

- `match_id: int`
- `innings_number: int`
- `batting_team_id: Optional[int]`
- `bowling_team_id: Optional[int]`
- `target_runs: Optional[int]` — for chasing innings
- `balls_per_innings: Optional[int]` — format‑specific legal ball cap (e.g. 120 for T20)
- `balls: List[BallEvent]`

Derived properties (from `InningsState` implementation):

- `total_runs: int` — \(\sum_b \mathrm{runs\_total}_b\)
- `total_wickets: int` — number of balls with `player_out_id is not None`, capped at 10
- `legal_balls: int` — count of balls with `is_legal = True`
- `overs_bowled: (int, int)` — `(completed_overs, balls_in_current_over)` from `legal_balls`
- `extras_total: int` — \(\sum_b \mathrm{runs\_extras}_b\)
- `extras_breakdown: Dict[ExtrasKind, int]` — extras aggregated by type
- `is_all_out: bool` — `total_wickets >= 10`
- `innings_completed: bool` — `is_all_out` OR (`balls_per_innings` set and `legal_balls >= balls_per_innings`)

### 1.3 `MatchState`

- `match_id: int`
- `format_code: Optional[str]` — `TEST`, `ODI`, `T20`, `T20I`, etc.
- `venue_id: Optional[int]`
- `season_id: Optional[int]`
- `outcome_winner_team_id: Optional[int]`
- `innings_list: List[InningsState]`

Derived helpers:

- `innings_by_number: Dict[int, InningsState]`
- `team_totals: Dict[int, int]` — sum of `total_runs` per batting team across innings
- `team_wickets: Dict[int, int]` — sum of `total_wickets` per batting team across innings
- `is_two_innings_limited_overs: bool` — heuristic (ODI/T20‑style formats)

---

## 2. Deterministic accounting identities

All reconciled outputs must satisfy these identities.

### 2.1 Per‑ball identities

For every `BallEvent`:

- **Runs decomposition**
  - `runs_total = runs_batter + runs_extras`
  - `runs_total >= 0`, `runs_batter >= 0`, `runs_extras >= 0`

- **Legal vs illegal deliveries**
  - If `is_legal = True`, the ball **consumes** one of the innings’ legal deliveries.
  - If `extras_kind` is **wide** or **no‑ball**:
    - That ball does **not** consume a legal delivery:
      - `is_legal = False` for wides and front‑foot no‑balls.
    - It still contributes to:
      - `runs_extras` and `runs_total`.

Implementation detail: some historical datasets may encode certain no‑balls as `is_legal = True`. Reconciliation logic must still satisfy **ball count constraints** at the innings level (section 2.3).

### 2.2 Per‑innings score and wickets

Given an `InningsState`:

- **Team score decomposition**
  - `innings_score = total_runs`
  - `innings_score = sum_batsman_runs + extras_total`
    - `sum_batsman_runs` is the sum of per‑batsman runs in the batting scorecard.
  - When bowling figures are defined as including extras conceded:
    - `innings_score = sum_bowler_runs_conceded`
  - When bowling figures exclude some extras (e.g. byes/leg‑byes):
    - `innings_score = sum_bowler_runs_conceded + byes_leg_byes + penalty_runs`

- **Wickets**
  - `innings_wickets = total_wickets`
    - Computed as the number of `BallEvent`s with a non‑null `player_out_id`.
  - Team wickets must satisfy:
    - `0 <= innings_wickets <= 10`
  - In the reconciled scorecard:
    - `innings_wickets = sum_bowler_wickets` for the bowling side in that innings.
    - `innings_wickets = number_of_batsmen_out` for the batting side.

- **Batsmen vs bowlers vs team consistency**
  - For the batting team:
    - `sum_batsman_runs + extras_total = innings_score`
    - `number_of_batsmen_out = innings_wickets`
  - For the bowling team:
    - `sum_bowler_wickets = innings_wickets`
    - `sum_bowler_runs_conceded` relates to `innings_score` as per your chosen convention above.

These directly enforce the user requirements:

1. **Team total equals runs conceded by bowlers (plus extras, if defined that way)**.
2. **Wickets fallen equal wickets taken**.

### 2.3 Balls and overs

Within an `InningsState`:

- `legal_balls = count(is_legal = True)`
- `(completed_overs, balls_in_current_over) = divmod(legal_balls, 6)`
- Format‑specific caps (through `balls_per_innings`):
  - For T20: `balls_per_innings = 120`
  - For ODI: `balls_per_innings = 300`
  - For TEST (if used): `balls_per_innings` may be `None` (unlimited per innings).

At the **scorecard level**:

- **Batting balls faced**
  - `sum_balls_faced_by_batsmen` must equal `legal_balls`
    - Subject to the usual cricket caveats (e.g. certain no‑ball rules), but for model outputs we treat this as a **hard equality**.

- **Bowling balls bowled**
  - `sum_balls_bowled_by_bowlers = legal_balls`

Thus:

3. **Total balls faced by batsmen = total balls bowled by bowlers = legal_balls.**

### 2.4 Extras breakdown

Within an `InningsState`:

- `extras_total = sum(runs_extras over all balls)`
- Breakdown:
  - `wides = sum(runs_extras for balls with extras_kind_enum == WIDE)`
  - `no_balls = sum(runs_extras for extras_kind_enum == NO_BALL)`
  - `byes = sum(runs_extras for extras_kind_enum == BYE)`
  - `leg_byes = sum(runs_extras for extras_kind_enum == LEG_BYE)`
  - `penalty_runs = sum(runs_extras for extras_kind_enum == PENALTY)`
- Identity:
  - `extras_total = wides + no_balls + byes + leg_byes + penalty_runs`

Relationship to bowlers:

- Wides and no‑balls:
  - Always contribute to `extras_total`.
  - Also increase `bowler_runs_conceded` for the responsible bowler.

Thus:

4. **Total extras in the scorecard must match the wides/no‑balls/byes/leg‑byes/penalty runs implied by ball events and bowling figures.**

### 2.5 Innings termination conditions

Each `InningsState` must either:

- End because **all wickets fall**:
  - `innings_wickets = 10`
  - `innings_completed = True`
  - `legal_balls <= balls_per_innings` (never more than the maximum allowed)

or

- End because **all legal balls are used**:
  - `innings_completed = True`
  - `legal_balls = balls_per_innings`
  - `innings_wickets <= 10` (can be less than 10)

For the **second innings** in limited‑overs formats:

- The innings may end early when the chasing team reaches or exceeds the target:
  - Let `target_runs` be the first‑innings score + 1.
  - If `total_runs >= target_runs`, the chasing innings may terminate before:
    - `legal_balls = balls_per_innings` and/or
    - `innings_wickets = 10`.

These rules capture:

5. **Not all batsmen must bat** (innings can finish early without all out).  
6. **All wickets can fall before the over limit is reached** (innings ends with unused balls).

### 2.6 Hard vs soft classification

For reconciliation and simulation, constraints are classified as follows.

**Hard (must never be violated):**

- All per‑ball and per‑innings accounting identities in §2.1–2.5 (runs decomposition, wickets, balls, extras, termination).
- Match‑level runs/wickets symmetry and result determination (§3).
- Structural rules: player/team roles per ball (§4.1), bowling workload caps (§4.2), score progression monotonicity (§4.3).

The reconciliation layer encodes these as linear equalities (or bounds) and enforces them exactly.

**Soft (desirable but negotiable; encoded as penalties or monitoring):**

- **Batting order:** Prefer that top-order batsmen face more balls; implemented as higher weight on their preferred `balls_faced` in the objective (parameter `soft_favor_top_order_balls`).
- **Bowler spell lengths:** Prefer realistic distribution of overs across bowlers; can be added as variance penalties or priors (tunable strength).
- **Fielding vs dismissals:** Predicted catches/run-outs vs caught/run-out dismissal counts (§4.4); used as monitoring metrics or optional penalty terms.

Soft constraints are parameterized so their strength can be tuned from historical realism metrics without changing hard rules.

---

## 3. Match‑level constraints (two‑innings limited‑overs)

For limited‑overs formats with exactly two innings (e.g. T20, ODI):

- Let `Inn1` be the first innings, `Inn2` the second.
- Team assignments:
  - `Inn1.batting_team_id = team1`
  - `Inn1.bowling_team_id = team2`
  - `Inn2.batting_team_id = team2`
  - `Inn2.bowling_team_id = team1`

Match‑level identities:

- **Runs for/against symmetry**
  - `runs_for(team1) = Inn1.total_runs`
  - `runs_against(team1) = Inn2.total_runs`
  - `runs_for(team2) = Inn2.total_runs`
  - `runs_against(team2) = Inn1.total_runs`

- **Wickets for/against symmetry**
  - `wickets_for(team1) = Inn2.total_wickets`
  - `wickets_against(team1) = Inn1.total_wickets`
  - `wickets_for(team2) = Inn1.total_wickets`
  - `wickets_against(team2) = Inn2.total_wickets`

- **Result determination (ignoring ties/no result)**
  - If `Inn1.total_runs > Inn2.total_runs`:
    - `outcome_winner_team_id = Inn1.batting_team_id`
  - If `Inn2.total_runs > Inn1.total_runs`:
    - `outcome_winner_team_id = Inn2.batting_team_id`

These match‑level constraints must always hold for reconciled states in `MatchState`.

---

## 4. Additional structural and role constraints

These constraints go beyond pure score accounting and capture structural rules of the game. Some will be enforced as **hard** constraints in simulations; others may be treated as **soft** constraints or realism checks, depending on data quality.

### 4.1 Player and team roles

- For any `BallEvent` in an `InningsState`:
  - `striker_id` and `non_striker_id` belong to the batting team for that innings.
  - `bowler_id` belongs to the bowling team for that innings.
  - `striker_id != non_striker_id`.
  - `bowler_id` is not equal to `striker_id` or `non_striker_id` (no self‑bowling).
- Across an innings:
  - The set of unique batting player IDs used as `striker_id`/`non_striker_id` has size ≤ 11.
  - The set of unique bowling player IDs used as `bowler_id` has size ≤ 11.
- Across a match:
  - Each player ID appears for exactly one team (no player switching teams mid‑match).

These constraints ensure that any ball‑by‑ball simulator or reconciled state is compatible with a valid playing XI for each team.

### 4.2 Bowling workload and overs per bowler

For limited‑overs formats with `balls_per_innings` set:

- Define:
  - `total_overs = balls_per_innings / 6`.
  - `max_overs_per_bowler = total_overs / 5`.
    - Example:
      - T20: `balls_per_innings = 120` → `total_overs = 20`, `max_overs_per_bowler = 4`.
      - ODI: `balls_per_innings = 300` → `total_overs = 50`, `max_overs_per_bowler = 10`.
- For each bowler:
  - `legal_balls_bowled_by_bowler <= max_overs_per_bowler * 6`.
  - `legal_balls_bowled_by_bowler >= 0`.

At reconciliation time, the sum of per‑bowler deliveries still must satisfy:

- \(\sum_j \mathrm{deliveries}_j = \mathrm{legal\_balls}\),
- and each bowler’s share must respect the `max_overs_per_bowler` cap implied by the format.

### 4.3 Score progression

Within an innings, let:

- `R(k)` be cumulative runs after the `k`‑th ball (in `ball_seq` order),
- `W(k)` be cumulative wickets after the `k`‑th ball.

Then:

- `R(k+1) >= R(k)` for all `k` (team score is non‑decreasing).
- `W(k+1) >= W(k)` for all `k` (wickets are non‑decreasing).

Any ball‑by‑ball generator must maintain these monotonicity properties. At the aggregate level, these serve as checks on any reconstructed ball‑by‑ball path.

### 4.4 Fielding and dismissal relationships

The fielding model predicts per‑player:

- `catches`, `run_outs` (and potentially other fielding actions).

Dismissals in `BallEvent` encode:

- `wicket_kind_enum` ∈ {CAUGHT, RUN_OUT, STUMPED, etc.}

Consistency requirements (typically **soft** constraints, monitored or used as penalties):

- The number of **caught** dismissals in an innings should be compatible with total predicted catches:
  - `sum_fielding_catches` should be close to `number_of_caught_dismissals`.
- The number of **run‑out** dismissals should be compatible with total predicted run‑outs:
  - `sum_fielding_run_outs` should be close to `number_of_run_out_dismissals`.

These can be enforced as:

- Monitoring metrics (to detect model drift), and/or
- Soft penalties in the reconciliation objective when integrating fielding predictions with batting/bowling scorecards.

---

## 5. Model‑level consistency requirements

The ML models (batting, bowling, fielding, extras, win, innings) are trained separately, but their **outputs must be reconciled** so that the final `MatchState` obeys this spec.

High‑level consistency requirements:

- **Batting model outputs**
  - Per‑batsman:
    - `runs_scored`, `balls_faced`, fours, sixes, out/not‑out.
  - At reconciliation:
    - \(\sum_i \mathrm{runs\_scored}_i + \mathrm{extras\_total} = \mathrm{innings\_score}\).
    - \(\sum_i \mathrm{balls\_faced}_i = \mathrm{legal\_balls}\).

- **Bowling model outputs**
  - Per‑bowler:
    - `runs_conceded`, `deliveries` (balls), `wickets_taken`, economy.
  - At reconciliation:
    - \(\sum_j \mathrm{runs\_conceded}_j\) matches the innings score under the chosen convention.
    - \(\sum_j \mathrm{wickets\_taken}_j = \mathrm{innings\_wickets}\).
    - \(\sum_j \mathrm{deliveries}_j = \mathrm{legal\_balls}\).

- **Extras model outputs**
  - `total_extras`, optionally a breakdown.
  - At reconciliation:
    - `total_extras = extras_total` from `InningsState`.
    - If the model predicts a breakdown, it must respect:
      - `total_extras = wides + no_balls + byes + leg_byes + penalty_runs`.

- **Innings model outputs**
  - `innings_runs`, `innings_wickets` for each innings.
  - At reconciliation:
    - `innings_runs = total_runs`, `innings_wickets = total_wickets` in `InningsState`.
    - Batting and bowling aggregates must be scaled to hit these targets.

- **Win model outputs**
  - `team1_win_probability` (or equivalent).
  - At reconciliation:
    - Features supplied to the win model must be derived from a **consistent** `MatchState` (or projected variant).
    - Optional: implied win probability from simulated score distributions should be close to the win model’s output (used as a monitoring metric, not a hard constraint).

---

## 6. Usage notes

- This document is the **single source of truth** for cricket accounting rules in the system.
- The reconciliation layer and simulator should:
  - Accept unconstrained model outputs as “preferred values”.
  - Solve for a `MatchState` that:
    - Satisfies all **hard constraints** above, and
    - Minimizes deviation from the preferred values, subject to realism priors.
- Any changes to:
  - How bowling figures treat extras, or
  - How balls/no‑balls/wides are encoded
  must be reflected both here and in `ml-service/ml/match_schema.py`.

