import os
from dataclasses import dataclass
from datetime import datetime
from typing import Dict, Iterable, List, Optional, Protocol, Tuple

import numpy as np

try:
    from ml.config import get_feature_defaults
except Exception:
    get_feature_defaults = None

from .models import (
    BacktestMatchAgg,
    BacktestMetrics,
    BacktestPlayerPred,
    BattingFeatures,
    BowlingFeatures,
    HistoricalMatchBacktestRequest,
    HistoricalMatchBacktestResponse,
    MatchComparison,
    PlayerComparison,
    PlayerPoint,
)


def _float(d: Dict[str, float], key: str, default: float) -> float:
    v = d.get(key)
    if v is None:
        return default
    try:
        return float(v)
    except (TypeError, ValueError):
        return default


def _get_required_float(d: Dict[str, float], key: str) -> float:
    """Return float for a required feature; raise ValueError if missing or invalid."""
    v = d.get(key)
    if v is None:
        raise ValueError(f"Required feature '{key}' is missing from the feature map.")
    try:
        return float(v)
    except (TypeError, ValueError):
        raise ValueError(f"Feature '{key}' has a non-numeric value: {v}")


def _int(d: Dict[str, float], key: str, default: int) -> int:
    v = d.get(key)
    if v is None:
        return default
    try:
        return int(round(float(v)))
    except (TypeError, ValueError):
        return default


def _feature_defaults() -> Dict:
    """Feature defaults for missing keys (from config ml.feature_defaults or built-in)."""
    if get_feature_defaults is not None:
        try:
            return get_feature_defaults()
        except Exception:
            pass
    return {
        "common": {
            "temp": 25,
            "humidity": 50,
            "wind": 0,
            "rain": 0,
            "cloud": 0,
            "pressure": 0,
            "viscosity": 0,
            "inning": 1,
            "session": 1,
            "toss": 0,
        },
        "fielding": {"consistency": 0.5, "form": 0.0, "venue": 0.5, "opposition": 0.5},
    }


def build_batting_features_from_map(
    player_id: int,
    cutoff: datetime,
    fmt: Optional[str],
    feature_map: Dict[str, float],
) -> BattingFeatures:
    """Build BattingFeatures from go-app feature map; required features raise if missing."""
    d = {k: v for k, v in feature_map.items()}
    defs = _feature_defaults()
    c = defs.get("common", {})
    season = _int(d, "season", cutoff.year if cutoff else 0)
    return BattingFeatures(
        batting_consistency=max(0.0, _get_required_float(d, "batting_consistency")),
        batting_form=max(0.0, _get_required_float(d, "batting_form")),
        batting_temp=_int(d, "batting_temp", c.get("temp", 25)),
        batting_wind=_int(d, "batting_wind", c.get("wind", 0)),
        batting_rain=_int(d, "batting_rain", c.get("rain", 0)),
        batting_humidity=_int(d, "batting_humidity", c.get("humidity", 50)),
        batting_cloud=_int(d, "batting_cloud", c.get("cloud", 0)),
        batting_pressure=_int(d, "batting_pressure", c.get("pressure", 0)),
        batting_viscosity=min(1, max(0, _int(d, "batting_viscosity", c.get("viscosity", 0)))),
        batting_inning=min(2, max(1, _int(d, "batting_inning", c.get("inning", 1)))),
        batting_session=min(3, max(1, _int(d, "batting_session", c.get("session", 1)))),
        toss=min(1, max(0, _int(d, "toss", c.get("toss", 0)))),
        venue=_get_required_float(d, "venue"),
        opposition=_get_required_float(d, "opposition"),
        season=season,
        player_name="",
        format=fmt,
    )


@dataclass
class FieldingFeatures:
    """Feature vector for fielding model (catches, run_outs, stumpings prediction)."""

    fielding_consistency: float
    fielding_form: float
    fielding_temp: int
    fielding_wind: int
    fielding_rain: int
    fielding_humidity: int
    fielding_cloud: int
    fielding_pressure: int
    fielding_viscosity: int
    fielding_inning: int
    fielding_toss: int
    fielding_venue: float
    fielding_opposition: float
    fielding_season: int


