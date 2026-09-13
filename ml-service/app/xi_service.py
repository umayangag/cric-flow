"""Serving-side glue for the XI-responsive win model, the player-performance model and the
match simulator: a lazily loaded ``XiStore`` and the functions the ``/xi/*``,
``/performance/*`` and ``/simulate`` routes call. Kept free of FastAPI so it is testable
in-process."""

from __future__ import annotations

import json
import os
import threading
from dataclasses import dataclass
from datetime import date
from typing import Dict, List, Optional

import pandas as pd

from app.errors import error_payload
from app.logging import get_struct_logger
from app.models.xi import (
    BestAlternativeModel,
    PerformancePredictRequest,
    PerformancePredictResponse,
    PerformanceRange,
    PlayerPerformance,
    PlayerRoles,
    PlayerRolesRequest,
    PlayerRolesResponse,
    PlayerSelectionReasonModel,
    RatingsFreshness,
    ServedRatings,
    SimulatedMargin,
    SimulatedPlayer,
    SimulatedScorecardLine,
    SimulatedSide,
    SimulatedTotal,
    SimulatedWinProbability,
    SimulateRequest,
    SimulateResponse,
    VenueContext,
    WicketDistribution,
    XiConstraintCheck,
    XiConstraints,
    XiOptimizeRequest,
    XiOptimizeResponse,
    XiStatusResponse,
    XiWinRequest,
    XiWinResponse,
)
from ml.config import get_ratings_max_age_days
from ml.xi import glossary, runs, simulator
from ml.xi import roles as R
from ml.xi.asof import AsOfServer
from ml.xi.evaluate import REPORT_NAME as EVALUATE_REPORT_NAME
from ml.xi.optimizer import (
    NOT_OPTIMISED_REASONS,
    OPTIMISED_SELECTION_FORMATS,
    Constraints,
    SelectionReason,
    marginal_values,
    select_xi,
    select_xi_by_ratings,
    selection_reasons,
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


@dataclass(frozen=True)
class ServedRun:
    """One loaded run: its store, its training report and the directory both came from.

    The registry swaps a whole ``ServedRun`` in one reference assignment, so a reader that
    never takes the lock still sees a store beside its own report and directory -- never
    the store of one run with the report of another, and never a store that is half built.
    """

    store: XiStore
    report: Optional[dict]
    directory: str

    @property
    def run_id(self) -> Optional[str]:
        return None if self.store.manifest is None else self.store.manifest.run_id


def _load_served_run(directory: str) -> ServedRun:
    """Everything a run needs before it can serve, built without touching the registry."""
    store = XiStore.load(directory)
    report = None
    report_path = os.path.join(directory, REPORT_NAME)
    if os.path.exists(report_path):
        with open(report_path) as fh:
            report = json.load(fh)
    return ServedRun(store=store, report=report, directory=directory)


class XiRegistry:
    """Holds the run being served. ``reload`` is idempotent and tolerant of missing artifacts."""

    def __init__(self) -> None:
        self._served: Optional[ServedRun] = None
        self._models_dir: Optional[str] = None
        # The last reload's refusal (D-6), kept beside whatever is serving. It names the
        # run that was refused, which is not the run ``status`` reports as loaded.
        self._error: Optional[str] = None
        # Serialises reloads against each other and against the as-of pass. The live path
        # (``store``, ``status``, ``freshness``) never takes it: each reads ``_served`` once.
        self._lock = threading.Lock()
        self._as_of_server: Optional[AsOfServer] = None
        # A seam, so tests can serve the as-of pass from an in-memory source.
        self.as_of_source_factory = _postgres_as_of_source

    def reload(self, models_dir: str, run_id: Optional[str] = None) -> dict:
        """Load a run and serve it. ``run_id`` names one; without it, whichever run
        ``current`` points at, and if nothing does, the newest run.

        Serving what was published is what startup wants, and it is the *only* caller
        that leaves ``run_id`` empty. The `reload` step does not: ``POST /admin/reload``
        resolves the newest run first (``main._run_to_publish``), because a reload with
        no run named is asking for the run just built, and `current` -- which every
        reload sets -- would otherwise answer with the run already serving.

        A run that cannot be served is *refused*, and the reason is kept and reported
        (D-6): loading is where an operator can still be told to retrain, and an
        exception swallowed here becomes an IndexError in a prediction an hour later.

        A refusal leaves the run that was serving exactly as it was (B-13): the new run
        is built first and swapped in only once it has loaded. (A root with no run at
        all is the one case that unloads -- see below.) The lock is held for the
        whole reload, load included. Nothing on the live path waits for it -- a
        prediction reads ``_served`` once and never locks -- so a slow load blocks no
        request; what it holds off is a second reload, which could otherwise interleave
        its swap and its ``set_current`` with this one's and leave `current` naming a run
        the registry is not serving, and the as-of pass, which must not be built against
        a store that is about to be replaced.
        """
        with self._lock:
            # Remembered even when there is nothing to load: L4's report lives under the
            # same root, and reading it from anywhere else is how the two drift.
            self._models_dir = models_dir
            serving = self.run_id

            target = run_id or runs.read_current(models_dir) or runs.newest_run_id(models_dir)
            if target is None:
                # Not a refusal: the root holds no run at all, so nothing is what the disk
                # says to serve, and what a restart would serve. Serving a run the disk no
                # longer has would put `/xi/status` and `/artifacts/status` at odds.
                self._served, self._as_of_server, self._error = None, None, None
                logger.info("xi.runs.absent", models_dir=models_dir, unloaded=serving)
                return self.status().model_dump()
            directory = runs.run_dir(models_dir, target)
            try:
                loaded = _load_served_run(directory)
                runs.set_current(models_dir, target)
            except RunArtifactsInvalid as e:
                # The refusal is the answer, not a failure to report one -- and not an
                # outage: whatever was serving goes on serving.
                self._error = str(e)
                logger.error(
                    "xi.artifacts.refused", run_id=target, models_dir=models_dir, error=str(e), serving=serving
                )
                return self.status().model_dump()
            except Exception as e:  # a corrupt artifact must not take the service down
                self._error = f"run {target}: {e}"
                logger.error(
                    "xi.artifacts.load_failed",
                    run_id=target,
                    directory=directory,
                    error=str(e),
                    serving=serving,
                    exc_info=True,
                )
                return self.status().model_dump()

            # The swap. One reference, so a concurrent reader sees the old run or the new
            # one and nothing in between; the as-of pass was built for the old store's
            # context and goes with it.
            self._served = loaded
            self._as_of_server = None
            self._error = None
            logger.info(
                "xi.artifacts.loaded",
                run_id=target,
                formats=sorted(loaded.store.models),
                players=len(loaded.store.state.players),
                replaced=serving,
            )
            return self.status().model_dump()

    def store(self, format_code: str) -> XiStore:
        served = self._served
        if served is None:
            raise XiUnavailable("XI win model artifacts are not loaded")
        s = served.store
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
        if as_of is None or store.covers_as_of(as_of):
            # H-11 is a verdict on the through-today state, so it applies whenever that
            # state is what answers -- a live request, or an as-of date past everything
            # the state holds, which the through-today state serves unchanged (SERVE-08).
            # A backtest naming a date the as-of pass serves is not asked "how old is
            # today's state?": it gets the date it named, and refusing it would break the
            # harness for a reason that does not describe it.
            freshness = self.freshness()
            if not freshness.fresh:
                raise RatingsStale(freshness)
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
        served = self._served
        return None if served is None else served.directory

    @property
    def run_id(self) -> Optional[str]:
        """The run being served. ``None`` when nothing loaded."""
        served = self._served
        return None if served is None else served.run_id

    def freshness(self, today: Optional[date] = None) -> RatingsFreshness:
        """H-11's verdict on the loaded state.

        With nothing loaded there is no state to be stale, so the verdict is "not fresh"
        with no age: the request will be refused for the other reason, and inventing an
        age would be inventing a fact.
        """
        limit = get_ratings_max_age_days()
        served = self._served
        through = served.store.state.last_date if served is not None else None
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
        """What is serving, and -- separately -- whether the last reload was refused.

        ``error`` describes the last reload, not the loaded run: with ``loaded`` false it
        is why nothing serves, with ``loaded`` true it is a run that was refused while
        ``run_id`` went on serving (B-13). The message names the run it refused.
        """
        served = self._served
        if served is None:
            return XiStatusResponse(
                loaded=False,
                formats=[],
                players=0,
                ratings_through=None,
                report=None,
                error=self._error,
                ratings=self.freshness(),
            )
        s = served.store
        manifest = s.manifest
        return XiStatusResponse(
            loaded=True,
            formats=sorted(s.models),
            performance_formats=sorted(s.performance),
            players=len(s.state.players),
            ratings_through=s.state.last_date.isoformat() if s.state.last_date else None,
            report=served.report,
            run_id=served.run_id,
            manifest=None if manifest is None else manifest.summary(),
            error=self._error,
            ratings=self.freshness(),
        )


REGISTRY = XiRegistry()


def _keys(ids: List[str]) -> List[str]:
    """The wire's player ids *are* the rating state's keys (registry ids), so this is
    identity -- kept as a named seam because it was a conversion until P-5, and the
    conversion was the bug: ``str(int)`` can never spell a hex registry id."""
    return list(ids)


def _served_ratings(store: XiStore) -> ServedRatings:
    """The stamp every prediction carries: which run answered, and the date its ratings
    run through (P1-5).

    Read off the store that computed the answer -- ``manifest.run_id`` and
    ``state.last_date``, the same two things ``XiRegistry.status`` reports -- so the
    payload and ``/xi/status`` describe a store identically. A store that cannot name
    either is refused rather than stamped with a blank: a prediction without its date is
    the defect the stamp exists to close, and a state holding no matches has nothing to
    predict from anyway.
    """
    run_id = None if store.manifest is None else store.manifest.run_id
    through = store.state.last_date
    if not run_id or through is None:
        raise XiUnavailable(
            "the served rating state cannot name its run or the date it runs through, so no prediction is made from it",
            hint="reload a run written by `make retrain`; a state with no matches carries no date",
        )
    return ServedRatings(run_id=run_id, ratings_through=through.isoformat())


def _selection_reason_models(reasons: Dict[str, SelectionReason]) -> Dict[str, PlayerSelectionReasonModel]:
    """The optimiser's own reason objects as wire models (P1-3). A mapping, not a
    recomputation: nothing is derived here that the selection did not already read."""
    return {
        key: PlayerSelectionReasonModel(
            roles=list(reason.roles),
            selection_rating=reason.selection_rating,
            rating_percentile=reason.rating_percentile,
            pool_size=reason.pool_size,
            best_alternative=(
                None
                if reason.best_alternative is None
                else BestAlternativeModel(
                    player_id=reason.best_alternative.player_key,
                    win_probability_gap=reason.best_alternative.win_probability_gap,
                )
            ),
            best_alternative_note=reason.best_alternative_note,
        )
        for key, reason in reasons.items()
    }


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
    reasons = selection_reasons(
        store,
        req.format,
        result.selected,
        pool,
        opponent,
        constraints=_constraints(req.constraints),
        team_is_team1=req.team_is_team1,
    )
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
        selection_reasons=_selection_reason_models(reasons),
        served_ratings=_served_ratings(store),
    )


