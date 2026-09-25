"""As-of serving: the rating state as it stood before a date, and the proof that what is
served matches the training frame (H-8).

The serving artifact holds ratings through today, which is what a live prediction wants
and what a backtest must not have -- its team Elo above all carries the results of the
matches being scored. P-0 had to freeze the artifact by hand
(``scripts/experiments/xi/freeze_ratings.py``, retired by this module). ``AsOfRatings``
instead advances a fresh state through a match source and answers ``state_as_of(d)``:
every match strictly before ``d`` folded in, nothing at ``d`` or after -- provably,
because asking for a date the state has already passed raises instead of guessing.
Ascending queries share one pass, so a chronological backtest costs one sweep of the
source, not one per match.

``serving_parity`` is the proof, and since EVAL-10 it reaches the artifact: the store it
serves from has been written to a run directory and loaded back through ``XiStore.load``,
so the rating payload, the win artifacts' column lists and the performance pickles are the
ones under test, and the numbers compared are the ones the routes answer with --
``display_probability`` and ``objective_probability`` -- not only the rows they are built
from.

    python -m ml.xi.asof --postgres                # the run `current` points at
    python -m ml.xi.asof --postgres --run <run_id> # a named run under the artifacts root

checks a run on disk against a fresh pass over its own data (``make serving-parity``).
"""

from __future__ import annotations

