"""Feature contract for the XI-responsive win model.

Two families, kept apart on purpose:

* ``XI_FEATURE_COLS`` -- every column is a function of the two elevens (and nothing else).
  These are the selection objective: changing one player changes them.
* ``TEAM_CONTEXT_COLS`` -- team Elo, form, head-to-head, venue. Constant with respect to the
  XI, so they raise outcome accuracy but cannot choose a player. They are excluded from the
  objective used by the optimiser and included in the model used for the displayed
  probability.
"""

from __future__ import annotations

from typing import Dict, List, Optional, Tuple

import numpy as np

FORMAT_CODES: List[str] = ["T20", "T20I", "ODI", "TEST"]
FORMAT_INDEX: Dict[str, int] = {code: i for i, code in enumerate(FORMAT_CODES)}

# The gender half of a team's identity, as go-app writes it into ``match.gender`` and
# ``opposition.gender`` from Cricsheet's ``info.gender``. This service *matches on the
# literal*: ``RatingState._ctx_group`` reads ``GENDER_FEMALE`` to pick E7's context-baseline
# group, and ``team_key`` folds it into the key a team is rated under. It is declared in
# contracts/ops-console.contract.json and asserted from both sides (H-24, D-10) -- a private
# copy of a word two services agree on is exactly what D-9 was.
GENDER_MALE = "male"
GENDER_FEMALE = "female"
TEAM_GENDERS: List[str] = [GENDER_MALE, GENDER_FEMALE]

# Rating-pass hyperparameters. Changing any of these changes the feature definitions and
# requires a re-run of the pass; they are recorded in the artifact metadata.
DECAY_PER_MATCH = 0.90  # exponential forgetting applied to a player's accumulators per match played
PRIOR_BALLS = 60.0  # shrinkage: a rate is (sum above expectation) / (balls + PRIOR_BALLS)
K_TEAM_ELO = 24.0
K_PLAYER_ELO = 12.0
ELO_INITIAL = 1500.0
MAX_OVER_INDEX = 100  # context baselines are indexed by over number, capped here
#: Expected balls bowled **per XI appearance** (``exp_balls_bowled``, every match the
#: player was named for, bowled in or not -- FEAT-01) for a player to count as a bowling
#: option. The bar is the one the pass has always applied, expressed in that unit
#: (FEAT-15): 12 / 12 / 30 / 60 were set when the vector was balls per match *bowled in*,
#: and read per appearance the same numbers were a far higher bar -- a frontline bowler
#: who bowls his allocation in half his appearances sat exactly on the line, and a fifth
#: of T20I sides read as short of five options. Derived from the population before any
#: outcome was read, by prevalence: the old bar admitted 58.4 % / 54.2 % / 49.7 % / 47.5 %
#: of the archive's XI appearances (T20 / T20I / ODI / TEST; 272,949 / 46,904 / 115,322 /
#: 68,750 appearances with deliveries, 22,905 matches, 2026-09-19), so the new bar is the
#: per-appearance quantile that admits the same share -- 3.30 / 3.81 / 18.97 / 40.34,
#: rounded to whole balls. It is not the old number scaled by how often an option bowls
#: (0.85 / 0.83 / 0.91 / 0.95 of appearances, which would give 10 / 10 / 27 / 57): the
#: players the line decides bowl in about half their appearances, not 85 %, and that
#: scaling leaves 12-28 % of real sides under five. On the decided sides since 2024 the
#: share under five reads 3.2 / 3.9 / 12.8 / 12.5 % here against 4.3 / 4.8 / 17.3 / 15.8 %
#: before FEAT-01 and 25.1 / 19.1 / 38.2 / 21.5 % on the old numbers in the new unit
#: (``tests/fixtures/bowling_option_population.json`` holds a sample of those sides).
MIN_BOWLING_BALLS: Dict[str, int] = {"T20": 3, "T20I": 4, "ODI": 19, "TEST": 40}


