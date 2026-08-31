> **Status (2026-08-31).** Written before the repo was inspected, as a first-principles plan from the dataset. Sections 0-3 are implemented in spirit by `ml-service/ml/xi/` (S-10 in [WIN_PROB_SELECTION_PR_CHECKLIST.md](WIN_PROB_SELECTION_PR_CHECKLIST.md)); the ball-outcome simulator, distributional performance models and evaluation harness of sections 3-6 are carried forward by [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md).

# Optimal XI Selection — ML Plan (Cricsheet dataset)

## 0. What the data actually gives us (verified on the folder)

- 22,734 match JSON files (Cricsheet v1.2.0), 2001–2026, ~7 GB. Every file is ball-by-ball.
- Mix: T20 ≈ 60% (3.5k men's intl T20, 2.1k women's intl T20, IPL 1.2k, BBL, PSL, CPL, BPL, SA20, county T20 etc.), ODI/List-A ≈ 20%, Test/first-class ≈ 15%.
- Per match: teams, venue, city, dates, season, event/stage, toss (winner + decision), outcome (winner, margin, D/L `method`, `result: no result/tie`, `eliminator`), player_of_match, the named squads (11, or 12 in IPL impact-player seasons), officials, and a `registry` mapping every name to a stable person ID. **Always key on the registry ID, not the name.**
- Per delivery: batter, non-striker, bowler, runs (batter / extras / total), extras breakdown (wides, noballs, byes, legbyes, penalty), wickets (kind, player_out, fielders), reviews, `replacements` (IPL impact player), powerplay ranges, target for chasing side.
- **Not in the data** (must be inferred or brought in externally): player roles (batter/bowler/keeper), bowling style (pace/spin, arm), batting hand, line/length, fielding positions, injuries/availability, weather. Afghanistan men's matches (374) are withheld.
- Sample-based housekeeping numbers: ~7% of matches are "no result", ~4% decided by D/L, super overs exist, ~450+ distinct venues in a 1.5k sample (expect 1,000+ overall, long tail of tiny venues).

Two facts drive the whole design:

1. **Selection is counterfactual.** We never observe the same team on the same day with a different XI. A model that learns "team X wins when players {a,b,c} are listed" memorises squads and cannot rank unseen combinations. The win-probability model must therefore be *compositional*: built from player-level and pair-level representations, so any 11-set can be scored.
2. **Roles must be inferred**, because combination quality is mostly about role coverage (a keeper, 5+ real bowling options, death bowlers, a finisher), and the dataset doesn't label roles.

## 1. Data pipeline

1. Parse all JSON → three Parquet tables:
   - `matches`: match_id, date, format (T20/ODI/Test/multi-day), gender, team_type, competition, venue_id (normalised: strip ", city" variants, map aliases), city, teams, toss_winner, toss_decision, winner, margin, method, result flag, target, player_of_match.
   - `lineups`: match_id, team, player_id, batting_position (order of first appearance), overs_bowled, is_replacement/impact player.
   - `balls`: match_id, innings, over, ball_in_over, legal_ball flag, batter_id, non_striker_id, bowler_id, runs_batter, extras by type, wicket_kind, player_out_id, fielder_ids, plus *derived match state*: cumulative score, wickets down, balls remaining, required runs / RRR (2nd inns), phase (powerplay / middle / death by format).
2. Cleaning rules: drop `no result` from the outcome model (keep their balls for player stats); keep D/L results with a winner; treat ties/super-over as 0.5 label or drop; handle 12-man IPL lists via `replacements`.
3. **Model per format** (at minimum T20 vs ODI vs long-form) and per gender. Start with men's T20 — it has the most matches, the most fixed match length, and the most obvious selection problem.
4. All player features are computed **as-of the match date** (strictly from earlier matches), stored as a time-indexed feature store. This is the single biggest leakage risk; do it once, correctly.

## 2. Feature extraction

### 2.1 Role inference (per player, per format, rolling)
- `bat_pos_dist`: distribution of batting positions (median, IQR) → opener / top / middle / finisher / tail.
- `overs_bowled_per_match`, `pct_matches_bowled`, `bowling_phase_share` (which over numbers they usually bowl) → frontline / part-time / non-bowler; new-ball / middle / death specialist.
- `keeper_flag`: takes stumpings, or credited "caught" while never bowling and with byes attributed to their side (heuristic; can be hand-corrected).
- `bowl_type_pace_vs_spin` (inferred): spinners have higher stumping share, lower wide/no-ball rate, bowl overs 7–15 more; cluster on these, or import an external style table keyed by Cricsheet ID (recommended — big accuracy gain for cheap).