import argparse
import itertools
import logging
import math
import os
import sys
from datetime import date, datetime, timezone
from typing import Callable, Dict, Iterator, List, Optional, Sequence

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi import runs, simulator
from ml.xi.biography import BirthDates
from ml.xi.performance import PerformanceModels
from ml.xi.ratings import RatingState
from ml.xi.rows import build_match_rows
from ml.xi.sources import MatchRecord, MatchSource, VenueCountries
from ml.xi.store import FormatModels, XiStore, save_models, save_performance, save_ratings, state_shape
from ml.xi.train import marginalised_probabilities

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
        self.state = RatingState(
            gender_split_context=gender_split_context,
            birth_dates=source.birth_dates(),
            venue_countries=source.venue_countries(),
        )
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
    birth dates and venue countries of the source it was opened from."""

    def __init__(self, matches: Iterator[MatchRecord], birth_dates: BirthDates, venue_countries: VenueCountries):
        self._matches = matches
        self._birth_dates = birth_dates
        self._venue_countries = venue_countries

    def iter_matches(self) -> Iterator[MatchRecord]:
        return self._matches

    def birth_dates(self) -> BirthDates:
        return self._birth_dates

    def venue_countries(self) -> VenueCountries:
        return self._venue_countries


def serving_parity(
    source: MatchSource,
    frame: pd.DataFrame,
    player_frame: pd.DataFrame,
    last_n: int = 50,
    gender_split_context: bool = False,
    store: Optional[XiStore] = None,
) -> Dict:
    """Rebuild the last ``last_n`` matches from the as-of serving path and compare what is
    served with the training frame (H-8).

    The training pass and ``AsOfRatings`` evolve their states through different code --
    day-close buffering there, a strict date threshold here -- so agreement is a real
    check on both, while the row assembly is shared (``ml.xi.rows``) so the two cannot
    even in principle spell a column differently (the D-4 defect class). That is the row
    comparison, and on its own it compares ``rows.py`` with ``rows.py`` (EVAL-10).

    ``store`` is what makes the check reach the artifact. It must have come through
    ``XiStore.load`` -- ``round_trip_store`` for models still in memory -- so a run this
    code cannot serve has already been refused by name (D-6) before anything is compared.
    With it, three more things are held to the tolerance for the same matches:

    * the served probabilities -- ``display_probability``, the number ``/xi/predict-win``
      shows, and ``objective_probability``, the number ``/xi/optimize`` maximises -- from
      the store over the as-of state, exactly as ``XiRegistry`` answers a backtest, against
      ``marginalised_probabilities`` of the artifact's models on the frame's own row. This
      is where the store's row assembly (``row.get(c, 0.0)`` over the artifact's own column
      list, ``serving_match`` stamped with the state's date) and the artifact's column
      contract meet the training frame, which the loader's list cannot check;
    * the same two probabilities from the loaded through-today state, as a live request is
      answered, against the same models over the state the as-of pass ends on -- the same
      cricket, folded by different code and round-tripped through the artifact on one side
      only. A payload that does not carry an accumulator the served number reads (the D-6
      shape, one level above the list ``_check_payload_shape`` walks) shows here and
      nowhere else, because the as-of comparison replaces the loaded state;
    * the performance model's pre-toss prediction for the rebuilt rows against its
      prediction for the frame's rows, output by output, and the simulator's draws from
      them at a fixed seed (every total and every player's runs) -- from the store's own
      loaded performance artifact.
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
        + C.TOSS_COLS
        + C.SIMULATION_CONTEXT_COLS
        + C.FIXTURE_CONTEXT_COLS
        + C.STAKES_COLS
        + C.INNINGS_OUTCOME_COLS
    )
    player_cols = C.PLAYER_MATCH_FEATURE_COLS + C.PLAYER_MATCH_TARGET_COLS
    matches_compared = 0
    win_rows_compared = 0
    player_rows_compared = 0
    predictions_compared = 0
    simulations_compared = 0
    served_probabilities_compared = 0
    artifact_probabilities_compared = 0
    max_abs_difference = 0.0
    mismatches: List[str] = []
    performance_models: Dict[str, PerformanceModels] = store.performance if store is not None else {}
    served_fixtures: List[MatchRecord] = []

    matches_for_lookup, matches_for_state = itertools.tee(source.iter_matches())
    asof = AsOfRatings(
        _IteratorSource(matches_for_state, source.birth_dates(), source.venue_countries()), gender_split_context
    )
    for match in matches_for_lookup:
        # Advance on every match so the two tee'd iterators stay at most a day apart.
        state = asof.state_as_of(match.match_date)
        if match.match_id not in wanted_set or match.outcome is None:
            continue
        matches_compared += 1
        rebuilt_win, rebuilt_players = build_match_rows(state, match)
        expected = frame_rows[match.match_id]
        for col in win_cols:
            diff = _absolute_difference(float(rebuilt_win[col]), float(getattr(expected, col)))
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
                diff = _absolute_difference(float(rebuilt[col]), float(getattr(expected_row, col)))
                max_abs_difference = max(max_abs_difference, diff)
                if diff > PARITY_TOLERANCE:
                    mismatches.append(f"match {match.match_id} player {rebuilt['player_key']} {col}: {diff:.3g}")
            player_rows_compared += 1
        if store is not None and store.has_format(match.format_code):
            served_fixtures.append(match)
            served = _served_probabilities(store.with_state(state), match)
            from_frame = _frame_probabilities(store.models[match.format_code], expected)
            for name, value in served.items():
                diff = abs(value - from_frame[name])
                max_abs_difference = max(max_abs_difference, diff)
                if diff > PARITY_TOLERANCE:
                    mismatches.append(f"match {match.match_id} served {name} vs the frame: {diff:.3g}")
            served_probabilities_compared += 1
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

    if store is not None and served_fixtures:
        # The loaded through-today state against the state the pass ends on: the rest of
        # the source folded in, so both stand at the same date and a live request to
        # either would be stamped the same.
        rebuilt = store.with_state(asof.state_as_of(date.max))
        for match in served_fixtures:
            loaded, fresh = _served_probabilities(store, match), _served_probabilities(rebuilt, match)
            for name, value in loaded.items():
                diff = abs(value - fresh[name])
                max_abs_difference = max(max_abs_difference, diff)
                if diff > PARITY_TOLERANCE:
                    mismatches.append(f"match {match.match_id} {name} from the loaded artifact: {diff:.3g}")
            artifact_probabilities_compared += 1

    report = {
        "matches_compared": matches_compared,
        "win_rows_compared": win_rows_compared,
        "player_rows_compared": player_rows_compared,
        "served_probabilities_compared": served_probabilities_compared,
        "artifact_probabilities_compared": artifact_probabilities_compared,
        "performance_predictions_compared": predictions_compared,
        "simulations_compared": simulations_compared,
        "run_id": None if store is None or store.manifest is None else store.manifest.run_id,
        "max_abs_difference": float(max_abs_difference),
        "mismatches": mismatches[:20],
        "passed": not mismatches,
    }
    logger.info(
        "serving parity (H-8): %d matches, %d player rows, %d served probabilities, %d from the loaded artifact, "
        "%d performance predictions, %d simulations, max diff %.3g, %s",
        matches_compared,
        player_rows_compared,
        served_probabilities_compared,
        artifact_probabilities_compared,
        predictions_compared,
        simulations_compared,
        max_abs_difference,
        "passed" if report["passed"] else f"FAILED ({len(mismatches)} mismatches)",
    )
    return report


