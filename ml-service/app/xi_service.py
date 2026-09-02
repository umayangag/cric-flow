"""Serving-side glue for the XI-responsive win model, the player-performance model and the
match simulator: a lazily loaded ``XiStore`` and the functions the ``/xi/*``,
``/performance/*`` and ``/simulate`` routes call. Kept free of FastAPI so it is testable
in-process."""

from __future__ import annotations

import json
import os
import threading
from datetime import date
from typing import Dict, List, Optional

import pandas as pd

from app.errors import error_payload
from app.logging import get_struct_logger
from app.models.xi import (
    PerformancePredictRequest,
    PerformancePredictResponse,
    PerformanceRange,
    PlayerPerformance,
    RatingsFreshness,
    SimulatedMargin,
    SimulatedPlayer,
    SimulatedScorecardLine,
    SimulatedSide,
    SimulatedTotal,
    SimulatedWinProbability,
    SimulateRequest,
    SimulateResponse,
    WicketDistribution,
    XiConstraints,
    XiOptimizeRequest,
    XiOptimizeResponse,
    XiStatusResponse,
    XiWinRequest,
    XiWinResponse,
)
from ml.config import get_ratings_max_age_days
from ml.xi import runs, simulator
from ml.xi.asof import AsOfServer
from ml.xi.evaluate import REPORT_NAME as EVALUATE_REPORT_NAME
from ml.xi.optimizer import (
    NOT_OPTIMISED_REASONS,
    OPTIMISED_SELECTION_FORMATS,
    Constraints,
    marginal_values,
    select_xi,
    select_xi_by_ratings,
)
from ml.xi.rows import player_feature_rows, serving_match
from ml.xi.runs import RunArtifactsInvalid
from ml.xi.store import XiStore
from ml.xi.train import REPORT_NAME

logger = get_struct_logger()


def _postgres_as_of_source():
    """Default source for as-of serving: the go-app database, all formats."""
    from ml.db import get_db_connection
    from ml.xi.sources import PostgresSource

    return PostgresSource(get_db_connection())


class XiUnavailable(Exception):
    """Raised when no XI artifacts are loaded; routes map it to 503."""

    def __init__(
        self,
        message: str,
        hint: str = "run `make retrain` then `make reload`",
        code: str = "XI_MODEL_UNAVAILABLE",
    ):
        super().__init__(message)
        self.payload = error_payload(code=code, message=message, hint=hint)


class RatingsStale(XiUnavailable):
    """H-11: the ratings are older than the configured limit, so a live prediction is
    refused rather than answered.

    It is a refusal, not a warning, because the alternative is the failure this plan
    keeps meeting: a number that looks like every other number and is quietly describing
    a squad from a month ago. The code is machine-readable so an operator's tooling can
    act on it without parsing prose, and the message names the step that fixes it.
    """

    def __init__(self, freshness: RatingsFreshness):
        super().__init__(
            f"ratings run through {freshness.ratings_through} "
            f"({freshness.age_days} days old, limit {freshness.max_age_days})",
            hint="run the retrain step, then reload -- or raise XI_RATINGS_MAX_AGE_DAYS if this is deliberate",
            code="RATINGS_STALE",
        )


