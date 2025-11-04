# Algorithms Baseline — Derived from Prototype (Initial Draft)

Status: In progress. This baseline summarizes the prototype’s algorithms in `src/` as formulas/pseudocode for parity validation. It focuses on player form, consistency, encodings, team selection/combination, and final team prediction.

Sources (prototype)
- `src/team_selection/select_pool.py`
- `src/team_selection/shared/*` (e.g., `match_data.py`, `metrics.py` if present)
- `src/team_selection/create_final_dataset.py`
- `src/final_data/*` (batting/bowling regressors and team win predictor)

Note: The prototype uses MySQL-style schemas; semantics carry to the implementation (Postgres). Numeric rounding differences are acceptable within small tolerance.

---

## Feature Inputs (expected by ML)
See also `ml-service/ml/dataset_definitions.py` for canonical column sets.

Batting inputs (`input_batting_columns`)
1. batting_consistency (float)
2. batting_form (float)
3. batting_temp (int)
4. batting_wind (int)
5. batting_rain (int)
6. batting_humidity (int)
7. batting_cloud (int)
8. batting_pressure (int)
9. batting_viscosity (int: 0=dry,1=humid)
10. batting_inning (int: 1|2)
11. batting_session (int: 1|2|3)
12. toss (int: 0|1)
13. venue (float)
14. opposition (float)
15. season (int)
16. player_name (str)

Bowling inputs (`input_bowling_columns`)
1. bowling_consistency (float)
2. bowling_form (float)
3. bowling_temp (int)
4. bowling_wind (int)
5. bowling_rain (int)
6. bowling_humidity (int)
7. bowling_cloud (int)
8. bowling_pressure (int)
9. bowling_viscosity (int: 0=dry,1=humid)
10. batting_inning (int: 1|2)
11. bowling_session (int: 1|2|3)
12. toss (int: 0|1)
13. bowling_venue (float)
14. bowling_opposition (float)
15. season (int)
16. player_name (str)

Encodings (prototype semantics)
- session → encode_session(session_text) ∈ {1,2,3}
- viscosity → encode_viscosity(text): dry→0, humid→1, NULL→0
- toss (text) → numeric {0,1} (semantics: batting-first/field-first; see exporter mapping)

---

## Player Form and Consistency

Definitions are inferred from usage in `select_pool.py` and related scripts:

- Consistency (stored):
  - `player.batting_consistency` and `player.bowling_consistency` are precomputed aggregates (range [0,1] or model-specific scaling). Players with both consistencies 0 are excluded from pool.

- Form (rolling/seasonal):
  - For match with `season_id = S`, form uses prior season S-1 for the same skill:
    - batting_form = get_player_metric(match_id, "batting", player, "form", dim="season", key=S-1)
    - bowling_form = get_player_metric(match_id, "bowling", player, "form", dim="season", key=S-1)
  - Fallbacks: if missing, default to 0 (or global/position-specific prior mean) — prototype generally treats missing as 0 before model input.

- Venue/Opportunistic aggregates:
  - batting_venue = get_player_metric(match_id, "batting", player, "venue", dim="venue", key=venue_id)
  - batting_opposition = get_player_metric(match_id, "batting", player, "opposition", dim="opposition", key=opposition_id)
  - bowling counterparts analogous.
  - Aggregates are numeric (e.g., averages or normalized performance scores) in [0, 1] or model scale.

Pseudocode
```
for each player in candidate_list:
  # prior season form
  bf = batting_form(player, season_id-1) or 0
  bv = batting_venue(player, venue_id) or 0
  bo = batting_opposition(player, opposition_id) or 0

  features_bat = [
    player.batting_consistency,
    bf,
    weather.temp, weather.wind, weather.rain, weather.humidity, weather.cloud, weather.pressure,
    encode_viscosity(weather.viscosity),
    match.inning,
    encode_session(match.batting_session),
    encode_toss(match.toss),
    bv,
    bo,
    season_id,
    player.name,
  ]
```

Bowling analogous with `bowling_*` fields; note `batting_inning` is used in bowling input.

---

## Predictions (Per-player)

- Batting regressor: `predict_batting(features_bat)` → outputs:
  - runs_scored, balls_faced, fours_scored, sixes_scored, batting_position
  - Derived: strike_rate = 100 * runs_scored / max(1, balls_faced); batting_contribution based on weighted components

- Bowling regressor: `predict_bowling(features_bowl)` → outputs:
  - runs_conceded, deliveries, wickets_taken
  - Derived: econ = 6 * runs_conceded / max(1, deliveries); bowling_contribution weighted

- Merge by `player_name` to create a pool with both batting and bowling predictions (missing values filled via `fill_missing_attributes`).

---

## Team Selection & Combination

High-level flow (from `select_pool.py` and helpers):
1. Pool creation:
   - Filter players where `is_retired=0` and `(batting_consistency != 0 or bowling_consistency != 0)`.
   - Identify wicket keepers: `is_wicket_keeper == 1`.
   - Identify bowlers: `bowling_consistency > 0`.
2. Predict per-player batting and bowling performance using features above.
3. Rank and select XI:
   - Primary ranking: `winning_probability` (after team win computation; see below) or composite contribution.
   - Constraints:
     - At least one wicket keeper in XI
     - Sufficient number of bowlers (domain-specific threshold; often ≥4 in limited overs)
     - Positions may be guided by predicted batting_position
   - Tie-breakers: deterministic order by individual contribution, then name.
4. Combinations (if enumerated):
   - Generate combinations of size 11 from candidate pool, apply constraints, evaluate win probability for each, choose best.
   - Prototype sometimes shortcuts by sorting top-N by individual metrics to prune search.

---

## Final Team Prediction

- Function: `predict_for_team(team_frame_without_names)` in `final_data.match_win_predict`.
- Inputs: per-player predictions (without `player_name`, possibly without `season` and `match_number` per prototype snippet).
- Output:
  - `players` with `winning_probability` (per-player contribution to win)
  - `team_win_probability` (overall chance of winning)
- The prototype computes this by aggregating per-player features into team-level features (e.g., sum/avg of runs, wickets, econ) and feeding into a meta-model or heuristic.

---

## Acceptance Tolerances

- Feature parity: exact match, with float tolerance ≤1e-6 due to encoding/casting differences.
- Predictions: identical if same artifacts are used; if retrained, allow small relative deltas (e.g., ±1–3%).
- Team selection: identical XI for golden cases or documented differences with explicit tie-breakers.

---

## Open Clarifications
- Exact toss encoding: confirm mapping (e.g., bat-first=1 vs field-first=0); exporter currently maps text→int — must match prototype’s assumption.
- Whether form is strictly `season-1` or includes rolling window within season; prototype usage implies `season-1`.
- Minimum number of bowlers in selection constraints for each format.
