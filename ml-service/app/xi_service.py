"""Serving-side glue for the XI-responsive win model: a lazily loaded ``XiStore`` and the
functions the ``/xi/*`` routes call. Kept free of FastAPI so it is testable in-process."""

from __future__ import annotations

import json
import os
import threading
from typing import Dict, List, Optional

from app.errors import error_payload
from app.logging import get_struct_logger
from app.models.xi import (
    XiConstraints,
    XiOptimizeRequest,
    XiOptimizeResponse,
    XiStatusResponse,
    XiWinRequest,
    XiWinResponse,
)
from ml.xi.optimizer import Constraints, marginal_values, select_xi
from ml.xi.store import RATINGS_ARTIFACT, XiStore
from ml.xi.train import REPORT_NAME

logger = get_struct_logger()


class XiUnavailable(Exception):
    """Raised when no XI artifacts are loaded; routes map it to 503."""

    def __init__(self, message: str):
        super().__init__(message)
        self.payload = error_payload(
            code="XI_MODEL_UNAVAILABLE", message=message, hint="run `make train-xi` and POST /admin/reload"
        )


class XiRegistry:
    """Holds the loaded store. ``reload`` is idempotent and tolerant of missing artifacts."""

    def __init__(self) -> None:
        self._store: Optional[XiStore] = None
        self._report: Optional[dict] = None
        self._lock = threading.Lock()

    def reload(self, models_dir: str) -> dict:
        with self._lock:
            self._store, self._report = None, None
            if not os.path.exists(os.path.join(models_dir, RATINGS_ARTIFACT)):
                logger.info("xi.artifacts.absent", models_dir=models_dir)
                return self.status().model_dump()
            try:
                self._store = XiStore.load(models_dir)
            except Exception as e:  # a corrupt artifact must not take the service down
                logger.error("xi.artifacts.load_failed", models_dir=models_dir, error=str(e), exc_info=True)
                return self.status().model_dump()
            report_path = os.path.join(models_dir, REPORT_NAME)
            if os.path.exists(report_path):
                with open(report_path) as fh:
                    self._report = json.load(fh)
            logger.info(
                "xi.artifacts.loaded", formats=sorted(self._store.models), players=len(self._store.state.players)
            )
            return self.status().model_dump()

    def store(self, format_code: str) -> XiStore:
        s = self._store
        if s is None:
            raise XiUnavailable("XI win model artifacts are not loaded")
        if not s.has_format(format_code):
            raise XiUnavailable(f"no XI win model for format {format_code!r}; loaded: {sorted(s.models)}")
        return s

    def status(self) -> XiStatusResponse:
        s = self._store
        if s is None:
            return XiStatusResponse(loaded=False, formats=[], players=0, ratings_through=None, report=None)
        return XiStatusResponse(
            loaded=True,
            formats=sorted(s.models),
            players=len(s.state.players),
            ratings_through=s.state.last_date.isoformat() if s.state.last_date else None,
            report=self._report,
        )


REGISTRY = XiRegistry()


def _keys(ids: List[int]) -> List[str]:
    return [str(i) for i in ids]


def _constraints(c: XiConstraints) -> Constraints:
    return Constraints(
        team_size=c.team_size,
        min_bowlers=c.min_bowlers,
        require_keeper=c.require_keeper,
        must_include=_keys(c.must_include),
        must_exclude=_keys(c.must_exclude),
    )


def optimize(req: XiOptimizeRequest, registry: XiRegistry = REGISTRY) -> XiOptimizeResponse:
    store = registry.store(req.format)
    pool, opponent = _keys(req.pool_player_ids), _keys(req.opponent_player_ids)
    unknown = [pid for pid, known in zip(req.pool_player_ids, store.known_players(pool)) if not known]
    if unknown:
        logger.warning("xi.optimize.unknown_players", format=req.format, count=len(unknown), ids=unknown[:10])
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
        selected_player_ids=[int(k) for k in result.selected],
        win_probability=result.win_probability,
        evaluations=result.evaluations,
        improved_over_seed=result.improved_over_seed,
        unknown_player_ids=unknown,
        marginal_values={int(k): v for k, v in mv.items()},
    )


def predict_win(req: XiWinRequest, registry: XiRegistry = REGISTRY) -> XiWinResponse:
    store = registry.store(req.format)
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


def status(registry: XiRegistry = REGISTRY) -> XiStatusResponse:
    return registry.status()


def loaded_formats(registry: XiRegistry = REGISTRY) -> Dict[str, List[str]]:
    return {"loaded_xi_formats": registry.status().formats}
