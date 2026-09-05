"""Row assembly for the training frames: one win row per decided match, and one row per
(match, player) for every XI member (L1 -> L2-B).

Both the training pass (``ml.xi.builder``) and the serving-parity check (H-8,
``ml.xi.asof``) build rows through this module, always from a ``RatingState`` that has not
yet folded the match in. Player rows cover ALL XI players -- never only those who batted
or bowled, because who got to bat is decided by the result (H-20); a player without a
delivery gets zero targets, which is what happened to them.
"""

from __future__ import annotations

from typing import Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi.ratings import RatingState, aggregate_side, match_features
from ml.xi.sources import Deliveries, MatchRecord, batting_positions

_ZERO_ACTUALS: Dict[str, float] = {name: 0.0 for name in C.PLAYER_MATCH_TARGET_COLS}


def match_actuals(match: MatchRecord) -> Dict[str, Dict[str, float]]:
    """What each player did in the match, keyed by player key (``PLAYER_MATCH_TARGET_COLS``).

    Counts are over deliveries, wides included -- the same definition the as-of
    ``exp_balls_*`` vectors use -- and ``wickets`` / ``runs_conceded`` follow
    ``bowl_wrate`` / ``bowl_rate`` (bowler-credited kinds; total runs off the ball).
    """
    d = match.deliveries
    out: Dict[str, Dict[str, float]] = {}
    if not len(d):
        return out

    def entry(key: str) -> Dict[str, float]:
        return out.setdefault(key, dict(_ZERO_ACTUALS))

    unique_batters, batter_inverse = np.unique(d.batter, return_inverse=True)
    runs = np.bincount(batter_inverse, weights=d.runs_batter)
    fours = np.bincount(batter_inverse, weights=(d.runs_batter == 4).astype(float))
    sixes = np.bincount(batter_inverse, weights=(d.runs_batter == 6).astype(float))
    balls_faced = np.bincount(batter_inverse).astype(float)
    positions = batting_positions(d)
    for i, key in enumerate(unique_batters):
        e = entry(key)
        e["balls_faced"] = float(balls_faced[i])
        e["runs"] = float(runs[i])
        e["fours"] = float(fours[i])
        e["sixes"] = float(sixes[i])
        e["batting_position"] = float(positions.get(key, 0))

    unique_bowlers, bowler_inverse = np.unique(d.bowler, return_inverse=True)
    balls_bowled = np.bincount(bowler_inverse).astype(float)
    runs_conceded = np.bincount(bowler_inverse, weights=d.runs_total)
    wickets = np.bincount(bowler_inverse, weights=d.bowler_wicket)
    for i, key in enumerate(unique_bowlers):
        e = entry(key)
        e["balls_bowled"] = float(balls_bowled[i])
        e["runs_conceded"] = float(runs_conceded[i])
        e["wickets"] = float(wickets[i])

    # A source built before player_out existed (hand-made test Deliveries) records no
    # dismissals; both real sources always fill the column.
    if len(d.player_out) == len(d):
        dismissed = d.player_out[d.player_out != ""]
        unique_out, out_counts = np.unique(dismissed, return_counts=True)
        for key, count in zip(unique_out, out_counts):
            entry(key)["dismissals"] = float(count)

    # A catch is a fielder credited on a bowler-credited dismissal that is not a stumping
    # (caught, caught and bowled); run-outs are not the bowler's and carry no catch.
    caught = np.flatnonzero((d.bowler_wicket > 0) & (d.stumping == 0))
    for i in caught:
        for key in d.fielders[i] if i < len(d.fielders) else []:
            entry(key)["catches"] += 1.0
    return out


def innings_outcomes(match: MatchRecord) -> Dict[str, float]:
    """What the first two innings did (``contract.INNINGS_OUTCOME_COLS``): runs, dismissals
    and deliveries. Outcome columns on the win row -- E2 scores simulated totals against
    them -- and never an input."""
    d = match.deliveries
    out = {col: 0.0 for col in C.INNINGS_OUTCOME_COLS}
    if not len(d):
        return out
    for number, inning in enumerate(np.unique(d.innings)[:2], start=1):
        mask = d.innings == inning
        out[f"innings{number}_runs"] = float(d.runs_total[mask].sum())
        out[f"innings{number}_wickets"] = float(d.wicket[mask].sum())
        out[f"innings{number}_deliveries"] = float(mask.sum())
    return out