def _served_probabilities(store: XiStore, match: MatchRecord) -> Dict[str, float]:
    """The two numbers the routes answer with for the match's fixture, toss unknown, from
    the store's own row assembly: the display probability ``/xi/predict-win`` shows and
    the objective ``/xi/optimize`` maximises. The keys are the frame's player keys, which
    are the registry ids go-app sends (``xi_service._keys`` is the identity)."""
    fmt = match.format_code
    return {
        "display probability": store.display_probability(
            fmt, match.team1_players, match.team2_players, match.team1, match.team2, match.venue
        ),
        "objective probability": store.objective_probability(
            fmt, store.side_vectors(fmt, match.team1_players), store.side_vectors(fmt, match.team2_players)
        ),
    }


def _frame_probabilities(models: FormatModels, expected) -> Dict[str, float]:
    """The same two numbers from the frame's row of the match, as the harness scores
    them: both batting orders averaged over the contract's column lists."""
    row = pd.DataFrame([expected._asdict()])
    return {
        "display probability": float(marginalised_probabilities(models.display, row, C.DISPLAY_FEATURE_COLS)[0]),
        "objective probability": float(marginalised_probabilities(models.objective, row, C.XI_FEATURE_COLS)[0]),
    }


#: The run id a harness round trip writes under; never published, and named so a refusal
#: quoting it reads as what it is.
ROUND_TRIP_RUN_ID = "h8-round-trip"


def round_trip_store(
    state: RatingState,
    win_models: Dict[str, FormatModels],
    performance_models: Dict[str, PerformanceModels],
    directory: str,
) -> XiStore:
    """Write the state and models as a run directory and load them back through
    ``XiStore.load``: the artifact contract end to end, the way ``retrain`` writes and
    ``reload`` reads, so the store the parity check serves from is a store the way the
    service makes one and never the objects still in memory.

    The manifest carries what the loader asserts -- the run id, the date the state runs
    through and the state's shape -- and says, rather than blanks, what a round trip does
    not have: no cutoff, because nothing was held out, and no dataset digest, because the
    caller holds the source and this writes models already in memory. The digest block is
    still written, naming that absence (``runs.no_dataset_digest``): the loader requires
    one, and a run that never had a dataset must not be refused as though it were a run
    written before the digest existed (EVAL-12). The directory is the caller's to discard.
    """
    if state.last_date is None:
        raise ValueError("the rating state consumed no matches, so there is no run to round-trip")
    if not win_models:
        raise ValueError("no format fitted a win model, so there is nothing for the parity check to serve")
    os.makedirs(directory, exist_ok=True)
    save_ratings(state, directory)
    for models in win_models.values():
        save_models(models, directory)
    for format_code, model in performance_models.items():
        save_performance(model, format_code, directory)
    runs.write_manifest(
        directory,
        runs.RunManifest(
            run_id=ROUND_TRIP_RUN_ID,
            created_at=datetime.now(timezone.utc).isoformat(),
            cutoff="",
            ratings_through=state.last_date.isoformat(),
            dataset_sha=runs.UNKNOWN,
            dataset_digest=runs.no_dataset_digest(
                "a harness round trip: the models were already in memory and no source was walked"
            ),
            git_sha=runs.git_sha(),
            state_shape=state_shape(state),
            formats=sorted(win_models),
        ),
    )
    return XiStore.load(directory)