class XiRegistry:
    """Holds the loaded store. ``reload`` is idempotent and tolerant of missing artifacts."""

    def __init__(self) -> None:
        self._store: Optional[XiStore] = None
        self._report: Optional[dict] = None
        self._models_dir: Optional[str] = None
        self._run_dir: Optional[str] = None
        self._error: Optional[str] = None
        self._lock = threading.Lock()
        self._as_of_server: Optional[AsOfServer] = None
        # A seam, so tests can serve the as-of pass from an in-memory source.
        self.as_of_source_factory = _postgres_as_of_source

    def reload(self, models_dir: str, run_id: Optional[str] = None) -> dict:
        """Load a run and serve it. ``run_id`` names one; without it, whichever run
        ``current`` points at, and if nothing does, the newest run -- which is then
        published, so `retrain` followed by `reload` serves the run just built without
        either step having to pass an id to the other.

        A run that cannot be served is *refused*, and the reason is kept and reported
        (D-6): loading is where an operator can still be told to retrain, and an
        exception swallowed here becomes an IndexError in a prediction an hour later.
        """
        with self._lock:
            self._store, self._report, self._error = None, None, None
            self._as_of_server = None
            # Remembered even when there is nothing to load: L4's report lives under the
            # same root, and reading it from anywhere else is how the two drift.
            self._models_dir = models_dir
            self._run_dir = None

            target = run_id or runs.read_current(models_dir) or runs.newest_run_id(models_dir)
            if target is None:
                logger.info("xi.runs.absent", models_dir=models_dir)
                return self.status().model_dump()
            directory = runs.run_dir(models_dir, target)
            try:
                store = XiStore.load(directory)
                runs.set_current(models_dir, target)
            except RunArtifactsInvalid as e:
                # The refusal is the answer, not a failure to report one.
                self._error = str(e)
                logger.error("xi.artifacts.refused", run_id=target, models_dir=models_dir, error=str(e))
                return self.status().model_dump()
            except Exception as e:  # a corrupt artifact must not take the service down
                self._error = f"run {target}: {e}"
                logger.error(
                    "xi.artifacts.load_failed", run_id=target, directory=directory, error=str(e), exc_info=True
                )
                return self.status().model_dump()

            self._store = store
            self._run_dir = directory
            report_path = os.path.join(directory, REPORT_NAME)
            if os.path.exists(report_path):
                with open(report_path) as fh:
                    self._report = json.load(fh)
            logger.info(
                "xi.artifacts.loaded",
                run_id=target,
                formats=sorted(store.models),
                players=len(store.state.players),
            )
            return self.status().model_dump()

    def store(self, format_code: str) -> XiStore:
        s = self._store
        if s is None:
            raise XiUnavailable("XI win model artifacts are not loaded")
        if not s.has_format(format_code):
            raise XiUnavailable(f"no XI win model for format {format_code!r}; loaded: {sorted(s.models)}")
        return s

    def performance(self, format_code: str, as_of) -> tuple:
        """The store (as of ``as_of`` when given) and the format's performance model."""
        store = self.store_as_of(format_code, as_of)
        if not store.has_performance(format_code):
            raise XiUnavailable(f"no performance model for format {format_code!r}; loaded: {sorted(store.performance)}")
        return store, store.performance[format_code]

    def store_as_of(self, format_code: str, as_of) -> XiStore:
        """The store serving ratings as they stood strictly before ``as_of`` (a backtest's
        view). ``None``, or a date past everything the loaded state holds, serves the
        loaded through-today state unchanged. The advancing pass is shared and sequential:
        a backtest walking matches in date order pays one sweep of the source in total.
        """
        store = self.store(format_code)
        if as_of is None:
            # H-11 applies to live requests only. A backtest names the date it wants
            # served and gets exactly that, so "how old is today's state?" is not a
            # question about it -- refusing one would break the harness for a reason
            # that does not describe it.
            freshness = self.freshness()
            if not freshness.fresh:
                raise RatingsStale(freshness)
            return store
        if store.covers_as_of(as_of):
            return store
        with self._lock:
            if self._as_of_server is None:
                self._as_of_server = AsOfServer(
                    self.as_of_source_factory, gender_split_context=store.state.gender_split_context
                )
            state = self._as_of_server.state_as_of(as_of)
            logger.info("xi.as_of.served", as_of=str(as_of), players=len(state.players))
            return store.with_state(state)

    @property
    def models_dir(self) -> Optional[str]:
        """The artifacts root. ``None`` before the first reload."""
        return self._models_dir

    @property
    def run_dir(self) -> Optional[str]:
        """The loaded run's directory -- where its report lives. ``None`` when nothing loaded."""
        return self._run_dir

    def freshness(self, today: Optional[date] = None) -> RatingsFreshness:
        """H-11's verdict on the loaded state.

        With nothing loaded there is no state to be stale, so the verdict is "not fresh"
        with no age: the request will be refused for the other reason, and inventing an
        age would be inventing a fact.
        """
        limit = get_ratings_max_age_days()
        through = self._store.state.last_date if self._store is not None else None
        if through is None:
            return RatingsFreshness(fresh=False, age_days=None, max_age_days=limit, ratings_through=None)
        age = ((today or date.today()) - through).days
        fresh = limit <= 0 or age <= limit
        return RatingsFreshness(
            fresh=fresh,
            age_days=age,
            max_age_days=limit,
            ratings_through=through.isoformat(),
            code=None if fresh else "RATINGS_STALE",
        )

    def status(self) -> XiStatusResponse:
        s = self._store
        if s is None:
            return XiStatusResponse(
                loaded=False,
                formats=[],
                players=0,
                ratings_through=None,
                report=None,
                error=self._error,
                ratings=self.freshness(),
            )
        manifest = s.manifest
        return XiStatusResponse(
            loaded=True,
            formats=sorted(s.models),
            performance_formats=sorted(s.performance),
            players=len(s.state.players),
            ratings_through=s.state.last_date.isoformat() if s.state.last_date else None,
            report=self._report,
            run_id=None if manifest is None else manifest.run_id,
            manifest=None if manifest is None else manifest.summary(),
            ratings=self.freshness(),
        )