def build_fielding_features_from_map(
    player_id: int,
    cutoff: datetime,
    fmt: Optional[str],
    feature_map: Dict[str, float],
) -> FieldingFeatures:
    """Build FieldingFeatures from go-app feature map. Uses config defaults for missing keys."""
    d = {k: v for k, v in feature_map.items()}
    defs = _feature_defaults()
    c = defs.get("common", {})
    f = defs.get("fielding", {})
    season = _int(d, "season", cutoff.year if cutoff else 0)
    return FieldingFeatures(
        fielding_consistency=max(0.0, _float(d, "fielding_consistency", f.get("consistency", 0.5))),
        fielding_form=max(0.0, _float(d, "fielding_form", f.get("form", 0.0))),
        fielding_temp=_int(d, "fielding_temp", _int(d, "batting_temp", c.get("temp", 25))),
        fielding_wind=_int(d, "fielding_wind", _int(d, "batting_wind", c.get("wind", 0))),
        fielding_rain=_int(d, "fielding_rain", _int(d, "batting_rain", c.get("rain", 0))),
        fielding_humidity=_int(d, "fielding_humidity", _int(d, "batting_humidity", c.get("humidity", 50))),
        fielding_cloud=_int(d, "fielding_cloud", _int(d, "batting_cloud", c.get("cloud", 0))),
        fielding_pressure=_int(d, "fielding_pressure", _int(d, "batting_pressure", c.get("pressure", 0))),
        fielding_viscosity=min(
            1, max(0, _int(d, "fielding_viscosity", _int(d, "batting_viscosity", c.get("viscosity", 0))))
        ),
        fielding_inning=min(2, max(1, _int(d, "fielding_inning", _int(d, "batting_inning", c.get("inning", 1))))),
        fielding_toss=min(1, max(0, _int(d, "toss", c.get("toss", 0)))),
        fielding_venue=_float(d, "fielding_venue", _float(d, "venue", f.get("venue", 0.5))),
        fielding_opposition=_float(d, "fielding_opposition", _float(d, "opposition", f.get("opposition", 0.5))),
        fielding_season=season,
    )


def build_bowling_features_from_map(
    player_id: int,
    cutoff: datetime,
    fmt: Optional[str],
    feature_map: Dict[str, float],
) -> BowlingFeatures:
    """Build BowlingFeatures from go-app feature map; required features raise if missing."""
    d = {k: v for k, v in feature_map.items()}
    defs = _feature_defaults()
    c = defs.get("common", {})
    season = _int(d, "season", cutoff.year if cutoff else 0)
    return BowlingFeatures(
        bowling_consistency=max(0.0, _get_required_float(d, "bowling_consistency")),
        bowling_form=max(0.0, _get_required_float(d, "bowling_form")),
        bowling_temp=_int(d, "bowling_temp", c.get("temp", 25)),
        bowling_wind=_int(d, "bowling_wind", c.get("wind", 0)),
        bowling_rain=_int(d, "bowling_rain", c.get("rain", 0)),
        bowling_humidity=_int(d, "bowling_humidity", c.get("humidity", 50)),
        bowling_cloud=_int(d, "bowling_cloud", c.get("cloud", 0)),
        bowling_pressure=_int(d, "bowling_pressure", c.get("pressure", 0)),
        bowling_viscosity=min(1, max(0, _int(d, "bowling_viscosity", c.get("viscosity", 0)))),
        batting_inning=min(2, max(1, _int(d, "batting_inning", c.get("inning", 1)))),
        bowling_session=min(3, max(1, _int(d, "bowling_session", c.get("session", 1)))),
        toss=min(1, max(0, _int(d, "toss", c.get("toss", 0)))),
        bowling_venue=_float(d, "bowling_venue", _get_required_float(d, "venue")),
        bowling_opposition=_float(d, "bowling_opposition", _get_required_float(d, "opposition")),
        season=season,
        player_name="",
        format=fmt,
    )


