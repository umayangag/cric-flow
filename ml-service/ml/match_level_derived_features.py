"""Derived features shared by match-level models (extras, innings) and reconciliation.

``form_differential``, ``consistency_differential``, and ``weather_composite`` use one
implementation so training, tuning data loaders, and inference stay aligned.
"""

from __future__ import annotations

import numpy as np
import pandas as pd

MATCH_LEVEL_DERIVED_FEATURE_COLS: tuple[str, ...] = (
    "form_differential",
    "consistency_differential",
    "weather_composite",
)


def add_match_level_derived_features_to_df(df: pd.DataFrame) -> None:
    """Compute derived features in-place from base columns.

    - form_differential: bat_form_sum - bowl_form_sum
    - consistency_differential: bat_consistency_sum - bowl_consistency_sum
    - weather_composite: 0.5 * rain + 0.3 * humidity/100 + 0.2 * cloud/100
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

    df["form_differential"] = bat_form - bowl_form
    df["consistency_differential"] = bat_cons - bowl_cons
    df["weather_composite"] = 0.5 * rain + 0.3 * (humidity / 100.0) + 0.2 * (cloud / 100.0)


def compute_match_level_derived_features_scalars(
    bat_form_sum: float,
    bowl_form_sum: float,
    bat_consistency_sum: float,
    bowl_consistency_sum: float,
    rain: float,
    humidity: float,
    cloud: float,
) -> tuple[float, float, float]:
    """Return (form_differential, consistency_differential, weather_composite) for one row.

    Uses the same formulas as :func:`add_match_level_derived_features_to_df`.
    Non-finite inputs are treated as 0.0 for parity with coerced DataFrame columns.
    """

    def _f(x: float) -> float:
        v = float(x)
        return 0.0 if not np.isfinite(v) else v

    bf, bwf = _f(bat_form_sum), _f(bowl_form_sum)
    bc, bwc = _f(bat_consistency_sum), _f(bowl_consistency_sum)
    r, h, c = _f(rain), _f(humidity), _f(cloud)
    form_differential = bf - bwf
    consistency_differential = bc - bwc
    weather_composite = 0.5 * r + 0.3 * (h / 100.0) + 0.2 * (c / 100.0)
    return form_differential, consistency_differential, weather_composite
