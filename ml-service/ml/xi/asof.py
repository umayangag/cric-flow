"""As-of serving: the rating state as it stood before a date, and the proof it matches
the training frame (H-8).

The serving artifact holds ratings through today, which is what a live prediction wants
and what a backtest must not have -- its team Elo above all carries the results of the
matches being scored. P-0 had to freeze the artifact by hand
(``scripts/experiments/xi/freeze_ratings.py``, retired by this module). ``AsOfRatings``
instead advances a fresh state through a match source and answers ``state_as_of(d)``:
every match strictly before ``d`` folded in, nothing at ``d`` or after -- provably,
because asking for a date the state has already passed raises instead of guessing.
Ascending queries share one pass, so a chronological backtest costs one sweep of the
source, not one per match.
"""

from __future__ import annotations

import itertools
import logging
from datetime import date
from typing import Callable, Dict, Iterator, List, Optional

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi import simulator
from ml.xi.biography import BirthDates
from ml.xi.performance import PerformanceModels
from ml.xi.ratings import RatingState
from ml.xi.rows import build_match_rows
from ml.xi.sources import MatchRecord, MatchSource

logger = logging.getLogger(__name__)

#: Comparisons tighter than this are float noise; the two passes run the same operations
#: in the same order, so genuine divergence shows up orders of magnitude above it.
PARITY_TOLERANCE = 1e-9


class AsOfRatings:
    """A rating state advancing through a source, answering "ratings as of date D".

    Folding all matches strictly before ``d`` is day-close by construction: a date
    boundary can never split a day, so the same-day rule the training pass enforces holds
    here too.
    """

    def __init__(self, source: MatchSource, gender_split_context: bool = False):
        self.state = RatingState(gender_split_context=gender_split_context, birth_dates=source.birth_dates())
        self._matches: Iterator[MatchRecord] = source.iter_matches()
        self._next: Optional[MatchRecord] = next(self._matches, None)
        self._folded_through: Optional[date] = None

    def state_as_of(self, as_of: date) -> RatingState:
        """The state with every match strictly before ``as_of`` folded in.

        Raises when the state already contains a match at or after ``as_of`` -- the pass
        cannot run backwards, and silently serving a state that has seen the future is the
        exact defect this class exists to make impossible.
        """
        if self._folded_through is not None and as_of <= self._folded_through:
            raise ValueError(
                f"as-of state already contains matches through {self._folded_through}; "
                f"cannot serve ratings as of {as_of} without a fresh pass"
            )
        while self._next is not None and self._next.match_date < as_of:
            self.state.update(self._next)
            self._folded_through = self._next.match_date
            self._next = next(self._matches, None)
        return self.state


class AsOfServer:
    """Long-lived ``ratings_as_of`` for the serving path.

    A backtest walks matches in date order, so one advancing pass serves the whole run; a
    request that goes backwards pays for a fresh pass over the source, which is logged
    because it is the caller ordering work badly, not this class.
    """

    def __init__(self, source_factory: Callable[[], MatchSource], gender_split_context: bool = False):
        self._source_factory = source_factory
        self._gender_split_context = gender_split_context
        self._asof: Optional[AsOfRatings] = None

    def state_as_of(self, as_of: date) -> RatingState:
        if self._asof is None:
            self._asof = AsOfRatings(self._source_factory(), self._gender_split_context)
        try:
            return self._asof.state_as_of(as_of)
        except ValueError:
            logger.warning("as-of request for %s is behind the running pass; rebuilding from scratch", as_of)
            self._asof = AsOfRatings(self._source_factory(), self._gender_split_context)
            return self._asof.state_as_of(as_of)


class _IteratorSource:
    """Adapter presenting an already-open match iterator as a MatchSource, with the
    birth dates of the source it was opened from."""

    def __init__(self, matches: Iterator[MatchRecord], birth_dates: BirthDates):
        self._matches = matches
        self._birth_dates = birth_dates

    def iter_matches(self) -> Iterator[MatchRecord]:
        return self._matches

    def birth_dates(self) -> BirthDates:
        return self._birth_dates