def _absolute_difference(rebuilt: float, expected: float) -> float:
    """How far a rebuilt value is from the frame's. Two unobserved values agree: a decided
    match with no deliveries carries NaN innings outcomes on both paths (FEAT-03), and
    ``abs(nan - nan)`` would neither trip the tolerance nor register as agreement."""
    if math.isnan(rebuilt) and math.isnan(expected):
        return 0.0
    return abs(rebuilt - expected)


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


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    src = p.add_mutually_exclusive_group(required=True)
    src.add_argument("--cricsheet-dir", help="directory of Cricsheet JSON files")
    src.add_argument("--postgres", action="store_true", help="read the go-app database (POSTGRES_* env vars)")
    p.add_argument(
        "--birth-dates",
        default=None,
        help="archive path only: CSV of player_key,birth_date written by `python -m ml.xi.biography --export`",
    )
    p.add_argument("--out", default=None, help="artifacts root (default: ml.config.default_artifacts_dir())")
    p.add_argument("--run", default=None, help="run id under the artifacts root (default: the run `current` names)")
    p.add_argument("--last-n", type=int, default=50, help="how many of the most recent matches to compare")
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    """Check a run on disk against a fresh pass over its own data: exit 0 when parity
    holds, 1 when it does not, 2 when the run cannot be checked -- refused by the loader,
    or trained on other cricket than the source now holds, in which case the through-today
    comparison would measure the data and not the artifact, so it is not run."""
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    from ml.xi.builder import build

    if args.cricsheet_dir:
        from ml.xi.sources import CricsheetJsonSource
        from ml.xi.train import _international_teams_from_config

        international_teams = _international_teams_from_config()

        def source_factory() -> MatchSource:
            return CricsheetJsonSource(args.cricsheet_dir, international_teams, birth_dates_path=args.birth_dates)

    else:
        from ml.db import get_db_connection
        from ml.xi.sources import PostgresSource

        connection = get_db_connection()

        def source_factory() -> MatchSource:
            return PostgresSource(connection)

    artifacts_dir = args.out
    if artifacts_dir is None:
        from ml.config import default_artifacts_dir

        artifacts_dir = default_artifacts_dir()
    run_id = args.run or runs.read_current(artifacts_dir)
    if run_id is None:
        logger.error("no run named and nothing published under %s; name one with --run", artifacts_dir)
        return 2
    try:
        store = XiStore.load(runs.run_dir(artifacts_dir, run_id))
    except runs.RunArtifactsInvalid as exc:
        logger.error("run %s cannot be served, so there is nothing to compare: %s", run_id, exc)
        return 2

    result = build(
        source_factory(),
        progress=lambda i: logger.info("rating pass: %d matches", i),
        gender_split_context=store.state.gender_split_context,
        age_aware_cold_start=store.state.age_aware_cold_start,
    )
    fresh_sha, _ = result.dataset_digest()
    if fresh_sha != store.manifest.dataset_sha:
        logger.error(
            "run %s was trained on dataset %s but the source now holds %s (%d matches); a through-today "
            "comparison would measure the data, not the artifact. Retrain, then check the new run.",
            run_id,
            store.manifest.dataset_sha[:12],
            fresh_sha[:12],
            len(result.frame),
        )
        return 2
    report = serving_parity(
        source_factory(),
        result.frame,
        result.player_frame,
        last_n=args.last_n,
        gender_split_context=store.state.gender_split_context,
        store=store,
    )
    for mismatch in report["mismatches"]:
        logger.error("serving parity: %s", mismatch)
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