def is_bowling_option(expected_balls_bowled, format_code: str):
    """Whether a player's expected balls bowled per XI appearance make them a bowling
    option. Works on scalars and arrays; the tolerance keeps a decayed ratio that is
    exactly the threshold on the right side of it. This is the single definition the
    feature and the constraint share."""
    return expected_balls_bowled >= MIN_BOWLING_BALLS[format_code] - 1e-6


TEAM_FORM_WINDOW = 10
HEAD_TO_HEAD_WINDOW = 10
VENUE_PRIOR_MATCHES = 5.0

# Innings phases, per format: (first middle over, first death over). Batting and bowling
# impact is accumulated per phase so the performance model can see that a death-overs
# hitter and an opening bowler are different jobs. TEST has no powerplay; its buckets are
# positional (new ball / middle / old ball) and exist only so every format defines the
# same columns.
PHASE_NAMES: List[str] = ["pp", "mid", "death"]
PHASE_BOUNDS: Dict[str, Tuple[int, int]] = {"T20": (6, 15), "T20I": (6, 15), "ODI": (10, 40), "TEST": (30, 80)}
# Per-phase shrinkage: a third of PRIOR_BALLS, since each phase holds roughly a third of a
# player's balls.
PHASE_PRIOR_BALLS = PRIOR_BALLS / 3.0

# Expected batting slot: shrunk toward position 7 with the weight of two innings, so a
# player with no batting history reads as lower-middle order rather than an opener.
BAT_POSITION_PRIOR = 7.0
BAT_POSITION_PRIOR_INNINGS = 2.0

# Per-player as-of vector keys (what the serving store holds for every player x format).
PLAYER_VECTOR_KEYS: List[str] = [
    "exp_balls_faced",
    "exp_balls_bowled",
    "bat_rate",  # runs above expectation per ball faced (shrunk)
    "bat_wrate",  # dismissals below expectation per ball faced (shrunk; positive = harder to get out)
    "bowl_rate",  # runs saved vs expectation per ball bowled (shrunk)
    "bowl_wrate",  # bowler-credited wickets above expectation per ball bowled (shrunk)
    "career",  # matches in this format before this match
    "career_all",  # matches in any format before this match
    "pelo",  # player Elo in this format
    "keeper",  # 1.0 if the player has ever been credited with a stumping
]

# As-of expected-role keys, held beside the vectors for every player x format. They feed
# the player-match rows (L2-B's training frame) and are returned by the same
# ``side_vectors`` read the win path uses, so training and serving cannot drift apart.
PLAYER_ROLE_KEYS: List[str] = (
    [
        "exp_bat_position",  # decayed mean batting position, shrunk toward BAT_POSITION_PRIOR
        "bat_innings_share",  # decayed share of XI appearances in which the player batted
    ]
    + [f"bat_{p}_rate" for p in PHASE_NAMES]  # runs above expectation per ball faced, per phase (shrunk)
    + [f"bowl_{p}_rate" for p in PHASE_NAMES]  # runs saved vs expectation per ball bowled, per phase (shrunk)
)

