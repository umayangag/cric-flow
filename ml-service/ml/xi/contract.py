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

FORMAT_CODES: List[str] = ["T20", "T20I", "ODI", "TEST"]
FORMAT_INDEX: Dict[str, int] = {code: i for i, code in enumerate(FORMAT_CODES)}

# Rating-pass hyperparameters. Changing any of these changes the feature definitions and
# requires a re-run of the pass; they are recorded in the artifact metadata.
DECAY_PER_MATCH = 0.90  # exponential forgetting applied to a player's accumulators per match played
PRIOR_BALLS = 60.0  # shrinkage: a rate is (sum above expectation) / (balls + PRIOR_BALLS)
K_TEAM_ELO = 24.0
K_PLAYER_ELO = 12.0
ELO_INITIAL = 1500.0
MAX_OVER_INDEX = 100  # context baselines are indexed by over number, capped here
# Expected legal balls bowled per match for a player to count as a bowling option.
MIN_BOWLING_BALLS: Dict[str, int] = {"T20": 12, "T20I": 12, "ODI": 30, "TEST": 60}


def is_bowling_option(expected_balls_bowled, format_code: str):
    """Whether a player's expected balls bowled make them a bowling option. Works on scalars
    and arrays; the tolerance keeps a decayed ratio that is exactly the threshold on the
    right side of it. This is the single definition the feature and the constraint share."""
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
# 0 no constraint. Used for the monotone selection objective.
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
_SIDE_STEMS: List[str] = [
    "pelo_mean",
    "pelo_std",
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

DISPLAY_FEATURE_COLS: List[str] = XI_FEATURE_COLS + TEAM_CONTEXT_COLS

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
    sequence_families: Tuple[str, ...] = SEQUENCE_FAMILIES_KEPT, joint_format: bool = False
) -> List[str]:
    """The performance model's input columns: the row's as-of vectors and role, the kept
    sequence families, both sides' aggregates, venue context, the Elo edge and the innings."""
    sequence_keys = [key for family in sequence_families for key in SEQUENCE_FAMILIES[family]]
    cols = (
        PLAYER_VECTOR_KEYS
        + PLAYER_ROLE_KEYS
        + sequence_keys
        + [f"own_{s}" for s in SIDE_FEATURE_STEMS]
        + [f"opp_{s}" for s in SIDE_FEATURE_STEMS]
        + ["venue_bf_rate", "venue_n", "elo_edge", BATS_FIRST_COL]
    )
    if joint_format:
        cols.append(FORMAT_INDICATOR_COL)
    return cols


def monotone_directions(columns: List[str]) -> List[int]:
    """Monotone constraint per column for P(team1 wins): +1 / -1 / 0."""
    out = []
    for col in columns:
        if col.startswith("d_"):
            out.append(_STEM_DIRECTION.get(col[2:], 0))
        elif col.startswith("t1_"):
            out.append(_STEM_DIRECTION.get(col[3:], 0))
        elif col.startswith("t2_"):
            out.append(-_STEM_DIRECTION.get(col[3:], 0))
        else:
            out.append(0)
    return out