def serving_parity(
    source: MatchSource,
    frame: pd.DataFrame,
    player_frame: pd.DataFrame,
    last_n: int = 50,
    gender_split_context: bool = False,
    performance_models: Optional[Dict[str, PerformanceModels]] = None,
) -> Dict:
    """Rebuild the last ``last_n`` matches' rows from the as-of serving path and compare
    them with the training frame's rows (H-8).

    The training pass and ``AsOfRatings`` evolve their states through different code --
    day-close buffering there, a strict date threshold here -- so agreement is a real
    check on both, while the row assembly is shared (``ml.xi.rows``) so the two cannot
    even in principle spell a column differently (the D-4 defect class).

    With ``performance_models`` (per format) the check extends to what is served: the
    model's pre-toss prediction for the rebuilt rows must equal its prediction for the
    frame's rows, output by output, and so must the simulator's draws from them at a fixed
    seed (every total and every player's runs).
    """
    ordered = frame.sort_values(["match_date", "match_id"], kind="stable")
    wanted = list(ordered.match_id.tail(last_n))
    wanted_set = set(wanted)
    frame_rows = {row.match_id: row for row in ordered[ordered.match_id.isin(wanted_set)].itertuples()}
    player_rows_by_match: Dict[str, pd.DataFrame] = {
        match_id: group
        for match_id, group in player_frame[player_frame.match_id.isin(wanted_set)].groupby("match_id", sort=False)
    }

    win_cols = (
        C.XI_FEATURE_COLS
        + C.TEAM_CONTEXT_COLS
        + C.SIMULATION_CONTEXT_COLS
        + C.FIXTURE_CONTEXT_COLS
        + C.INNINGS_OUTCOME_COLS
    )
    player_cols = C.PLAYER_MATCH_FEATURE_COLS + C.PLAYER_MATCH_TARGET_COLS
    matches_compared = 0
    win_rows_compared = 0
    player_rows_compared = 0
    predictions_compared = 0
    simulations_compared = 0
    max_abs_difference = 0.0
    mismatches: List[str] = []
    performance_models = performance_models or {}

    matches_for_lookup, matches_for_state = itertools.tee(source.iter_matches())
    asof = AsOfRatings(_IteratorSource(matches_for_state, source.birth_dates()), gender_split_context)
    for match in matches_for_lookup:
        # Advance on every match so the two tee'd iterators stay at most a day apart.
        state = asof.state_as_of(match.match_date)
        if match.match_id not in wanted_set or match.outcome is None:
            continue
        matches_compared += 1
        rebuilt_win, rebuilt_players = build_match_rows(state, match)
        expected = frame_rows[match.match_id]
        for col in win_cols:
            diff = abs(float(rebuilt_win[col]) - float(getattr(expected, col)))
            max_abs_difference = max(max_abs_difference, diff)
            if diff > PARITY_TOLERANCE:
                mismatches.append(f"match {match.match_id} win column {col}: {diff:.3g}")
        win_rows_compared += 1
        expected_players = player_rows_by_match.get(match.match_id)
        expected_by_key = (
            {} if expected_players is None else {(r.side, r.player_key): r for r in expected_players.itertuples()}
        )
        for rebuilt in rebuilt_players:
            expected_row = expected_by_key.get((rebuilt["side"], rebuilt["player_key"]))
            if expected_row is None:
                mismatches.append(f"match {match.match_id}: no frame row for {rebuilt['player_key']}")
                continue
            for col in player_cols:
                diff = abs(float(rebuilt[col]) - float(getattr(expected_row, col)))
                max_abs_difference = max(max_abs_difference, diff)
                if diff > PARITY_TOLERANCE:
                    mismatches.append(f"match {match.match_id} player {rebuilt['player_key']} {col}: {diff:.3g}")
            player_rows_compared += 1
        model = performance_models.get(match.format_code)
        if model is not None and expected_players is not None and rebuilt_players:
            diff = _prediction_difference(model, pd.DataFrame(rebuilt_players), expected_players)
            max_abs_difference = max(max_abs_difference, diff)
            predictions_compared += len(rebuilt_players)
            if diff > PARITY_TOLERANCE:
                mismatches.append(f"match {match.match_id} performance prediction: {diff:.3g}")
            if match.format_code in simulator.SIMULATED_FORMATS:
                diff = _simulation_difference(
                    model, rebuilt_win, expected, pd.DataFrame(rebuilt_players), expected_players
                )
                max_abs_difference = max(max_abs_difference, diff)
                simulations_compared += 1
                if diff > PARITY_TOLERANCE:
                    mismatches.append(f"match {match.match_id} simulation: {diff:.3g}")

    if matches_compared < len(wanted_set):
        mismatches.append(f"source yielded {matches_compared} of {len(wanted_set)} matches the frame holds")
    report = {
        "matches_compared": matches_compared,
        "win_rows_compared": win_rows_compared,
        "player_rows_compared": player_rows_compared,
        "performance_predictions_compared": predictions_compared,
        "simulations_compared": simulations_compared,
        "max_abs_difference": float(max_abs_difference),
        "mismatches": mismatches[:20],
        "passed": not mismatches,
    }
    logger.info(
        "serving parity (H-8): %d matches, %d player rows, %d performance predictions, %d simulations, "
        "max diff %.3g, %s",
        matches_compared,
        player_rows_compared,
        predictions_compared,
        simulations_compared,
        max_abs_difference,
        "passed" if report["passed"] else f"FAILED ({len(mismatches)} mismatches)",
    )
    return report