# Sequence families (E1): the ``seqcalc`` calculators expressed as as-of accumulators in the
# rating pass, one decayed rate per (player, format). Each is (numerator over the balls a
# family selects) / (those balls + a prior), so a player without history reads as neutral.
# ``ml.xi.sequence`` derives the per-ball flags; the pass accumulates them beside every
# other rate. A family is fed to the performance model only if E1 kept it
# (``SEQUENCE_FAMILIES_KEPT``); the columns are always in the frame so the question can be
# re-asked without a new pass.
SEQUENCE_FAMILIES: Dict[str, List[str]] = {
    "dot_streaks": [
        "bat_stuck_share",  # share of balls faced that follow >= 2 consecutive dots by the batter
        "bat_release_rate",  # runs above expectation per ball on those balls
        "bowl_squeeze_share",  # share of balls bowled that follow >= 2 consecutive dots by the bowler
        "bowl_squeeze_wrate",  # bowler-credited wickets above expectation per ball on those balls
    ],
    "reactions": [
        "bat_after_boundary_rate",  # runs above expectation on the ball after the batter's own boundary
        "bowl_after_boundary_rate",  # runs saved on the ball after the bowler conceded a boundary
        "bowl_after_wicket_rate",  # runs saved on the ball after the bowler took a wicket
    ],
    "spells": [
        "bowl_spell_first_rate",  # runs saved per ball in the first over of a spell
        "bowl_spell_later_rate",  # runs saved per ball in a spell's later overs
        "bowl_spell_overs",  # decayed mean overs per spell
    ],
}
PLAYER_SEQUENCE_KEYS: List[str] = [key for keys in SEQUENCE_FAMILIES.values() for key in keys]
# A ball is "under a squeeze" after this many consecutive dots in the same stream.
SEQUENCE_DOT_STREAK = 2
# Successive overs by one bowler at most this far apart belong to one spell (alternate ends).
SEQUENCE_SPELL_MAX_GAP = 2
# Shrinkage for the sequence rates, which select a subset of a player's balls.
SEQUENCE_PRIOR_BALLS = PHASE_PRIOR_BALLS

# E1's verdict: families the performance model consumes. Decided on the walk-forward folds
# by the > 1 % pinball-loss rule over three seeds (plan §5); the table is in the plan.
SEQUENCE_FAMILIES_KEPT: Tuple[str, ...] = ()

# Fixture context (A-1): the scoring level of the ground and of the competition, as-of, so
# the performance model can forecast a different total at a ground where 140 is par than
# at one where 190 is. Each column is the key's shrunk as-of runs (dismissals) per delivery
# over the format's as-of rate, so 1.0 is "an average ground" and a key with no history
# reads exactly 1.0 (``RatingState.fixture_context``). Keyed by (format, venue) and
# (format, competition name); never decayed, like ``venue_bf_rate``. The columns are always
# in the frame; a family is fed to the performance model only if gate A-1 kept it
# (``FIXTURE_CONTEXT_FAMILIES_KEPT``, plan §8.9).
FIXTURE_CONTEXT_FAMILIES: Dict[str, List[str]] = {
    "venue": [
        "venue_run_rate_rel",  # shrunk runs per delivery at the ground / the format's runs per delivery
        "venue_wicket_rate_rel",  # shrunk dismissals per delivery at the ground / the format's
    ],
    "competition": [
        "competition_run_rate_rel",  # the same, keyed by the competition (Cricsheet's event name)
        "competition_wicket_rate_rel",
    ],
}
FIXTURE_CONTEXT_COLS: List[str] = [col for cols in FIXTURE_CONTEXT_FAMILIES.values() for col in cols]
#: Shrinkage of a key's rate toward the format's: the weight, in deliveries, of the prior --
#: five T20 innings, two ODI innings. One number, chosen by the size of an innings, not swept.
FIXTURE_CONTEXT_PRIOR_BALLS = 600.0
#: Gate A-1's verdict (plan §8.9): a recorded null. On the walk-forward folds no family
#: shrank the per-quarter |bias| of the simulated first-innings mean in both T20 and ODI --
#: T20 flat on every arm, ODI by 0.2-0.4 runs on a mean of 14 (within one fold-level
#: standard error), the -47-run quarter untouched -- so the performance model reads none.
FIXTURE_CONTEXT_FAMILIES_KEPT: Tuple[str, ...] = ()

