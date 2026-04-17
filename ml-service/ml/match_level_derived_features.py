"""Derived features shared by match-level models (extras, innings) and reconciliation.

``form_differential``, ``consistency_differential``, and ``weather_composite`` use one
implementation so training, tuning data loaders, and inference stay aligned.

Weather-composite weights are configurable (``ml.match_level_derived``) but training
pins them into the model sidecar (see :mod:`ml.artifact_sidecar`) so inference uses
the training-time weights even if config is later retuned.
"""

from __future__ import annotations

from typing import Dict, Mapping, Optional

import numpy as np
import pandas as pd

from ml.config import get_match_level_derived_config

MATCH_LEVEL_DERIVED_FEATURE_COLS: tuple[str, ...] = (
    "form_differential",
    "consistency_differential",
    "weather_composite",
)

_WEIGHT_KEYS = (
    "weather_composite_rain_weight",
    "weather_composite_humidity_weight",
    "weather_composite_cloud_weight",
)


def resolve_weights(weights: Optional[Mapping[str, float]] = None) -> Dict[str, float]:
    """Return a concrete weight dict.

    When *weights* is provided (e.g. loaded from a model sidecar) we use those
    values verbatim and fill any missing key with the current config default.
    When *weights* is None we return a full copy of the current config.
    """
    cfg = get_match_level_derived_config()
    if weights is None:
        return {k: float(cfg[k]) for k in _WEIGHT_KEYS}
    out: Dict[str, float] = {}
    for k in _WEIGHT_KEYS:
        val = weights.get(k) if weights is not None else None
        out[k] = float(val) if val is not None else float(cfg[k])
    return out


def add_match_level_derived_features_to_df(
    df: pd.DataFrame,
    weights: Optional[Mapping[str, float]] = None,
) -> None:
    """Compute derived features in-place from base columns.

    - form_differential: bat_form_sum - bowl_form_sum
    - consistency_differential: bat_consistency_sum - bowl_consistency_sum
    - weather_composite: configurable weights on rain, humidity/100, cloud/100.
      Uses *weights* when given (e.g. pinned from a trained model sidecar);
      otherwise reads ``ml.match_level_derived`` from the service config.
    """

    def _col(name: str) -> pd.Series:
        if name in df.columns:
            return pd.to_numeric(df[name], errors="coerce").fillna(0.0)
        return pd.Series(np.zeros(len(df)), index=df.index)

    bat_form = _col("bat_form_sum")
    bowl_form = _col("bowl_form_sum")
    bat_cons = _col("bat_consistency_sum")
    bowl_cons = _col("bowl_consistency_sum")
    rain = _col("rain")
    humidity = _col("humidity")
    cloud = _col("cloud")
    w = resolve_weights(weights)
    wr = w["weather_composite_rain_weight"]
    wh = w["weather_composite_humidity_weight"]
    wc = w["weather_composite_cloud_weight"]

    df["form_differential"] = bat_form - bowl_form
    df["consistency_differential"] = bat_cons - bowl_cons
    df["weather_composite"] = wr * rain + wh * (humidity / 100.0) + wc * (cloud / 100.0)


def compute_match_level_derived_features_scalars(
    bat_form_sum: float,
    bowl_form_sum: float,
    bat_consistency_sum: float,
    bowl_consistency_sum: float,
    rain: float,
    humidity: float,
    cloud: float,
    weights: Optional[Mapping[str, float]] = None,
) -> tuple[float, float, float]:
    """Return (form_differential, consistency_differential, weather_composite) for one row.

    Uses the same formulas as :func:`add_match_level_derived_features_to_df`.
    Non-finite inputs are treated as 0.0 for parity with coerced DataFrame columns.
    Inference should pass *weights* loaded from the model sidecar.
    """

    def _f(x: float) -> float:
        v = float(x)
        return 0.0 if not np.isfinite(v) else v

    bf, bwf = _f(bat_form_sum), _f(bowl_form_sum)
    bc, bwc = _f(bat_consistency_sum), _f(bowl_consistency_sum)
    r, h, c = _f(rain), _f(humidity), _f(cloud)
    w = resolve_weights(weights)
    wr = w["weather_composite_rain_weight"]
    wh = w["weather_composite_humidity_weight"]
    wcloud = w["weather_composite_cloud_weight"]
    form_differential = bf - bwf
    consistency_differential = bc - bwc
    weather_composite = wr * r + wh * (h / 100.0) + wcloud * (c / 100.0)
    return form_differential, consistency_differential, weather_composite
