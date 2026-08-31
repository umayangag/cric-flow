"""Artifacts and serving.

One artifact per training run, ``xi_win_<FORMAT>.joblib``, holds two fitted models and the
column lists; one shared ``xi_ratings.joblib`` holds the serving ``RatingState``. The store
loads them and answers two questions: P(team1 wins | XI_1, XI_2, context) and the per-player
vectors an optimiser needs to rebuild that probability for a different XI.
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass
from typing import Dict, List, Optional, Sequence

import joblib
import numpy as np

from ml.xi import contract as C
from ml.xi.ratings import RatingState, aggregate_side, xi_feature_vector

logger = logging.getLogger(__name__)

RATINGS_ARTIFACT = "xi_ratings.joblib"


def model_artifact_name(format_code: str) -> str:
    return f"xi_win_{format_code}.joblib"


@dataclass
class FormatModels:
    format_code: str
    objective: object  # monotone-by-construction model on XI_FEATURE_COLS -- what the optimiser maximises
    display: object  # richer model on DISPLAY_FEATURE_COLS -- the probability shown to users
    objective_cols: List[str]
    display_cols: List[str]
    metadata: Dict

    def objective_proba(self, xi_rows: np.ndarray) -> np.ndarray:
        return self.objective.predict_proba(np.atleast_2d(xi_rows))[:, 1]

    def display_proba(self, rows: np.ndarray) -> np.ndarray:
        return self.display.predict_proba(np.atleast_2d(rows))[:, 1]


def _state_to_payload(state: RatingState) -> Dict:
    return {
        "keys": list(state.players.keys),
        "arrays": {
            name: getattr(state, name)
            for name in (
                "bat_rae",
                "bat_balls",
                "bat_wae",
                "bat_matches",
                "bowl_rse",
                "bowl_balls",
                "bowl_wae",
                "bowl_matches",
                "career",
                "career_all",
                "keeper",
                "pelo",
                "ctx_balls",
                "ctx_runs",
                "ctx_wickets",
            )  # fmt: skip
        },
        "team_elo": dict(state.team_elo),
        "team_results": dict(state.team_results),
        "head_to_head": dict(state.head_to_head),
        "venue_bat_first": dict(state.venue_bat_first),
        "team_venue_matches": dict(state.team_venue_matches),
        "matches_seen": state.matches_seen,
        "last_date": state.last_date,
    }


def _state_from_payload(payload: Dict) -> RatingState:
    state = RatingState()
    for k in payload["keys"]:
        state.players.slot(k)
    for name, arr in payload["arrays"].items():
        setattr(state, name, arr)
    state.team_elo.update(payload["team_elo"])
    state.team_results.update(payload["team_results"])
    state.head_to_head.update(payload["head_to_head"])
    state.venue_bat_first.update(payload["venue_bat_first"])
    state.team_venue_matches.update(payload["team_venue_matches"])
    state.matches_seen = payload["matches_seen"]
    state.last_date = payload["last_date"]
    return state


def save_ratings(state: RatingState, artifacts_dir: str) -> str:
    path = os.path.join(artifacts_dir, RATINGS_ARTIFACT)
    joblib.dump(_state_to_payload(state), path, compress=3)
    logger.info("saved rating state (%d players, through %s) to %s", len(state.players), state.last_date, path)
    return path


def load_ratings(artifacts_dir: str) -> RatingState:
    return _state_from_payload(joblib.load(os.path.join(artifacts_dir, RATINGS_ARTIFACT)))


def save_models(models: FormatModels, artifacts_dir: str) -> str:
    path = os.path.join(artifacts_dir, model_artifact_name(models.format_code))
    joblib.dump(models, path, compress=3)
    return path


class XiStore:
    """Serving-side access to ratings + per-format models."""

    def __init__(self, state: RatingState, models: Dict[str, FormatModels]):
        self.state = state
        self.models = models

    @classmethod
    def load(cls, artifacts_dir: str) -> "XiStore":
        state = load_ratings(artifacts_dir)
        models: Dict[str, FormatModels] = {}
        for fmt in C.FORMAT_CODES:
            path = os.path.join(artifacts_dir, model_artifact_name(fmt))
            if os.path.exists(path):
                models[fmt] = joblib.load(path)
        if not models:
            raise FileNotFoundError(f"no xi_win_<FORMAT>.joblib artifacts in {artifacts_dir}")
        logger.info("xi store loaded: formats %s, %d players", sorted(models), len(state.players))
        return cls(state, models)

    def has_format(self, format_code: str) -> bool:
        return format_code in self.models

    def known_players(self, keys: Sequence[str]) -> List[bool]:
        return [k in self.state.players.key_to_slot for k in keys]

    def side_vectors(self, format_code: str, keys: Sequence[str]) -> Dict[str, np.ndarray]:
        return self.state.side_vectors(format_code, keys)

    def objective_probability(
        self, format_code: str, side1: Dict[str, np.ndarray], side2: Dict[str, np.ndarray]
    ) -> float:
        m = self.models[format_code]
        x = xi_feature_vector(aggregate_side(side1, format_code), aggregate_side(side2, format_code), m.objective_cols)
        return float(m.objective_proba(x)[0])

    def display_probability(
        self,
        format_code: str,
        team1_keys: Sequence[str],
        team2_keys: Sequence[str],
        team1_name: Optional[str] = None,
        team2_name: Optional[str] = None,
        venue: Optional[str] = None,
    ) -> float:
        """P(team1 wins) from the display model. Team names / venue are optional: without
        them the team-level columns fall back to their neutral values."""
        m = self.models[format_code]
        side1 = aggregate_side(self.side_vectors(format_code, team1_keys), format_code)
        side2 = aggregate_side(self.side_vectors(format_code, team2_keys), format_code)
        row = dict(zip(m.objective_cols, xi_feature_vector(side1, side2, m.objective_cols)))
        row.update(self._team_context(format_code, team1_name, team2_name, venue))
        x = np.asarray([row.get(c, 0.0) for c in m.display_cols], dtype=float)
        return float(m.display_proba(x)[0])

    def _team_context(self, fmt: str, t1: Optional[str], t2: Optional[str], venue: Optional[str]) -> Dict[str, float]:
        from ml.xi.sources import Deliveries, MatchRecord

        if t1 is None or t2 is None:
            return {
                "team_elo_diff": 0.0,
                "team_form_diff": 0.0,
                "team_h2h": 0.5,
                "team_h2h_n": 0.0,
                "venue_bf_rate": 0.5,
                "venue_n": 0.0,
                "venue_fam_diff": 0.0,
            }
        stub = MatchRecord(
            "", self.state.last_date, fmt, t1, t2, venue or "", "", [], [], None, None, Deliveries.empty()
        )
        return self.state.team_context(stub)