REGISTRY = XiRegistry()


def _keys(ids: List[str]) -> List[str]:
    """The wire's player ids *are* the rating state's keys (registry ids), so this is
    identity -- kept as a named seam because it was a conversion until P-5, and the
    conversion was the bug: ``str(int)`` can never spell a hex registry id."""
    return list(ids)


def _constraints(c: XiConstraints) -> Constraints:
    return Constraints(
        team_size=c.team_size,
        min_bowlers=c.min_bowlers,
        require_keeper=c.require_keeper,
        must_include=_keys(c.must_include),
        must_exclude=_keys(c.must_exclude),
    )


def optimize(req: XiOptimizeRequest, registry: XiRegistry = REGISTRY) -> XiOptimizeResponse:
    # The two policy checks come before the artifacts: whether a format is offered an
    # optimised selection is a rule (H-17), not a property of what happens to be loaded.
    if req.objective == "win" and req.format not in OPTIMISED_SELECTION_FORMATS:
        raise XiUnavailable(
            f"format {req.format!r} is not offered an optimised selection: "
            f"{NOT_OPTIMISED_REASONS.get(req.format, 'unknown format')}; "
            f"ask for objective='ratings'. Optimised formats: {sorted(OPTIMISED_SELECTION_FORMATS)}"
        )
    if req.objective == "win" and not req.opponent_player_ids:
        raise XiUnavailable("objective='win' needs the opposing XI: opponent_player_ids is empty")
    store = registry.store_as_of(req.format, req.as_of)
    pool, opponent = _keys(req.pool_player_ids), _keys(req.opponent_player_ids)
    unknown = [pid for pid, known in zip(req.pool_player_ids, store.known_players(pool)) if not known]
    if unknown:
        logger.warning("xi.optimize.unknown_players", format=req.format, count=len(unknown), ids=unknown[:10])
    if req.objective == "ratings":
        return _rating_ordered(req, store, pool, unknown)
    result = select_xi(
        store,
        req.format,
        pool,
        opponent,
        constraints=_constraints(req.constraints),
        team_is_team1=req.team_is_team1,
        max_evaluations=req.max_evaluations,
    )
    mv = marginal_values(store, req.format, result.selected, opponent, team_is_team1=req.team_is_team1)
    logger.info(
        "xi.optimize.done",
        format=req.format,
        pool=len(pool),
        p=round(result.win_probability, 4),
        evaluations=result.evaluations,
    )
    return XiOptimizeResponse(
        selected_player_ids=list(result.selected),
        objective="win",
        optimised=True,
        win_probability=result.win_probability,
        evaluations=result.evaluations,
        improved_over_seed=result.improved_over_seed,
        unknown_player_ids=unknown,
        marginal_values=dict(mv),
    )


def _rating_ordered(req: XiOptimizeRequest, store: XiStore, pool: List[str], unknown: List[int]) -> XiOptimizeResponse:
    """The pick for a format whose objective does not rank (H-17): rating order under the
    same constraints, no model evaluated, and marked as not optimised so the API and the
    UI can say so."""
    selected = select_xi_by_ratings(store, req.format, pool, constraints=_constraints(req.constraints))
    logger.info("xi.optimize.rating_ordered", format=req.format, pool=len(pool), selected=len(selected))
    return XiOptimizeResponse(
        selected_player_ids=list(selected),
        objective="ratings",
        optimised=False,
        win_probability=None,
        evaluations=0,
        improved_over_seed=0.0,
        unknown_player_ids=unknown,
        marginal_values={},
    )


def predict_win(req: XiWinRequest, registry: XiRegistry = REGISTRY) -> XiWinResponse:
    store = registry.store_as_of(req.format, req.as_of)
    t1, t2 = _keys(req.team1_player_ids), _keys(req.team2_player_ids)
    objective = store.objective_probability(
        req.format, store.side_vectors(req.format, t1), store.side_vectors(req.format, t2)
    )
    display = store.display_probability(
        req.format,
        t1,
        t2,
        team1_name=None if req.team1_id is None else str(req.team1_id),
        team2_name=None if req.team2_id is None else str(req.team2_id),
        venue=None if req.venue_id is None else str(req.venue_id),
        team1_bats_first=req.team1_bats_first,
    )
    return XiWinResponse(team1_win_probability=display, objective_probability=objective)