def resolve_model_version(app_version_fallback: str) -> str:
    """Return a model version string for responses.

    Preference order:
    1) ENV MODEL_VERSION
    2) Provided FastAPI app.version fallback
    """
    mv = os.environ.get("MODEL_VERSION", "").strip()
    if mv:
        return mv
    return app_version_fallback or "unknown"


def _deterministic_rng_seed(*parts: str) -> int:
    """Build a stable 32-bit seed from text parts.

    Simple non-cryptographic hash to keep baseline predictions deterministic.
    """
    acc = 0x345678
    for p in parts:
        for ch in str(p):
            acc = (acc * 1000003) ^ ord(ch)
        acc &= 0xFFFFFFFF
    return acc or 42


def predict_players_baseline(cutoff: datetime, player_ids: List[int]) -> List[BacktestPlayerPred]:
    """Deterministic simple baseline for player stats used in backtests.

    Pseudo-random but stable per (cutoff, player_id).
    """
    out: List[BacktestPlayerPred] = []
    for pid in player_ids:
        seed = _deterministic_rng_seed(cutoff.isoformat(), str(pid))
        rng = np.random.default_rng(seed)
        runs = float(max(0.0, np.round(rng.normal(20.0, 12.0))))
        wickets = float(max(0.0, np.round(rng.uniform(0.0, 3.0), 1)))
        economy = float(np.round(5.0 + rng.random() * 5.0, 1))
        catches = float(rng.integers(0, 4))
        run_outs = float(rng.integers(0, 3))
        out.append(
            BacktestPlayerPred(
                player_id=int(pid),
                runs=runs,
                wickets=wickets,
                economy=economy,
                catches=catches,
                run_outs=run_outs,
            )
        )
    return out


def predict_match_baseline(cutoff: datetime, teams: List[str]) -> BacktestMatchAgg:
    """Deterministic simple baseline aggregate for a match used in backtests."""
    a, b = teams[0].upper(), teams[1].upper()
    seed = _deterministic_rng_seed(cutoff.isoformat(), a, b)
    rng = np.random.default_rng(seed)
    # Aggregate team runs: sample a plausible total and enforce a sensible lower bound
    base_runs = int(np.round(rng.uniform(120, 190)))
    runs = float(base_runs)
    wickets = float(np.clip(np.round(rng.uniform(4, 8)), 2, 10))
    extras = float(int(np.clip(np.round(rng.uniform(5, 15)), 0, 25)))
    w_seed_a = _deterministic_rng_seed(a)
    w_seed_b = _deterministic_rng_seed(b)
    winner = a if (w_seed_a ^ seed) >= (w_seed_b ^ seed) else b
    return BacktestMatchAgg(runs=runs, wickets=wickets, extras=extras, winner_team_code=winner)


# -------------------- Historical backtest service (repository-abstracted) --------------------


class HistoricalDataRepo(Protocol):
    """Repository interface to access historical, already-played match data.

    Concrete implementation should fetch from the project database. Tests can provide fakes.
    """

    def resolve_match_id(self, *, match_id: Optional[int], filters: Optional[Dict[str, object]]) -> int:
        """Return the canonical match_id given either id or filters. Must raise on not found/ambiguous."""

    def get_playing_eleven(self, match_id: int) -> List[int]:
        """Return list of player_ids who actually played (both teams combined or pair of lists)."""

    def get_actual_player_points(self, match_id: int, player_ids: Iterable[int]) -> Dict[int, PlayerPoint]:
        """Return actual per-player points (runs, wickets, economy as available)."""

    def get_actual_match_agg(self, match_id: int) -> BacktestMatchAgg:
        """Return actual match aggregates (runs, wickets, extras, winner_team_code)."""

    def get_match_teams(self, match_id: int) -> Tuple[str, str]:
        """Return (team1_code, team2_code) for the match to enable aggregate prediction baseline."""