def _prediction_difference(model: PerformanceModels, rebuilt: pd.DataFrame, expected: pd.DataFrame) -> float:
    """Largest difference, over every served output, between the model's pre-toss
    prediction for the rebuilt rows and for the frame's rows of the same players."""
    key = ["side", "player_key"]
    aligned = expected.set_index(key).loc[list(zip(rebuilt.side, rebuilt.player_key))].reset_index()
    served, frame = model.predict_marginalised(rebuilt), model.predict_marginalised(aligned)
    largest = 0.0
    for name, value in served.items():
        if isinstance(value, dict):
            for output, arr in value.items():
                largest = max(largest, float(np.max(np.abs(arr - frame[name][output]))))
        else:
            largest = max(largest, float(np.max(np.abs(value - frame[name]))))
    return largest


#: Draws per parity simulation: the outputs are deterministic given the inputs and the
#: seed, so a small draw count proves as much as a large one.
PARITY_SIMULATION_SAMPLES = 200


def _simulation_difference(
    model: PerformanceModels, rebuilt_win: Dict, expected_win, rebuilt: pd.DataFrame, expected: pd.DataFrame
) -> float:
    """Largest difference between the simulator's draws (fixed seed, toss known) from the
    rebuilt rows and from the frame's rows: both sides' totals and every player's runs."""
    key = ["side", "player_key"]
    aligned = expected.set_index(key).loc[list(zip(rebuilt.side, rebuilt.player_key))].reset_index()
    expected_win_row = pd.DataFrame([expected_win._asdict()])
    largest = 0.0
    draws = []
    for players, win_row in ((rebuilt, pd.DataFrame([rebuilt_win])), (aligned, expected_win_row)):
        fixture = simulator.fixtures_from_rows(players, win_row, model.predict_oriented)[0]
        draws.append(
            simulator.simulate_match(
                fixture.team1, fixture.team2, fixture.context, PARITY_SIMULATION_SAMPLES, 0, True, model.simulation
            )
        )
    served, frame = draws
    for team_a, team_b in ((served.team1, frame.team1), (served.team2, frame.team2)):
        largest = max(largest, float(np.max(np.abs(team_a.total - team_b.total))))
        largest = max(largest, float(np.max(np.abs(team_a.runs - team_b.runs))))
    return largest
