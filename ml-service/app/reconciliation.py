"""Hybrid reconciliation: rescale player predictions to match innings model totals.

When the innings model is loaded and match context (team assignment) is provided,
raw batting/bowling predictions are rescaled so that:
- sum(batsman runs) = innings_runs (from innings model)
- sum(bowler wickets) = innings_wickets (from innings model)

This ensures consistency: runs conceded by bowlers = runs scored by batsmen.
"""

from typing import Any, Dict, List, Mapping, Optional, Set, Tuple

import numpy as np

# Single source of truth: match train_innings (base context, derived, format one-hot).
# Keep ml.* imports at module scope (not inside helpers) so the dependency graph stays explicit.
from ml.match_level_derived_features import compute_match_level_derived_features_scalars
from ml.train_innings import LEGACY_INNINGS_FEATURE_COLS
from ml.win_features import _format_one_hot_from_code

from .models import BacktestPlayerPred


def _innings_feature_dict(
    inning_number: int,
    bat_consistency_sum: float,
    bowl_consistency_sum: float,
    bat_form_sum: float,
    bowl_form_sum: float,
    venue_id: float,
    season_id: float,
    opposition_id: float,
    match_date_unix: float,
    temp: int,
    wind: int,
    rain: int,
    humidity: int,
    cloud: int,
    pressure: int,
    viscosity: int,
    format_code: Optional[str],
    derived_weights: Optional[Mapping[str, float]],
) -> Dict[str, float]:
    """Compute every potential innings feature as a name -> scalar mapping.

    This lets downstream code build the actual model input by selecting and
    ordering only the columns the trained model expects (via its sidecar).
    """
    one_hot = _format_one_hot_from_code(format_code)
    form_differential, consistency_differential, weather_composite = compute_match_level_derived_features_scalars(
        bat_form_sum,
        bowl_form_sum,
        bat_consistency_sum,
        bowl_consistency_sum,
        rain,
        humidity,
        cloud,
        weights=derived_weights,
    )
    values: Dict[str, float] = {
        "season_id": float(season_id),
        "season": float(season_id),
        "venue_id": float(venue_id),
        "inning_number": float(inning_number),
        "opposition_id": float(opposition_id),
        "match_date_unix": float(match_date_unix),
        "temp": float(temp),
        "wind": float(wind),
        "rain": float(rain),
        "humidity": float(humidity),
        "cloud": float(cloud),
        "pressure": float(pressure),
        "viscosity": float(viscosity),
        "bat_consistency_sum": float(bat_consistency_sum),
        "bowl_consistency_sum": float(bowl_consistency_sum),
        "bat_form_sum": float(bat_form_sum),
        "bowl_form_sum": float(bowl_form_sum),
        "form_differential": float(form_differential),
        "consistency_differential": float(consistency_differential),
        "weather_composite": float(weather_composite),
    }
    for col, val in one_hot.items():
        values[col] = float(val)
    return values


def _resolve_innings_feature_order(meta: Optional[Mapping[str, Any]]) -> List[str]:
    """Pick the feature column order: sidecar when available, else legacy pre-sidecar order."""
    if meta is not None:
        names = meta.get("feature_names")
        if isinstance(names, list) and names:
            return [str(n) for n in names]
    return list(LEGACY_INNINGS_FEATURE_COLS)


def build_innings_feature_vector(
    inning_number: int,
    bat_consistency_sum: float,
    bowl_consistency_sum: float,
    bat_form_sum: float,
    bowl_form_sum: float,
    venue_id: float = 0,
    season_id: float = 0,
    opposition_id: float = 0,
    match_date_unix: float = 0,
    temp: int = 0,
    wind: int = 0,
    rain: int = 0,
    humidity: int = 0,
    cloud: int = 0,
    pressure: int = 0,
    viscosity: int = 0,
    format_code: Optional[str] = None,
    meta: Optional[Mapping[str, Any]] = None,
) -> np.ndarray:
    """Build the innings-model feature vector.

    When *meta* (artifact sidecar) is provided, its ``feature_names`` drives
    column order/selection and ``derived_weights`` are used when computing the
    weather composite. Without sidecar metadata, uses :data:`LEGACY_INNINGS_FEATURE_COLS`
    so older artifacts (trained before derived features) keep the expected layout.
    """
    derived_weights: Optional[Mapping[str, float]] = None
    if meta is not None:
        dw = meta.get("derived_weights")
        if isinstance(dw, dict):
            derived_weights = dw
    feature_dict = _innings_feature_dict(
        inning_number=inning_number,
        bat_consistency_sum=bat_consistency_sum,
        bowl_consistency_sum=bowl_consistency_sum,
        bat_form_sum=bat_form_sum,
        bowl_form_sum=bowl_form_sum,
        venue_id=venue_id,
        season_id=season_id,
        opposition_id=opposition_id,
        match_date_unix=match_date_unix,
        temp=temp,
        wind=wind,
        rain=rain,
        humidity=humidity,
        cloud=cloud,
        pressure=pressure,
        viscosity=viscosity,
        format_code=format_code,
        derived_weights=derived_weights,
    )
    order = _resolve_innings_feature_order(meta)
    values = [feature_dict.get(col, 0.0) for col in order]
    return np.array(values, dtype=float).reshape(1, -1)