def _optional_str(value: Optional[int]) -> Optional[str]:
    """Team and venue ids stay integers: they key team-level context, not the player state."""
    return None if value is None else str(value)


def _fixture_rows(store: XiStore, req: PerformancePredictRequest) -> tuple:
    """The fixture's win row and player rows from the serving state -- the same assembly
    the training frame uses (``ml.xi.rows``) -- and the ids the state has never seen."""
    t1, t2 = _keys(req.team1_player_ids), _keys(req.team2_player_ids)
    unknown = [
        pid
        for pid, known in zip(req.team1_player_ids + req.team2_player_ids, store.known_players(t1 + t2))
        if not known
    ]
    match = serving_match(
        req.format,
        t1,
        t2,
        _optional_str(req.team1_id),
        _optional_str(req.team2_id),
        _optional_str(req.venue_id),
        store.state.last_date,
    )
    win_row, player_rows = player_feature_rows(store.state, match)
    rows = pd.DataFrame(player_rows)
    rows["team_side"] = rows.side  # which eleven the player belongs to, whatever the innings
    return win_row, rows, unknown


def predict_performance(req: PerformancePredictRequest, registry: XiRegistry = REGISTRY) -> PerformancePredictResponse:
    """Per-player performance distributions for two elevens: the same feature rows the
    training frame is built from (``ml.xi.rows``), predicted by the format's L2-B model,
    averaged over both batting orders unless the toss is known."""
    store, model = registry.performance(req.format, req.as_of)
    _, rows, unknown = _fixture_rows(store, req)
    if req.team1_bats_first is None:
        prediction = model.predict_marginalised(rows)
    else:
        # ``bats_first`` is per player: team1's players bat first exactly when team1 does.
        if not req.team1_bats_first:
            rows["side"] = 3 - rows.side
        prediction = model.predict_oriented(rows, None)
    players = [_player_performance(rows, prediction, i) for i in range(len(rows))]
    logger.info("performance.predict.done", format=req.format, players=len(players), unknown=len(unknown))
    return PerformancePredictResponse(
        players=players, innings_marginalised=req.team1_bats_first is None, unknown_player_ids=unknown
    )


def _player_performance(rows: pd.DataFrame, prediction: Dict, i: int) -> PlayerPerformance:
    def quantile_range(target: str) -> PerformanceRange:
        q = prediction[target]["quantiles"][i]
        return PerformanceRange(q10=float(q[0]), median=float(q[1]), q90=float(q[2]))

    wickets = prediction["wickets"]
    return PlayerPerformance(
        player_id=str(rows.player_key.iloc[i]),
        side=int(rows.team_side.iloc[i]),
        p_bats=float(prediction["p_bats"][i]),
        p_bowls=float(prediction["p_bowls"][i]),
        runs=quantile_range("runs"),
        balls_faced=quantile_range("balls_faced"),
        runs_conceded=quantile_range("runs_conceded"),
        wickets=WicketDistribution(
            expected=float(wickets["mean"][i]),
            p0=float(wickets["p0"][i]),
            p1=float(wickets["p1"][i]),
            p2_plus=float(wickets["p2plus"][i]),
        ),
        catches_expected=float(prediction["catches"]["mean"][i]),
    )