def _rating_ordered(req: XiOptimizeRequest, store: XiStore, pool: List[str], unknown: List[int]) -> XiOptimizeResponse:
    """The pick for a format whose objective does not rank (H-17): rating order under the
    same constraints, no model evaluated, and marked as not optimised so the API and the
    UI can say so."""
    selected = select_xi_by_ratings(store, req.format, pool, constraints=_constraints(req.constraints))
    # No opponent, so no alternative is scored: this path maximises nothing and the card
    # must not be handed a win-model number for a format the policy scoped off (P1-3 § 3).
    reasons = selection_reasons(
        store, req.format, selected, pool, constraints=_constraints(req.constraints), team_is_team1=True
    )
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
        selection_reasons=_selection_reason_models(reasons),
        served_ratings=_served_ratings(store),
    )


def _constraint_check(
    store: XiStore, format_code: str, keys: List[str], constraints: Optional[XiConstraints]
) -> Optional[XiConstraintCheck]:
    """Report whether an eleven meets its constraints, without changing it (P1-2).

    Only the counts are computed here; the rule that turns them into "broken" is the
    caller's constraint, echoed back beside them. Both predicates come from ``ml.xi.roles``
    -- the one definition the search enforces and the auction module reads -- over the same
    served vectors, so a chip that says "4 of 5 bowlers" is counting what the search would
    have counted.
    """
    if constraints is None:
        return None
    vectors = store.side_vectors(format_code, keys)
    bowlers = R.count_bowling_options(vectors["exp_balls_bowled"], format_code)
    has_keeper = R.any_keeper(vectors["keeper"])
    held = set(keys)
    missing = [key for key in constraints.must_include if key not in held]
    met = (
        len(keys) == constraints.team_size
        and bowlers >= constraints.min_bowlers
        and (has_keeper or not constraints.require_keeper)
        and not missing
    )
    return XiConstraintCheck(
        team_size=len(keys),
        bowlers=bowlers,
        min_bowlers=constraints.min_bowlers,
        has_keeper=has_keeper,
        require_keeper=constraints.require_keeper,
        missing_must_include=missing,
        met=met,
    )