def predict_innings(
    scaler,
    model,
    inning_number: int,
    bat_consistency_sum: float,
    bowl_consistency_sum: float,
    bat_form_sum: float,
    bowl_form_sum: float,
    venue_id: float = 0,
    season_id: float = 0,
    opposition_id: float = 0,
    match_date_unix: float = 0,
    temp: int = 0,
    wind: int = 0,
    rain: int = 0,
    humidity: int = 0,
    cloud: int = 0,
    pressure: int = 0,
    viscosity: int = 0,
    format_code: Optional[str] = None,
    meta: Optional[Mapping[str, Any]] = None,
) -> Tuple[float, float]:
    """Predict innings_runs and innings_wickets for one innings.

    Pass *meta* (artifact sidecar) to pin feature order and derived-feature
    weights to those used at training time. See :func:`build_innings_feature_vector`.
    """
    X = build_innings_feature_vector(
        inning_number=inning_number,
        bat_consistency_sum=bat_consistency_sum,
        bowl_consistency_sum=bowl_consistency_sum,
        bat_form_sum=bat_form_sum,
        bowl_form_sum=bowl_form_sum,
        venue_id=venue_id,
        season_id=season_id,
        opposition_id=opposition_id,
        match_date_unix=match_date_unix,
        temp=temp,
        wind=wind,
        rain=rain,
        humidity=humidity,
        cloud=cloud,
        pressure=pressure,
        viscosity=viscosity,
        format_code=format_code,
        meta=meta,
    )
    if scaler is not None:
        X = scaler.transform(X)
    Y = model.predict(X)
    row = np.atleast_1d(Y[0]).ravel()
    runs = float(max(0.0, row[0])) if len(row) > 0 else 0.0
    wickets = float(max(0.0, min(10.0, row[1]))) if len(row) > 1 else 0.0  # cap at 10
    return runs, wickets


def rescale_player_predictions(
    preds: List[BacktestPlayerPred],
    team1_ids: Set[int],
    team2_ids: Set[int],
    innings1_runs: float,
    innings1_wickets: float,
    innings2_runs: float,
    innings2_wickets: float,
    default_economy: float = 6.0,
) -> List[BacktestPlayerPred]:
    """Rescale player predictions so totals match innings model outputs.

    team1 bats in innings 1, team2 bowls in innings 1.
    team2 bats in innings 2, team1 bowls in innings 2.

    Rescales runs for batsmen and wickets for bowlers proportionally.
    Economy is recomputed from rescaled runs_conceded and balls.
    """
    pid_to_pred = {p.player_id: p for p in preds}

    def _sum_runs(ids: Set[int]) -> float:
        return sum(pid_to_pred[pid].runs for pid in ids if pid in pid_to_pred)

    def _sum_wickets(ids: Set[int]) -> float:
        return sum(pid_to_pred[pid].wickets for pid in ids if pid in pid_to_pred)

    def _runs_conceded(p: BacktestPlayerPred) -> float:
        """Derive runs_conceded from economy and balls."""
        balls = p.balls if p.balls is not None and p.balls > 0 else 6.0
        return p.economy * (balls / 6.0)

    def _sum_runs_conceded(ids: Set[int]) -> float:
        return sum(_runs_conceded(pid_to_pred[pid]) for pid in ids if pid in pid_to_pred)

    runs_team1_raw = _sum_runs(team1_ids)
    runs_team2_raw = _sum_runs(team2_ids)
    wickets_team2_raw = _sum_wickets(team2_ids)  # team2 bowls in inn1
    wickets_team1_raw = _sum_wickets(team1_ids)  # team1 bowls in inn2

    scale_runs_1 = innings1_runs / runs_team1_raw if runs_team1_raw > 0 else 1.0
    scale_runs_2 = innings2_runs / runs_team2_raw if runs_team2_raw > 0 else 1.0
    scale_wkts_1 = innings1_wickets / wickets_team2_raw if wickets_team2_raw > 0 else 1.0
    scale_wkts_2 = innings2_wickets / wickets_team1_raw if wickets_team1_raw > 0 else 1.0

    runs_conc_team2_raw = _sum_runs_conceded(team2_ids)  # team2 bowls in inn1
    runs_conc_team1_raw = _sum_runs_conceded(team1_ids)  # team1 bowls in inn2
    scale_runs_conc_1 = innings1_runs / runs_conc_team2_raw if runs_conc_team2_raw > 0 else 1.0
    scale_runs_conc_2 = innings2_runs / runs_conc_team1_raw if runs_conc_team1_raw > 0 else 1.0

    out: List[BacktestPlayerPred] = []
    for p in preds:
        runs = p.runs
        wickets = p.wickets
        economy = p.economy
        balls = p.balls if p.balls is not None else 0.0

        if p.player_id in team1_ids:
            runs = runs * scale_runs_1
            wickets = wickets * scale_wkts_2  # team1 bowls in inn2
            r_conc_scale = scale_runs_conc_2
        else:
            runs = runs * scale_runs_2
            wickets = wickets * scale_wkts_1  # team2 bowls in inn1
            r_conc_scale = scale_runs_conc_1

        if balls and balls > 0:
            r_conc = _runs_conceded(p) * r_conc_scale
            economy = r_conc / (balls / 6.0)

        out.append(
            BacktestPlayerPred(
                player_id=p.player_id,
                runs=max(0.0, runs),
                balls=p.balls,
                fours=p.fours,
                sixes=p.sixes,
                wickets=max(0.0, wickets),
                economy=economy,
                catches=p.catches,
                run_outs=p.run_outs,
            )
        )
    return out
