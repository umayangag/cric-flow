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
    state = RatingState()
    rows = []
    n_undecided = 0
    for i, match in enumerate(source.iter_matches()):
        if state.last_date is not None and match.match_date < state.last_date:
            raise ValueError(
                f"source is not in date order: {match.match_id} ({match.match_date}) after {state.last_date}"
            )
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
        state.update(match)
        if progress and i % 1000 == 0:
            progress(i)
    frame = pd.DataFrame(rows)
    logger.info(
        "rating pass: %d training rows, %d undecided matches, %d players", len(frame), n_undecided, len(state.players)
    )
    return BuildResult(frame=frame, state=state, n_undecided=n_undecided)