@dataclass
class HistoricalBacktestInputs:
    cutoff: datetime
    match_id: int
    playing_ids: List[int]
    actual_players: Dict[int, PlayerPoint]
    actual_match: BacktestMatchAgg
    teams: Tuple[str, str]


def _prepare_inputs(req: HistoricalMatchBacktestRequest, repo: HistoricalDataRepo) -> HistoricalBacktestInputs:
    cutoff = req.cutoff_date
    fid = None
    filters: Optional[Dict[str, object]] = None
    if req.filters is not None:
        f = req.filters
        filters = {
            "format": f.format,
            "team1": f.team1,
            "team2": f.team2,
            "match_date": f.match_date,
        }
    fid = repo.resolve_match_id(match_id=req.match_id, filters=filters)
    playing = repo.get_playing_eleven(fid)
    actual_players = repo.get_actual_player_points(fid, playing)
    actual_match = repo.get_actual_match_agg(fid)
    teams = repo.get_match_teams(fid)
    return HistoricalBacktestInputs(
        cutoff=cutoff,
        match_id=fid,
        playing_ids=list(playing),
        actual_players=actual_players,
        actual_match=actual_match,
        teams=teams,
    )


def _predict_players_for_ids(cutoff: datetime, player_ids: List[int]) -> Dict[int, PlayerPoint]:
    """Predict player points for given IDs using baseline for now.

    Later, this can be switched to true model predictions trained with data < cutoff.
    """
    preds = predict_players_baseline(cutoff, player_ids)
    out: Dict[int, PlayerPoint] = {}
    for p in preds:
        out[p.player_id] = PlayerPoint(runs=p.runs, wickets=p.wickets, economy=p.economy)
    return out


def _compute_player_comparisons(
    player_ids: List[int],
    predicted: Dict[int, PlayerPoint],
    actual: Dict[int, PlayerPoint],
) -> Tuple[List[PlayerComparison], float, float, Optional[float]]:
    comps: List[PlayerComparison] = []
    errors_runs: List[float] = []
    sq_errors_runs: List[float] = []
    errors_wkts: List[float] = []
    for pid in player_ids:
        pred = predicted.get(pid, PlayerPoint(runs=0.0))
        act = actual.get(pid, PlayerPoint(runs=0.0))
        er = abs((pred.runs or 0.0) - (act.runs or 0.0))
        errors_runs.append(er)
        sq_errors_runs.append(er * er)
        ew: Optional[float] = None
        if pred.wickets is not None and act.wickets is not None:
            ew = abs((pred.wickets or 0.0) - (act.wickets or 0.0))
            errors_wkts.append(ew)
        comps.append(
            PlayerComparison(
                player_id=pid,
                predicted=pred,
                actual=act,
                abs_error_runs=er,
                abs_error_wickets=ew,
            )
        )
    mae_runs = float(sum(errors_runs) / max(1, len(errors_runs)))
    rmse_runs = float((sum(sq_errors_runs) / max(1, len(sq_errors_runs))) ** 0.5)
    mae_wkts: Optional[float] = None
    if errors_wkts:
        mae_wkts = float(sum(errors_wkts) / len(errors_wkts))
    return comps, mae_runs, rmse_runs, mae_wkts


def historical_backtest(
    req: HistoricalMatchBacktestRequest, repo: HistoricalDataRepo, model_version: str
) -> HistoricalMatchBacktestResponse:
    """Run a historical match backtest end-to-end using the provided repository.

    Current implementation uses deterministic baselines for predictions. It enforces the
    cutoff by constructing seeds with the cutoff timestamp. Replace with true model logic later.
    """
    inputs = _prepare_inputs(req, repo)
    pred_players = _predict_players_for_ids(inputs.cutoff, inputs.playing_ids)
    comps, mae_runs, rmse_runs, mae_wkts = _compute_player_comparisons(
        inputs.playing_ids, pred_players, inputs.actual_players
    )
    # Aggregate match prediction baseline using teams
    match_pred = predict_match_baseline(inputs.cutoff, list(inputs.teams))
    winner_correct: Optional[bool] = None
    if inputs.actual_match.winner_team_code and match_pred.winner_team_code:
        winner_correct = inputs.actual_match.winner_team_code == match_pred.winner_team_code

    return HistoricalMatchBacktestResponse(
        players=comps,
        match=MatchComparison(predicted=match_pred, actual=inputs.actual_match),
        metrics=BacktestMetrics(
            mae_runs=mae_runs, rmse_runs=rmse_runs, mae_wickets=mae_wkts, winner_correct=winner_correct
        ),
        model_version=model_version or "unknown",
    )


