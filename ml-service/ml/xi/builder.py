"""Run the rating pass over a source: training frame out, serving state out."""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Callable, List, Optional

import pandas as pd

from ml.xi import contract as C
from ml.xi.ratings import RatingState, aggregate_side, match_features
from ml.xi.sources import MatchSource

logger = logging.getLogger(__name__)


@dataclass
class BuildResult:
    frame: pd.DataFrame  # one row per match with a decided winner (plus metadata columns)
    state: RatingState  # as-of the end of the source: what the serving path predicts from
    n_undecided: int  # matches folded into the state but not usable as a training row


META_COLS: List[str] = ["match_id", "match_date", "format_code", "gender", "team1", "team2", "venue", C.TARGET_COL]


def build(source: MatchSource, progress: Optional[Callable[[int], None]] = None) -> BuildResult:
    """Run the pass. Matches are folded into the state at *day close*: every match on a date
    reads features from prior dates only, then the whole day is applied. Within a date the
    source's order is by id, not by start time, so sequential updates would let a match see
    the result of a same-day match it may in fact have preceded. 78% of matches share a date
    with another in the same format; the cost of the strict rule is <= 0.003 AUC."""
    state = RatingState()
    rows = []
    n_undecided = 0
    pending: List = []
    current_date = None
    for i, match in enumerate(source.iter_matches()):
        if current_date is not None and match.match_date < current_date:
            raise ValueError(f"source is not in date order: {match.match_id} ({match.match_date}) after {current_date}")
        if current_date is not None and match.match_date != current_date:
            for done in pending:
                state.update(done)
            pending.clear()
        current_date = match.match_date
        y = match.outcome
        if y is not None:
            side1 = aggregate_side(state.side_vectors(match.format_code, match.team1_players), match.format_code)
            side2 = aggregate_side(state.side_vectors(match.format_code, match.team2_players), match.format_code)
            row = {
                "match_id": match.match_id,
                "match_date": pd.Timestamp(match.match_date),
                "format_code": match.format_code,
                "gender": match.gender,
                "team1": match.team1,
                "team2": match.team2,
                "venue": match.venue,
                C.TARGET_COL: y,
            }
            row.update(match_features(side1, side2))
            row.update(state.team_context(match))
            rows.append(row)
        else:
            n_undecided += 1
        pending.append(match)
        if progress and i % 1000 == 0:
            progress(i)
    for done in pending:
        state.update(done)
    frame = pd.DataFrame(rows)
    logger.info(
        "rating pass: %d training rows, %d undecided matches, %d players",
        len(frame),
        n_undecided,
        len(state.players),
    )
    return BuildResult(frame=frame, state=state, n_undecided=n_undecided)