def serving_match(
    format_code: str,
    team1_players: Sequence[str],
    team2_players: Sequence[str],
    team1: Optional[str],
    team2: Optional[str],
    venue: Optional[str],
    match_date,
) -> MatchRecord:
    """A match that has not been played, for the serving path to build feature rows from.
    Team and venue names are optional: without them the context columns read neutral."""
    return MatchRecord(
        match_id="",
        match_date=match_date,
        format_code=format_code,
        team1=team1 or "",
        team2=team2 or "",
        venue=venue or "",
        gender="",
        team1_players=list(team1_players),
        team2_players=list(team2_players),
        winner=None,
        result=None,
        deliveries=Deliveries.empty(),
    )


def player_feature_rows(state: RatingState, match: MatchRecord) -> Tuple[Dict, List[Dict]]:
    """Both sides' aggregates as the win-feature row, and one feature row per XI player
    (``PLAYER_MATCH_META_COLS`` + ``PLAYER_MATCH_FEATURE_COLS``), from the state as of the
    match date. The win row also carries the simulator's as-of context
    (``SIMULATION_CONTEXT_COLS``) and the fixture context (``FIXTURE_CONTEXT_COLS``), which
    every player row repeats; every player row also carries the player's age at the match
    date (``AGE_COLS``). The performance model's serving path reads rows from here;
    the training pass adds what the player then did through ``build_match_rows``."""
    vectors1 = state.side_vectors(match.format_code, match.team1_players, on=match.match_date)
    vectors2 = state.side_vectors(match.format_code, match.team2_players, on=match.match_date)
    side1 = aggregate_side(vectors1, match.format_code)
    side2 = aggregate_side(vectors2, match.format_code)
    context = team_context_or_neutral(state, match)
    fixture_context = state.fixture_context(match)
    # Age at the match date (X-1b): a static fact read at a date, on every player row; the
    # win row does not carry it, since no win feature reads it.
    ages1 = state.age_vectors(match.team1_players, match.match_date)
    ages2 = state.age_vectors(match.team2_players, match.match_date)

    win_row = {
        "match_id": match.match_id,
        "match_date": pd.Timestamp(match.match_date),
        "format_code": match.format_code,
        "gender": match.gender,
        "team1": match.team1,
        "team2": match.team2,
        "venue": match.venue,
        C.TARGET_COL: match.outcome,
    }
    win_row.update(match_features(side1, side2))
    win_row.update(context)
    win_row.update(state.simulation_context(match.format_code, match.gender))
    win_row.update(fixture_context)

    player_rows: List[Dict] = []
    sides = (
        (1, match.team1_players, vectors1, ages1, side1, side2, match.team1, match.team2, 1.0),
        (2, match.team2_players, vectors2, ages2, side2, side1, match.team2, match.team1, -1.0),
    )
    for side, keys, vectors, ages, own, opp, own_team, opp_team, elo_sign in sides:
        for i, key in enumerate(keys):
            row = {
                "match_id": match.match_id,
                "match_date": pd.Timestamp(match.match_date),
                "format_code": match.format_code,
                "gender": match.gender,
                "side": side,
                "team": own_team,
                "opponent": opp_team,
                "venue": match.venue,
                "player_key": key,
            }
            for name in C.PLAYER_VECTOR_KEYS + C.PLAYER_ROLE_KEYS + C.PLAYER_SEQUENCE_KEYS:
                row[name] = float(vectors[name][i])
            for stem in C.SIDE_FEATURE_STEMS:
                row[f"own_{stem}"] = own[stem]
                row[f"opp_{stem}"] = opp[stem]
            row["venue_bf_rate"] = context["venue_bf_rate"]
            row["venue_n"] = context["venue_n"]
            row["elo_edge"] = elo_sign * context["team_elo_diff"]
            row.update(fixture_context)
            for name in C.AGE_COLS:
                row[name] = float(ages[name][i])
            player_rows.append(row)
    return win_row, player_rows


def build_match_rows(state: RatingState, match: MatchRecord) -> Tuple[Dict, List[Dict]]:
    """The win-frame row and the player-match rows for one decided match, computed from
    the state as of the match date. The caller guarantees the match is not folded in yet."""
    win_row, player_rows = player_feature_rows(state, match)
    win_row.update(innings_outcomes(match))
    actuals = match_actuals(match)
    for row in player_rows:
        row.update(actuals.get(row["player_key"], _ZERO_ACTUALS))
    return win_row, player_rows


def team_context_or_neutral(state: RatingState, match: MatchRecord) -> Dict[str, float]:
    """Team-level context, neutral when the serving caller named no teams."""
    if match.team1 and match.team2:
        return state.team_context(match)
    return {
        "team_elo_diff": 0.0,
        "team_form_diff": 0.0,
        "team_h2h": 0.5,
        "team_h2h_n": 0.0,
        "venue_bf_rate": 0.5,
        "venue_n": 0.0,
        "venue_fam_diff": 0.0,
    }