# Player biography (X-1b): the one fact the archive cannot hold that X-1a found at usable
# coverage -- a date of birth for 85 % of appearances. ``age`` is the player's age in years
# at the match date and ``age_known`` says whether a date of birth exists; a player without
# one reads ``age`` 0.0 *and* ``age_known`` 0.0, so the missing rows are their own category
# and never an imputed age. Both columns are on every player row (``rows.player_feature_rows``
# computes them from the state's birth dates and the match date, one read path for training
# and serving), and the performance model reads them only if gate X-1b's age family kept
# them (``AGE_FEATURES_KEPT``, plan §8.12). No curvature term: the model is a tree ensemble,
# whose splits are invariant to any monotone transform of a column, and age squared is
# monotone over every age a cricketer has.
AGE_COLS: List[str] = ["age", "age_known"]
#: Gate X-1b, family 1 (plan §8.12): whether the performance model reads ``AGE_COLS``.
AGE_FEATURES_KEPT = False
#: Age bands for the age-aware cold start (X-1b family 3): the upper bound of each band in
#: years, the last band open. Chosen from the population -- the quartiles of age at match
#: date sit near 24.5 / 28 / 31.5 in every format -- before any outcome was read.
AGE_BANDS: Tuple[float, ...] = (22.0, 26.0, 30.0, 34.0)
N_AGE_BANDS = len(AGE_BANDS) + 1
#: Gate X-1b, family 3 (plan §8.12): whether a player with no history in the format and a
#: known age reads the as-of debut profile of his age band instead of the neutral vector.
AGE_AWARE_COLD_START = False


def age_band(age_years) -> "np.ndarray":
    """The band index (0 .. ``N_AGE_BANDS`` - 1) of an age; works on scalars and arrays."""
    return np.searchsorted(np.asarray(AGE_BANDS), np.asarray(age_years, dtype=float), side="right")


# One side's aggregates, produced by ml.xi.ratings.aggregate_side. Order is the contract.
SIDE_FEATURE_STEMS: List[str] = [
    "imp_bat_sum",
    "imp_bat_top6",
    "imp_bat_tail",
    "imp_bat_wk",
    "imp_bowl_sum",
    "imp_bowl_top5",
    "imp_bowl_wk",
    "imp_bowl_wk_top5",
    "n_bowlers",
    "exp_balls_bowled_top5",
    "exp_balls_faced_sum",
    "has_keeper",
    "n_allrounders",
    "exp_mean_matches",
    "n_debutants",
    "exp_mean_matches_all",
    "pelo_mean",
    "pelo_top3",
    "pelo_min",
    "pelo_std",
]

# Sign of each stem's effect on P(team1 wins) when it belongs to team1: +1 good, -1 bad,
# 0 no constraint. Both win models are fitted under it, through ``monotone_directions``:
# the display model as tree constraints, the objective as bounds on its coefficients
# (FEAT-14). A stem at 0 is one whose direction cannot be declared; such a stem may be
# read only if no one-player upgrade moves it, or the surface is not monotone under the
# swap -- which is why ``pelo_std`` is in no win model (``_SIDE_STEMS``).
_STEM_DIRECTION: Dict[str, int] = {
    "imp_bat_sum": 1,
    "imp_bat_top6": 1,
    "imp_bat_tail": 1,
    "imp_bat_wk": 1,
    "imp_bowl_sum": 1,
    "imp_bowl_top5": 1,
    "imp_bowl_wk": 1,
    "imp_bowl_wk_top5": 1,
    "n_bowlers": 1,
    "exp_balls_bowled_top5": 0,
    "exp_balls_faced_sum": 0,
    "has_keeper": 1,
    "n_allrounders": 0,
    "exp_mean_matches": 1,
    "n_debutants": -1,
    "exp_mean_matches_all": 1,
    "pelo_mean": 1,
    "pelo_top3": 1,
    "pelo_min": 1,
    "pelo_std": 0,
}

