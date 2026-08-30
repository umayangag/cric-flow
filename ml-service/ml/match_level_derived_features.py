"""Derived features shared by match-level models (extras, innings) and reconciliation.

``form_differential`` and ``consistency_differential`` use one implementation so
training, tuning data loaders, and inference stay aligned.

There was a third, ``weather_composite``, a configurable blend of rain, humidity and
cloud. C2-2b removed those three inputs from every export because nothing has ever
populated ``weather_data`` (see ``docs/weather-not-implemented.md``), which left the
composite computing a constant zero from columns that were no longer there. The
low-variance filter then discarded it on every format and every fit, so no trained
artifact ever named it. It is gone, and so is the weight configuration that only it
used.
"""

from __future__ import annotations

import logging

import numpy as np
import pandas as pd

logger = logging.getLogger(__name__)

MATCH_LEVEL_DERIVED_FEATURE_COLS: tuple[str, ...] = (
    "form_differential",
    "consistency_differential",
)


def add_match_level_derived_features_to_df(df: pd.DataFrame) -> None:
    """Compute derived features in-place from base columns.

    - form_differential: bat_form_sum - bowl_form_sum
    - consistency_differential: bat_consistency_sum - bowl_consistency_sum
    """

    def _col(name: str) -> pd.Series:
        if name in df.columns:
            return pd.to_numeric(df[name], errors="coerce").fillna(0.0)
        return pd.Series(np.zeros(len(df)), index=df.index)

    bat_form = _col("bat_form_sum")
    bowl_form = _col("bowl_form_sum")
    bat_cons = _col("bat_consistency_sum")
    bowl_cons = _col("bowl_consistency_sum")

    df["form_differential"] = bat_form - bowl_form
    df["consistency_differential"] = bat_cons - bowl_cons


def compute_match_level_derived_features_scalars(
    bat_form_sum: float,
    bowl_form_sum: float,
    bat_consistency_sum: float,
    bowl_consistency_sum: float,
) -> tuple[float, float]:
    """Return (form_differential, consistency_differential) for one row.

    Uses the same formulas as :func:`add_match_level_derived_features_to_df`.
    Non-finite inputs are treated as 0.0 for parity with coerced DataFrame columns.
    """

    def _f(x: float) -> float:
        v = float(x)
        return 0.0 if not np.isfinite(v) else v

    bf, bwf = _f(bat_form_sum), _f(bowl_form_sum)
    bc, bwc = _f(bat_consistency_sum), _f(bowl_consistency_sum)
    return bf - bwf, bc - bwc