# -------------------- Deterministic in-memory repo (temporary scaffolding) --------------------


class DeterministicInMemoryRepo:
    """A temporary repo implementation generating deterministic data for development and tests.

    Replace with a real DB-backed implementation that fetches precomputed features and actuals.
    """

    def __init__(self) -> None:
        self._teams_by_match: Dict[int, Tuple[str, str]] = {}

    def resolve_match_id(self, *, match_id: Optional[int], filters: Optional[Dict[str, object]]) -> int:
        if match_id is not None:
            # If teams known in mapping, keep; else synthesize later from id
            return int(match_id)
        if filters is None:
            raise ValueError("Must provide either match_id or filters")
        fmt = str(filters.get("format", "UNK")).upper()
        t1 = str(filters.get("team1", "T1")).upper()
        t2 = str(filters.get("team2", "T2")).upper()
        dt = str(filters.get("match_date", "1970-01-01"))
        seed = _deterministic_rng_seed("mid", fmt, t1, t2, dt)
        mid = int(seed % 10_000_000)
        # Record teams for this synthesized id
        self._teams_by_match[mid] = (t1, t2)
        return mid

    def get_playing_eleven(self, match_id: int) -> List[int]:
        # 22 players total (11 per side), using deterministic range per match
        base = int(_deterministic_rng_seed("ply", str(match_id)) % 100_000)
        return [base + i for i in range(1, 23)]

    def get_actual_player_points(self, match_id: int, player_ids: Iterable[int]) -> Dict[int, PlayerPoint]:
        out: Dict[int, PlayerPoint] = {}
        for pid in player_ids:
            seed = _deterministic_rng_seed("actp", str(match_id), str(pid))
            rng = np.random.default_rng(seed)
            runs = float(max(0.0, np.round(rng.normal(22.0, 15.0))))
            wickets = float(max(0.0, np.round(rng.uniform(0.0, 3.0), 1)))
            economy = float(np.round(5.0 + rng.random() * 6.0, 1))
            out[int(pid)] = PlayerPoint(runs=runs, wickets=wickets, economy=economy)
        return out

    def get_match_teams(self, match_id: int) -> Tuple[str, str]:
        if match_id in self._teams_by_match:
            return self._teams_by_match[match_id]
        # Fallback to synthetic teams derived from id
        a = f"T{(match_id % 26) + 1}A"
        b = f"T{((match_id // 3) % 26) + 1}B"
        self._teams_by_match[match_id] = (a, b)
        return a, b

    def get_actual_match_agg(self, match_id: int) -> BacktestMatchAgg:
        a, b = self.get_match_teams(match_id)
        seed = _deterministic_rng_seed("actm", str(match_id), a, b)
        rng = np.random.default_rng(seed)
        runs = float(int(np.round(rng.uniform(110, 200))))
        wickets = float(int(np.clip(np.round(rng.uniform(3, 9)), 2, 10)))
        extras = float(int(np.clip(np.round(rng.uniform(5, 18)), 0, 28)))
        w_seed_a = _deterministic_rng_seed("a", a)
        w_seed_b = _deterministic_rng_seed("b", b)
        winner = a if (w_seed_a ^ seed) >= (w_seed_b ^ seed) else b
        return BacktestMatchAgg(runs=runs, wickets=wickets, extras=extras, winner_team_code=winner)