# The XI-responsive contract: differentials plus both sides' raw values for the subset that
# is not purely relative (a strong side against a strong side is not the same as two weak ones).
_DIFF_STEMS: List[str] = [
    "pelo_mean",
    "pelo_top3",
    "pelo_min",
    "imp_bat_sum",
    "imp_bat_top6",
    "imp_bat_tail",
    "imp_bat_wk",
    "imp_bowl_sum",
    "imp_bowl_top5",
    "imp_bowl_wk",
    "imp_bowl_wk_top5",
    "n_bowlers",
    "exp_balls_bowled_top5",
    "exp_balls_faced_sum",
    "n_allrounders",
    "exp_mean_matches",
    "n_debutants",
    "exp_mean_matches_all",
]
# ``pelo_std`` -- the spread of player Elo across an eleven -- is a side aggregate (it
# reaches the performance model as ``own_pelo_std``) but no win model reads it. Its
# direction is 0 and cannot be declared: raising a player above the side's mean widens the
# spread and raising one below it narrows it. It is the only stem a one-player upgrade
# moves that the contract leaves free, so any model that reads it can lower P(win) on an
# upgrade whatever the other coefficients do. B-7 (PR #267) took it out of the display
# model for exactly that, at −0.0004 / +0.0088 / −0.0055 / −0.0047 of display AUC; FEAT-14
# took it out of the objective once the other stems were sign-bound and it was where every
# remaining swap violation came from (3 / 1 / 0 / 1 per format on the folds, all through
# this stem; 0 / 0 / 0 / 0 without it, for +0.0017 / +0.0022 / −0.0011 / −0.0001 of
# objective AUC in T20I / ODI / T20 / TEST, each within one fold-level standard error).
_SIDE_STEMS: List[str] = [
    "pelo_mean",
    "imp_bat_sum",
    "imp_bat_top6",
    "imp_bat_tail",
    "imp_bat_wk",
    "imp_bowl_sum",
    "imp_bowl_top5",
    "imp_bowl_wk",
    "imp_bowl_wk_top5",
    "n_bowlers",
    "n_debutants",
]

XI_FEATURE_COLS: List[str] = (
    [f"d_{s}" for s in _DIFF_STEMS] + [f"t1_{s}" for s in _SIDE_STEMS] + [f"t2_{s}" for s in _SIDE_STEMS]
)

TEAM_CONTEXT_COLS: List[str] = [
    "team_elo_diff",
    "team_form_diff",
    "team_h2h",
    "team_h2h_n",
    "venue_bf_rate",
    "venue_n",
    "venue_fam_diff",
]

# Sign of each team-context column's effect on P(team1 wins), for the three whose direction
# is knowable rather than merely plausible: a stronger, better-formed side more familiar
# with the ground does not win less often, and all three are negated by
# ``train.swap_orientation``, so the constraint means the same thing in both batting orders.
# The other four are deliberately absent: ``team_h2h_n``, ``venue_n`` and ``venue_bf_rate``
# are sample sizes and a venue property with no side attached, and ``team_h2h`` is a rate
# whose small-sample values say more about how often the pair has met than who is better.
#
# Gate B-7 decides whether the display model reads them as constraints; the objective never
# sees these columns at all.
_CONTEXT_DIRECTION: Dict[str, int] = {
    "team_elo_diff": 1,
    "team_form_diff": 1,
    "venue_fam_diff": 1,
}
#: Gate B-7: whether the display model's team-context columns are monotone-constrained.
DISPLAY_CONTEXT_MONOTONE_KEPT = False

# Match stakes (X-3): what the fixture was worth, derived from Cricsheet's event fields
# (``ml.xi.stakes``). ``stakes_knockout`` is 1.0 for a knockout or a final and
# ``stakes_stage_known`` says whether the archive placed the match in its competition at
# all -- so an unlabelled match reads 0.0 on both and is its own category rather than an
# implied group fixture, the same shape ``age`` / ``age_known`` uses. Both columns are on
# every win row; the display model reads them only if gate X-3's stakes family kept them
# (``STAKES_FEATURES_KEPT``). The dead-rubber flag is deliberately NOT here: it needs the
# edition's fixture list, which makes it fit to clean a measurement and unfit to be a
# feature (H-21, ``ml.xi.stakes``).
STAKES_COLS: List[str] = ["stakes_knockout", "stakes_stage_known"]
#: Gate X-3, use 2: whether the display model reads ``STAKES_COLS``.
STAKES_FEATURES_KEPT = False