### 2.2 Player skill features (rolling windows: last 10 / 25 / 50 matches, plus exponentially-weighted with ~1-year half-life; each with a sample-size column for shrinkage)
Batting: runs per ball, dismissal rate per ball (the true "average" driver), boundary %, dot %, runs per innings, balls faced per innings, 30+/50+ rate, phase-split SR & dismissal rate (PP / middle / death), 1st vs 2nd innings, SR under pressure (RRR > 9), performance vs inferred pace / spin, at venue, at home/away, against this opposition.
Bowling: economy, balls per wicket, dot %, boundary conceded %, wides+noballs per over, wicket-kind mix (bowled+lbw share ≈ attacks stumps), phase-split economy & strike rate, wickets vs top-4 vs 5–11, at venue, vs opposition.
Fielding: catches per match, run-out involvements, stumpings, dropped-catch proxy is not available.
Availability / experience: matches played (format), matches in last 12 months (form & fitness proxy), player_of_match count.

### 2.3 Contextual "impact" features (these separate a good selector from a stats page)
- **Expected-outcome baseline model**: a ball-level model P(outcome | phase, match state, venue, format, era) ignoring players. Then per player:
  - `RAE_bat` runs above expectation per ball and `DAE_bat` dismissals below expectation;
  - `RSE_bowl` runs saved vs expectation, `WAE_bowl` wickets above expectation.
  These already normalise for venue, phase and situation, so a death-overs 150 SR is credited more than a powerplay 150 SR.
- **Win-probability-added (WPA)**: fit a win-probability model on match state (score, wickets, balls left, target, venue par). Every ball's ΔWP is credited to batter and bowler. Sum per player per match → "clutch" contribution; average over rolling window. This is the most direct measure of *impact*.
- **Learned player embeddings**: train a matchup model P(ball outcome | batter_emb, bowler_emb, context). The embeddings encode style without any line/length data (e.g., who struggles against wrist-spin-like bowlers). Frozen embeddings feed the team model and the pairwise matchup features below.

### 2.4 Venue features (rolling, per format)
- Par score / avg 1st-inns score, avg 2nd-inns score, chase win %, bat-first win % by toss decision, boundary rate, phase run rates, wicket kinds distribution (bowled+lbw share ⇒ low/skiddy; caught share ⇒ bounce/pace), spin vs pace economy & strike-rate gap (⇒ spin-friendly index), 2nd-inns/1st-inns run-rate ratio (dew proxy for evening T20), match count (for shrinkage; fold small venues into city/country priors), home-team flag.

### 2.5 Opposition features
- Their probable XI (last XI or squad) → same aggregates as below, plus: their inferred pace/spin bowling split, top-order vs middle-order strength, death-bowling strength, weakness vs pace/spin (batting RAE split), recent form (last-10 win %, run-rate differential), head-to-head last N.

### 2.6 Combination (XI-level) features — the part that distinguishes lineups
Computed from the 11 chosen players' features + roles; these are what let the model say "these 11 fit together":