def player_roles(req: PlayerRolesRequest, registry: XiRegistry = REGISTRY) -> PlayerRolesResponse:
    """What the served vectors say about each id asked for: keeper, bowling option, or
    neither (P3-1).

    The two predicates are ``ml.xi.roles``', which is where the selection's own constraints
    and its ``selection_reasons`` read them from, so this answer and a "why this player"
    card cannot disagree about the same player on the same run. Nothing here evaluates the
    objective, scores an eleven or orders anything: it is a read of the rating state.

    An id the state has never seen is reported ``known=false`` with no roles, and is named
    again in ``unknown_player_ids``. It is not quietly given the vectors of a debutant and
    reported as a batter -- a role invented for a player the model has never read is the
    substitution §8.7 exists to forbid.

    It is a live request, so H-11 applies: past the freshness limit the whole read is
    refused as ``RATINGS_STALE`` rather than answered from ratings that have moved on.
    """
    store = registry.store_as_of(req.format, None)
    keys = _keys(req.player_ids)
    known = store.known_players(keys)
    known_keys = [key for key, seen in zip(keys, known) if seen]
    vectors = store.side_vectors(req.format, known_keys) if known_keys else {}
    position = {key: i for i, key in enumerate(known_keys)}

    players = [
        PlayerRoles(
            player_id=key,
            known=seen,
            roles=R.roles_of(vectors, position[key], req.format) if seen else [],
        )
        for key, seen in zip(keys, known)
    ]
    unknown = [key for key, seen in zip(keys, known) if not seen]
    if unknown:
        logger.warning("xi.player_roles.unknown_players", format=req.format, count=len(unknown), ids=unknown[:10])
    logger.info("xi.player_roles.done", format=req.format, asked=len(keys), unknown=len(unknown))
    return PlayerRolesResponse(
        format=req.format,
        players=players,
        unknown_player_ids=unknown,
        served_ratings=_served_ratings(store),
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
    return XiWinResponse(
        team1_win_probability=display,
        objective_probability=objective,
        team1_constraint_check=_constraint_check(store, req.format, t1, req.team1_constraints),
        team2_constraint_check=_constraint_check(store, req.format, t2, req.team2_constraints),
        served_ratings=_served_ratings(store),
    )


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
        players=players,
        innings_marginalised=req.team1_bats_first is None,
        unknown_player_ids=unknown,
        venue_context=_venue_context(rows),
        served_ratings=_served_ratings(store),
    )