# The display model reads every XI column the objective reads, plus the team context the
# objective must not see (it cannot distinguish two elevens). The two lists differ in
# nothing else since FEAT-14: B-7's ``t1_pelo_std`` / ``t2_pelo_std`` exclusion is now the
# contract for both models (see ``_SIDE_STEMS``).
DISPLAY_FEATURE_COLS: List[str] = (
    list(XI_FEATURE_COLS) + TEAM_CONTEXT_COLS + (list(STAKES_COLS) if STAKES_FEATURES_KEPT else [])
)

TARGET_COL = "team1_wins"

# --- Player-match rows (L1 -> L2-B training frame) -----------------------------------
#
# One row per (match, player), for ALL XI players of every decided match -- never only
# those who batted or bowled, because who got to bat is decided by the result (H-20).

PLAYER_MATCH_META_COLS: List[str] = [
    "match_id",
    "match_date",
    "format_code",
    "gender",
    "side",  # 1 = batted first
    "team",
    "opponent",
    "venue",
    "player_key",
]

PLAYER_MATCH_FEATURE_COLS: List[str] = (
    PLAYER_VECTOR_KEYS
    + PLAYER_ROLE_KEYS
    + PLAYER_SEQUENCE_KEYS
    + [f"own_{s}" for s in SIDE_FEATURE_STEMS]
    + [f"opp_{s}" for s in SIDE_FEATURE_STEMS]
    + ["venue_bf_rate", "venue_n", "elo_edge"]  # elo_edge = own team Elo minus opponent's
    + FIXTURE_CONTEXT_COLS
    + AGE_COLS
)

# What the player then did. Counts are over deliveries, wides included -- the same
# definition the as-of ``exp_balls_*`` vectors use. ``batting_position`` is the order of
# first appearance on strike in the player's first batting innings, 0 when they never
# faced a ball; ``wickets`` and ``runs_conceded`` are the bowler-credited kinds and total
# runs off the ball, matching ``bowl_wrate`` / ``bowl_rate``. ``catches`` credits each
# fielder named on a bowler-credited dismissal that is not a stumping.
PLAYER_MATCH_TARGET_COLS: List[str] = [
    "balls_faced",
    "runs",
    "fours",
    "sixes",
    "dismissals",
    "batting_position",
    "balls_bowled",
    "wickets",
    "runs_conceded",
    "catches",
]

PLAYER_MATCH_COLS: List[str] = PLAYER_MATCH_META_COLS + PLAYER_MATCH_FEATURE_COLS + PLAYER_MATCH_TARGET_COLS

# --- Performance model (L2-B) inputs ----------------------------------------------------
#
# Everything the model reads is as-of (H-1) or decided before the match starts: the innings
# (``bats_first``) is the toss, not the result, and is marginalised at prediction unless the
# caller knows it. Nothing here is a function of the match's own outcome.
BATS_FIRST_COL = "bats_first"
# Joint T20 + T20I training (E6) reads the format through this indicator.
FORMAT_INDICATOR_COL = "is_t20i"
# Formats E6 considered pooling. Everything else always trains alone.
E6_JOINT_FORMATS: Tuple[str, str] = ("T20", "T20I")
# E6's verdict: whether the T20 and T20I performance models are one joint fit (recorded in
# the artifact; decided on the walk-forward folds by the > 0.01 Spearman rule).
E6_JOINT_T20_FORMATS = False