def simulate(req: SimulateRequest, registry: XiRegistry = REGISTRY) -> SimulateResponse:
    """Draw the match ``n_samples`` times from the format's L2-B forecasts for the two
    elevens (``ml.xi.simulator``): totals, per-player ranges, the median-band scorecard,
    margins and P(win) all from the same draws, with the display model's P(win) for the
    same fixture beside it and E2's rule deciding which is the headline."""
    if req.format not in simulator.SIMULATED_FORMATS:
        raise simulator.SimulationUnavailable(
            f"format {req.format!r} has no innings length; the simulator runs for {list(simulator.SIMULATED_FORMATS)}"
        )
    store, model = registry.performance(req.format, req.as_of)
    win_row, rows, unknown = _fixture_rows(store, req)
    fixture = simulator.fixtures_from_rows(rows, pd.DataFrame([win_row]), model.predict_oriented)[0]
    draws = simulator.simulate_match(
        fixture.team1, fixture.team2, fixture.context, req.n_samples, req.seed, req.team1_bats_first, model.simulation
    )
    summary = simulator.summarize(draws)
    display = store.display_probability(
        req.format,
        _keys(req.team1_player_ids),
        _keys(req.team2_player_ids),
        team1_name=_optional_str(req.team1_id),
        team2_name=_optional_str(req.team2_id),
        venue=_optional_str(req.venue_id),
        team1_bats_first=req.team1_bats_first,
    )
    simulated = summary["win"]["team1"] + 0.5 * summary["win"]["tie"]
    simulator_headline = simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED.get(req.format, False)
    logger.info(
        "simulate.done",
        format=req.format,
        n=req.n_samples,
        p_simulated=round(simulated, 4),
        p_display=round(display, 4),
        team1_total=round(summary["team1"]["total"]["median"], 1),
        team2_total=round(summary["team2"]["total"]["median"], 1),
        unknown=len(unknown),
    )
    return SimulateResponse(
        format=req.format,
        n_samples=req.n_samples,
        seed=req.seed,
        toss_marginalised=summary["toss_marginalised"],
        team1=_simulated_side(summary["team1"], 1),
        team2=_simulated_side(summary["team2"], 2),
        win_probability=SimulatedWinProbability(
            simulated=simulated,
            p_tie=summary["win"]["tie"],
            display=display,
            headline=simulated if simulator_headline else display,
            headline_source="simulator" if simulator_headline else "display",
        ),
        margin=SimulatedMargin(
            **{
                key: (PerformanceRange(**value) if isinstance(value, dict) else value)
                for key, value in summary["margin"].items()
            }
        ),
        unknown_player_ids=unknown,
    )


def _simulated_side(side: Dict, team_side: int) -> SimulatedSide:
    return SimulatedSide(
        total=SimulatedTotal(**side["total"]),
        extras_scorecard=side["extras"]["scorecard"],
        extras_spread_share=side["extras"]["spread_share"],
        wickets_lost=PerformanceRange(**side["wickets_lost"]),
        players=[
            SimulatedPlayer(
                player_id=str(p["player_key"]),
                side=team_side,
                p_bats=p["p_bats"],
                p_bowls=p["p_bowls"],
                runs=PerformanceRange(**p["runs"]),
                balls_faced=PerformanceRange(**p["balls_faced"]),
                wickets=PerformanceRange(**p["wickets"]),
                runs_conceded=PerformanceRange(**p["runs_conceded"]),
                balls_bowled=PerformanceRange(**p["balls_bowled"]),
                scorecard=SimulatedScorecardLine(**p["scorecard"]),
                spread_share=p["spread_share"],
                spread_runs=p["spread_runs"],
            )
            for p in side["players"]
        ],
    )


def status(registry: XiRegistry = REGISTRY) -> XiStatusResponse:
    return registry.status()


def evaluate_report(registry: XiRegistry = REGISTRY) -> dict:
    """L4's report (``xi_evaluate_report.json``), as `make evaluate` last wrote it.

    Read from the directory the artifacts were loaded from -- which is
    ``ML_SERVICE_OUTPUT_DIR`` before it is anything else -- so the report and the models it
    describes can never come from two different places. Resolving it independently through
    ``ml.config.default_artifacts_dir()`` looked equivalent and was not: that is the last
    fallback in the chain, a relative path that is wrong inside the container.

    Read from disk on every request rather than cached at reload: the harness is run on
    demand, and a report that is one release stale because nobody restarted the service
    would be exactly the kind of number nobody can trace.
    """
    directory = registry.models_dir
    if directory is None:
        raise XiUnavailable(
            "artifacts have never been loaded, so there is nowhere to read the report from",
            hint="POST /admin/reload, then run `make evaluate` if the report is missing",
        )
    path = os.path.join(directory, EVALUATE_REPORT_NAME)
    if not os.path.exists(path):
        raise XiUnavailable(
            f"no evaluation report at {path}",
            hint="run `make evaluate` -- the harness writes the report the backtest surfaces read",
        )
    with open(path) as fh:
        report = json.load(fh)
    logger.info("xi.evaluate_report.served", path=path, formats=sorted(report.get("formats", {})))
    return report


def loaded_formats(registry: XiRegistry = REGISTRY) -> Dict[str, List[str]]:
    return {"loaded_xi_formats": registry.status().formats}