Role coverage
- `n_keepers` (exactly 1 wanted), `n_frontline_bowlers` (≥5 with adequate overs), `n_allrounders`, `n_pace`, `n_spin`, `has_death_specialist`, `has_new_ball_specialist`, `expected_overs_available` (sum of capped per-player expected overs vs 20/50 required — penalises lineups that can't bowl a full quota).
- `pace_spin_balance × venue_spin_index` interaction (explicit feature, also let the model learn it).

Batting structure
- Expected runs and dismissal rate at each slot 1–7 given inferred positions; `top3_strength`, `middle_strength`, `finisher_strength` (positions 5–7 death-phase RAE), `batting_depth` (expected runs from slots 8–11), `left_tail_length` (# players with near-zero batting sample), `pp_bat_strength` (openers' powerplay RAE), `pressure_SR` aggregate.

Bowling structure
- `phase_bowling_strength` (PP / middle / death aggregate RSE & WAE for the best available bowlers in each phase), `bowling_variety` (entropy over inferred types), `wicket_taking_index` vs `containment_index`, `extras_rate`.

Pairwise / synergy
- `partnership_synergy`: for batter pairs with history, actual partnership run rate & dismissal rate minus the pair's individual expectations (co-batting residual).
- `matchup_edge`: sum over our batters × their bowlers (and our bowlers × their batters) of the matchup model's predicted advantage — this is the direct "this XI vs that XI at this venue" signal.
- `cohesion`: mean number of past matches pairs have played together; matches this exact core (≥8) has played.
- `co-selection residual`: from a mixed-effects/ridge model of match margin on player indicators + pair indicators (plus-minus style), the pair terms capture "better together than the sum".

Aggregates of impact
- `sum_WPA`, `sum_RAE`, `sum_RSE`, and their variance (a high-variance XI may be right when underdog, wrong when favourite — include `is_favourite` interaction).
- Team-vs-opposition differentials of every aggregate above (ours − theirs); differentials generalise far better than raw values.

## 3. Models (layered)

| Layer | Model | Purpose |
|---|---|---|
| L0 | Ball-outcome model (multinomial GBM or small NN with player embeddings + context) | Expected-outcome baselines, embeddings, and Monte-Carlo simulation |
| L1 | Win-probability model on match state (GBM / logistic, per format) | WPA credit; also used inside simulation |
| L2 | **Match-outcome model**: P(team A wins | XI_A features, XI_B features, venue, toss, format, home) | The scorer for candidate XIs |
| L3 | Selector: enumerate / search 11-subsets of the pool, score with L2 (and optionally L0 simulation), apply hard constraints | The actual product |

L2 in two flavours, ensembled:
- (a) Gradient boosting (LightGBM/CatBoost) on the engineered combination features — strong, interpretable via SHAP ("why is this XI better: +death bowling, −batting depth").
- (b) Set-based neural model: DeepSets / set-transformer over the 11 player embeddings + role features for each side, venue embedding, cross-attention between the two sides. Permutation-invariant, naturally scores unseen sets, learns interactions the hand-crafted features miss. Needs the ~13k T20 matches; augment by training on both sides' perspective and on innings-level targets (margin, run rate) as auxiliary losses.
- (c) Simulation cross-check: simulate N innings with L0 using the chosen XI vs opposition XI at the venue → P(win). Slower but fully compositional; use to validate L2 on the final shortlist.

Selector: pools of 15–25 → C(20,11) = 168k, C(25,11) = 4.4M combinations; GBM scoring at ~1M/s makes exhaustive search fine for T20; otherwise prune with constraints (1 keeper, ≥5 bowlers, ≤4 overseas) then greedy-swap / genetic / ILP on a linearised score. Output the top-k XIs with the SHAP deltas between them.

## 4. Validation (temporal, no leakage)
- Train ≤ 2022, validate 2023–24, test 2025–26. Player features for a match use only earlier matches.
- Metrics: log-loss, Brier, AUC, calibration curve. Baselines to beat: 50/50, home-advantage, team Elo/Glicko, toss-adjusted Elo. Realistic ceiling for T20 is modest (AUC ≈ 0.62–0.70); the value is in *ranking XIs*, not absolute accuracy.
- Counterfactual sanity tests: removing a team's best WPA player should lower P(win); duplicating a role (2 keepers, 0 spinners at a spin venue) should lower it; swapping in an unknown player should regress toward the role prior, not collapse.
- Natural experiments: within a series, teams change 1–2 players between matches — check that predicted ΔP(win) correlates with realised outcomes/margins.
- Ablations: drop combination features → how much does ranking quality fall? (This quantifies whether the combination features actually carry signal.)

## 5. Known pitfalls
- Survivorship/selection bias: observed XIs are already the selectors' best guesses; the model learns from a narrow region of lineup space. Shrink toward player-level (compositional) signal; regularise pair terms hard.
- Sparse players: Bayesian shrinkage toward role/format priors; encode sample sizes as features.
- Venue/opposition sparsity: hierarchical priors (venue → city → country).
- Era drift (T20 scoring inflation): season-normalised expectations; include season as a feature in L0.
- Identity: use registry IDs; names collide and change.
- Format leakage: don't mix Test balls into T20 features except as an "experience" count.
- Withheld Afghanistan matches leave holes for players like Rashid Khan in international context; club data still covers them.

## 6. Milestones
1. Parquet pipeline + as-of feature store (T20 men's first). Verify ball counts against scorecards.
2. Role inference + external bowling-style table; L0 expected-outcome model; RAE/RSE/WPA per player-match.
3. Venue & opposition feature tables.
4. L2 GBM with combination features; temporal validation vs Elo baseline; SHAP report.
5. Set-transformer L2; ensemble; simulation check.
6. Selector with constraints + explanation output; extend to ODI, then women's T20.