# --- Simulator (L2-C) inputs -----------------------------------------------------------
#
# The simulator is derived from L2-B and trains nothing. Besides the model's outputs it reads
# three as-of rates from the rating pass, carried on the win row and served from the state
# (plan §3, "The innings sample"), and the laws of the game below. Nothing else is a constant.

#: Legal deliveries in one innings per format; None where the laws set no limit, and there
#: the simulator does not run (H-17: TEST stays on the greedy path).
INNINGS_LEGAL_BALLS: Dict[str, Optional[int]] = {"T20": 120, "T20I": 120, "ODI": 300, "TEST": None}
MAX_WICKETS = 10
#: A bowler may deliver at most this share of an innings (four of twenty overs, ten of fifty).
BOWLER_MAX_SHARE = 0.2
#: As-of match context per format (and context group, like the run baselines): extras per
#: delivery, deliveries per full first innings (one not all out, so it ran its overs), and
#: the bowler-credited share of dismissals. Running rates over every delivery before the match.
SIMULATION_CONTEXT_COLS: List[str] = ["ctx_extras_per_ball", "ctx_innings_deliveries", "ctx_bowler_wicket_share"]
#: What each innings then did -- outcome columns on the win row. Targets for E2 (simulated
#: totals against actual), never inputs to anything.
INNINGS_OUTCOME_COLS: List[str] = [
    "innings1_runs",
    "innings1_wickets",
    "innings1_deliveries",
    "innings2_runs",
    "innings2_wickets",
    "innings2_deliveries",
]


def performance_feature_cols(
    sequence_families: Tuple[str, ...] = SEQUENCE_FAMILIES_KEPT,
    joint_format: bool = False,
    fixture_context_families: Tuple[str, ...] = FIXTURE_CONTEXT_FAMILIES_KEPT,
    age: bool = AGE_FEATURES_KEPT,
) -> List[str]:
    """The performance model's input columns: the row's as-of vectors and role, the kept
    sequence families, both sides' aggregates, venue context, the Elo edge, the kept
    fixture-context families, the age columns if gate X-1b kept them, and the innings."""
    sequence_keys = [key for family in sequence_families for key in SEQUENCE_FAMILIES[family]]
    fixture_keys = [key for family in fixture_context_families for key in FIXTURE_CONTEXT_FAMILIES[family]]
    cols = (
        PLAYER_VECTOR_KEYS
        + PLAYER_ROLE_KEYS
        + sequence_keys
        + [f"own_{s}" for s in SIDE_FEATURE_STEMS]
        + [f"opp_{s}" for s in SIDE_FEATURE_STEMS]
        + ["venue_bf_rate", "venue_n", "elo_edge"]
        + fixture_keys
        + (list(AGE_COLS) if age else [])
        + [BATS_FIRST_COL]
    )
    if joint_format:
        cols.append(FORMAT_INDICATOR_COL)
    return cols


def monotone_directions(columns: List[str], constrain_team_context: bool = DISPLAY_CONTEXT_MONOTONE_KEPT) -> List[int]:
    """Monotone constraint per column for P(team1 wins): +1 / -1 / 0.

    The display model passes it to the booster as ``monotonic_cst``; the objective fits its
    coefficients under it as sign bounds (FEAT-14). ``constrain_team_context`` is gate B-7's
    arm switch: with it off, every column outside the XI stems reads 0 and nothing
    constrains the display model's view of team strength, form and venue familiarity. It
    defaults to what the gate decided.
    """
    out = []
    for col in columns:
        if col.startswith("d_"):
            out.append(_STEM_DIRECTION.get(col[2:], 0))
        elif col.startswith("t1_"):
            out.append(_STEM_DIRECTION.get(col[3:], 0))
        elif col.startswith("t2_"):
            out.append(-_STEM_DIRECTION.get(col[3:], 0))
        elif constrain_team_context:
            out.append(_CONTEXT_DIRECTION.get(col, 0))
        else:
            out.append(0)
    return out