def _venue_context(rows: pd.DataFrame) -> VenueContext:
    """What the served state knew about the ground, read off the rows the model consumed.

    Off the rows and not recomputed from the state: these are the two columns the
    prediction beside it was actually made from, so a caller comparing grounds is comparing
    what the model read. Recomputing them here would be a second definition of a number the
    answer is meant to be reporting."""
    venue_n = float(rows.venue_n.iloc[0])
    return VenueContext(
        venue_bf_rate=float(rows.venue_bf_rate.iloc[0]),
        venue_n=venue_n,
        # 0 is the prior, and the prior is where a ground the state has never seen lands --
        # and equally where a request that named no teams lands, since the neutral fallback
        # in ``rows.team_context_or_neutral`` is for the pair and takes the venue with it.
        neutral=venue_n == 0.0,
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
        # The draws themselves only where the caller asked (P3-2): they are the same numbers
        # `total` summarises, handed over so a caller pooling several simulations quantifies
        # the pool instead of averaging three summaries into a range nothing drew.
        team1=_simulated_side(summary["team1"], 1, _total_draws(draws.team1, req.return_total_draws)),
        team2=_simulated_side(summary["team2"], 2, _total_draws(draws.team2, req.return_total_draws)),
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
        # Read off the calibration that drew these samples, never off a status call: what
        # the record needs is whether *this* answer's simulator had a factor (B-12).
        shared_factor=model.simulation is not None and model.simulation.shared_factor is not None,
        unknown_player_ids=unknown,
        served_ratings=_served_ratings(store),
    )


def _total_draws(team, requested: bool) -> Optional[List[float]]:
    """One side's total in every draw, or nothing where the caller did not ask.

    Guarded rather than always sent: it is ``n_samples`` floats a side, and the only caller
    that needs them is one pooling grounds (P3-2). Nothing is rounded on the way out -- a
    pooled quantile computed from rounded draws is not the quantile of the draws."""
    if not requested:
        return None
    return [float(value) for value in team.total]


def _simulated_side(side: Dict, team_side: int, total_draws: Optional[List[float]] = None) -> SimulatedSide:
    return SimulatedSide(
        total=SimulatedTotal(**side["total"]),
        extras_scorecard=side["extras"]["scorecard"],
        extras_spread_share=side["extras"]["spread_share"],
        wickets_lost=PerformanceRange(**side["wickets_lost"]),
        total_draws=total_draws,
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


def metric_glossary() -> dict:
    """What every reported metric means (L-1), from ``ml.xi.glossary``.

    Served from the code rather than from a report on disk, so a surface can explain its
    numbers before any harness has run and cannot be left rendering the prose of a report
    two releases old. The harness embeds the same dict in its report.
    """
    return {"entries": glossary.as_dict()}


def loaded_formats(registry: XiRegistry = REGISTRY) -> Dict[str, List[str]]:
    return {"loaded_xi_formats": registry.status().formats}
