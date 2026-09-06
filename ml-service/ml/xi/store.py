"""Artifacts and serving.

One artifact per training run, ``xi_win_<FORMAT>.joblib``, holds two fitted models and the
column lists; ``xi_perf_<FORMAT>.joblib`` holds the format's performance model (L2-B); one
shared ``xi_ratings.joblib`` holds the serving ``RatingState``. The store loads them and
answers three questions: P(team1 wins | XI_1, XI_2, context), the per-player vectors an
optimiser needs to rebuild that probability for a different XI, and what each player of
the two elevens is expected to do.
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass
from typing import Dict, List, Optional, Sequence

import joblib
import numpy as np

from ml.xi import contract as C
from ml.xi.performance import PerformanceModels
from ml.xi.ratings import RatingState, aggregate_side, xi_feature_vector
from ml.xi.rows import serving_match, team_context_or_neutral
from ml.xi.runs import RunArtifactsInvalid, read_manifest

logger = logging.getLogger(__name__)

RATINGS_ARTIFACT = "xi_ratings.joblib"

# The arrays the serving path reads, split by what their last axis is.
#
# They are named constants because they are the shape a run has to have: D-6 (§10.5) was
# an artifact written before P-2, missing the nine arrays P-2 and P-3 added, which
# ``_state_from_payload`` left at the constructor's initial width -- so the first request
# touching a player past slot 1024 raised IndexError while /xi/status said loaded: true.
#
# The split matters to the check, not just to the reader. A player array's last axis is
# the player slot, so its width has to cover every registered key; a context array is
# indexed by (innings, format) or (innings, format, over), so its width says nothing
# about players and checking it against them would refuse every healthy run.
PLAYER_ARRAY_NAMES = (
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
    "bat_pos_sum",
    "bat_pos_n",
    "xi_n",
    "bat_ph_rae",
    "bat_ph_balls",
    "bowl_ph_rse",
    "bowl_ph_balls",
    "seq_num",
    "seq_den",
)  # fmt: skip

CONTEXT_ARRAY_NAMES = (
    "ctx_balls",
    "ctx_runs",
    "ctx_wickets",
    "ctx_extras",
    "ctx_deliveries",
    "ctx_bowler_wickets",
    "ctx_dismissals",
    "ctx_full_innings_deliveries",
    "ctx_full_innings",
    "debut_bat",
    "debut_bowl",
)  # fmt: skip

STATE_ARRAY_NAMES = PLAYER_ARRAY_NAMES + CONTEXT_ARRAY_NAMES

# The keyed tables beside the arrays: team and venue state, the fixture-context sums
# (A-1) and the players' dates of birth (X-1b). Checked for presence the same way, for
# the same reason.
STATE_TABLE_NAMES = (
    "team_elo",
    "team_results",
    "head_to_head",
    "venue_bat_first",
    "team_venue_matches",
    "venue_scoring",
    "competition_scoring",
    "birth_dates",
)


def model_artifact_name(format_code: str) -> str:
    return f"xi_win_{format_code}.joblib"


def performance_artifact_name(format_code: str) -> str:
    return f"xi_perf_{format_code}.joblib"


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
        "gender_split_context": state.gender_split_context,
        "age_aware_cold_start": state.age_aware_cold_start,
        "arrays": {name: getattr(state, name) for name in STATE_ARRAY_NAMES},
        "team_elo": dict(state.team_elo),
        "team_results": dict(state.team_results),
        "head_to_head": dict(state.head_to_head),
        "venue_bat_first": dict(state.venue_bat_first),
        "team_venue_matches": dict(state.team_venue_matches),
        "venue_scoring": dict(state.venue_scoring),
        "competition_scoring": dict(state.competition_scoring),
        "birth_dates": dict(state.birth_dates),
        "matches_seen": state.matches_seen,
        "last_date": state.last_date,
    }


def state_shape(state: RatingState) -> Dict:
    """The shape a run's rating state is in, for its manifest.

    Recorded so a manifest can be read without loading the joblib, and so the refusal
    below can quote what the run claims as well as what the payload holds.
    """
    return {
        "players": len(state.players),
        "arrays": {name: list(getattr(state, name).shape) for name in STATE_ARRAY_NAMES},
    }


def _check_payload_shape(payload: Dict, run_id: str) -> None:
    """Refuse a rating payload this code cannot serve, naming the run and the shape (D-6).

    Two things can be wrong. An array this code reads may be absent, which is the
    pre-P-2 artifact: assigning only what the payload carries leaves the rest at the
    constructor's initial width and the failure surfaces as an IndexError deep inside a
    prediction. Or a *player* array may be present but narrower than the number of
    registered players, which is the same failure with the arrays half-written.
    """
    arrays = payload.get("arrays") or {}
    players = len(payload.get("keys") or [])
    missing_tables = [name for name in STATE_TABLE_NAMES if name not in payload]
    if missing_tables:
        raise RunArtifactsInvalid(
            f"run {run_id}: the rating artifact is missing {len(missing_tables)} table(s) this code reads "
            f"({', '.join(missing_tables)}); it was written by an older pass and cannot be served. Retrain."
        )
    missing = [name for name in STATE_ARRAY_NAMES if name not in arrays]
    if missing:
        raise RunArtifactsInvalid(
            f"run {run_id}: the rating artifact is missing {len(missing)} array(s) this code reads "
            f"({', '.join(missing)}); it was written by an older pass and cannot be served. "
            f"Retrain to produce a run with all {len(STATE_ARRAY_NAMES)} arrays."
        )
    narrow = [
        f"{name} has width {arrays[name].shape[-1]}, expected {players}"
        for name in PLAYER_ARRAY_NAMES
        if arrays[name].shape[-1] < players
    ]
    if narrow:
        raise RunArtifactsInvalid(
            f"run {run_id}: the rating artifact registers {players} players but "
            f"{len(narrow)} array(s) are narrower than that ({'; '.join(narrow[:3])}"
            f"{', ...' if len(narrow) > 3 else ''}); it cannot answer for every player it names"
        )


def _check_model_columns(models: "FormatModels", run_id: str) -> None:
    """Refuse a win artifact fitted on columns this code no longer serves (D-6).

    The serving path builds its row from the artifact's own ``display_cols``, so an older
    artifact is internally consistent and loads without complaint -- it just answers with
    a surface the code has stopped contracting for. That is exactly D-6's failure mode:
    ``/xi/status`` reporting loaded while the numbers come from a shape nobody checked.
    B-7 dropped ``t1_pelo_std`` / ``t2_pelo_std`` from the display columns to make every
    upgrade raise the displayed probability; an artifact still carrying them would serve
    the incoherent surface silently. Retrain rather than reload.
    """
    for attribute, expected in (("objective_cols", C.XI_FEATURE_COLS), ("display_cols", C.DISPLAY_FEATURE_COLS)):
        actual = list(getattr(models, attribute, []) or [])
        if actual == list(expected):
            continue
        extra = [column for column in actual if column not in set(expected)]
        missing = [column for column in expected if column not in set(actual)]
        raise RunArtifactsInvalid(
            f"run {run_id}: the {models.format_code} win artifact was fitted on {attribute} this code does not "
            f"serve ({len(actual)} columns, expected {len(expected)}"
            f"{'; extra: ' + ', '.join(extra) if extra else ''}"
            f"{'; missing: ' + ', '.join(missing) if missing else ''}"
            f"{'; same columns in a different order' if not extra and not missing else ''}"
            f"); it was written by an older training pass. Retrain."
        )


def _state_from_payload(payload: Dict, run_id: str = "unnamed") -> RatingState:
    _check_payload_shape(payload, run_id)
    state = RatingState(
        gender_split_context=bool(payload.get("gender_split_context", False)),
        age_aware_cold_start=bool(payload.get("age_aware_cold_start", False)),
    )
    for k in payload["keys"]:
        state.players.slot(k)
    for name, arr in payload["arrays"].items():
        setattr(state, name, arr)
    for name in STATE_TABLE_NAMES:
        getattr(state, name).update(payload[name])
    state.matches_seen = payload["matches_seen"]
    state.last_date = payload["last_date"]
    return state


def save_ratings(state: RatingState, artifacts_dir: str) -> str:
    path = os.path.join(artifacts_dir, RATINGS_ARTIFACT)
    joblib.dump(_state_to_payload(state), path, compress=3)
    logger.info("saved rating state (%d players, through %s) to %s", len(state.players), state.last_date, path)
    return path


def load_ratings(artifacts_dir: str, run_id: str = "unnamed") -> RatingState:
    return _state_from_payload(joblib.load(os.path.join(artifacts_dir, RATINGS_ARTIFACT)), run_id)


def save_models(models: FormatModels, artifacts_dir: str) -> str:
    path = os.path.join(artifacts_dir, model_artifact_name(models.format_code))
    joblib.dump(models, path, compress=3)
    return path


def save_performance(models: PerformanceModels, format_code: str, artifacts_dir: str) -> str:
    """Write a format's performance artifact. ``format_code`` is the file's, which under
    E6's joint option can differ from the model's own (one fit serves both)."""
    path = os.path.join(artifacts_dir, performance_artifact_name(format_code))
    joblib.dump(models, path, compress=3)
    return path


class XiStore:
    """Serving-side access to ratings + per-format models."""

    def __init__(
        self,
        state: RatingState,
        models: Dict[str, FormatModels],
        performance: Optional[Dict[str, PerformanceModels]] = None,
    ):
        self.state = state
        self.models = models
        self.performance = performance or {}
        # The run these models came from, set by ``load``. None for a store assembled in
        # a test or by ``with_state``, which serves the same models over another state.
        self.manifest = None

    @classmethod
    def load(cls, run_directory: str) -> "XiStore":
        """Load one run's artifacts, or refuse with an error naming the run (H-16, D-6).

        The manifest is read first and on purpose: a directory of joblib files nothing
        can attribute to a run is not a run, and "it loaded" was never the question.
        """
        manifest = read_manifest(run_directory)
        artifacts_dir = run_directory
        state = load_ratings(artifacts_dir, manifest.run_id)
        models: Dict[str, FormatModels] = {}
        performance: Dict[str, PerformanceModels] = {}
        for fmt in C.FORMAT_CODES:
            path = os.path.join(artifacts_dir, model_artifact_name(fmt))
            if os.path.exists(path):
                models[fmt] = joblib.load(path)
                _check_model_columns(models[fmt], manifest.run_id)
            performance_path = os.path.join(artifacts_dir, performance_artifact_name(fmt))
            if os.path.exists(performance_path):
                performance[fmt] = joblib.load(performance_path)
        if not models:
            raise RunArtifactsInvalid(
                f"run {manifest.run_id}: no xi_win_<FORMAT>.joblib artifacts in {artifacts_dir}, "
                f"though its manifest names formats {manifest.formats}"
            )
        logger.info(
            "xi store loaded: run %s, win formats %s, performance formats %s, %d players",
            manifest.run_id,
            sorted(models),
            sorted(performance),
            len(state.players),
        )
        store = cls(state, models, performance)
        store.manifest = manifest
        return store

    def has_format(self, format_code: str) -> bool:
        return format_code in self.models

    def has_performance(self, format_code: str) -> bool:
        return format_code in self.performance

    def with_state(self, state: RatingState) -> "XiStore":
        """The same models over a different rating state -- how a backtest serves
        "ratings as of date D" (see ``ml.xi.asof``) instead of "through today"."""
        store = XiStore(state, self.models, self.performance)
        store.manifest = self.manifest
        return store

    def covers_as_of(self, as_of) -> bool:
        """Whether the loaded through-today state already is the as-of state for ``as_of``:
        true when every match it holds is strictly before that date."""
        return self.state.last_date is None or as_of > self.state.last_date

    def known_players(self, keys: Sequence[str]) -> List[bool]:
        return [k in self.state.players.key_to_slot for k in keys]

    def side_vectors(self, format_code: str, keys: Sequence[str]) -> Dict[str, np.ndarray]:
        """The per-player vectors as of the state's own date -- the ratings-through date
        for a live request, the as-of date's eve for a backtest -- which is also the date
        ``serving_match`` stamps on a fixture the serving path builds rows for."""
        return self.state.side_vectors(format_code, keys, on=self.state.last_date)

    def objective_probability(
        self, format_code: str, side1: Dict[str, np.ndarray], side2: Dict[str, np.ndarray]
    ) -> float:
        """P(side1 wins) from the objective model, marginalised over who bats first.

        The training label puts the side batting first as team1. At selection time the toss
        is unknown, so the prediction is the average of side1-bats-first and side2-bats-first
        (as 1 - P). Measured on the holdout this is at least as good as either orientation
        and it makes P(A, B) == 1 - P(B, A), which an argmax over XIs should be able to rely on.
        """
        m = self.models[format_code]
        a, b = aggregate_side(side1, format_code), aggregate_side(side2, format_code)
        x = np.vstack([xi_feature_vector(a, b, m.objective_cols), xi_feature_vector(b, a, m.objective_cols)])
        p = m.objective_proba(x)
        return float(0.5 * (p[0] + (1.0 - p[1])))

    def display_probability(
        self,
        format_code: str,
        team1_keys: Sequence[str],
        team2_keys: Sequence[str],
        team1_name: Optional[str] = None,
        team2_name: Optional[str] = None,
        venue: Optional[str] = None,
        team1_bats_first: Optional[bool] = None,
    ) -> float:
        """P(team1 wins) from the display model.

        Team names / venue are optional: without them the team-level columns fall back to
        their neutral values. ``team1_bats_first`` is optional too: unknown (None) averages
        both batting orders, which is the right treatment before the toss and measured
        +0.007 to +0.02 AUC over assuming an order; once the toss is known, pass it.
        """
        m = self.models[format_code]
        side1 = aggregate_side(self.side_vectors(format_code, team1_keys), format_code)
        side2 = aggregate_side(self.side_vectors(format_code, team2_keys), format_code)

        def row_for(first, second, first_name, second_name):
            row = dict(zip(m.objective_cols, xi_feature_vector(first, second, m.objective_cols)))
            row.update(self._team_context(format_code, first_name, second_name, venue))
            return np.asarray([row.get(c, 0.0) for c in m.display_cols], dtype=float)

        if team1_bats_first is True:
            return float(m.display_proba(row_for(side1, side2, team1_name, team2_name))[0])
        if team1_bats_first is False:
            return float(1.0 - m.display_proba(row_for(side2, side1, team2_name, team1_name))[0])
        p = m.display_proba(
            np.vstack([row_for(side1, side2, team1_name, team2_name), row_for(side2, side1, team2_name, team1_name)])
        )
        return float(0.5 * (p[0] + (1.0 - p[1])))

    def _team_context(self, fmt: str, t1: Optional[str], t2: Optional[str], venue: Optional[str]) -> Dict[str, float]:
        return team_context_or_neutral(self.state, serving_match(fmt, [], [], t1, t2, venue, self.state.last_date))
