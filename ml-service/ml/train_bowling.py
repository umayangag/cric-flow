"""
Train bowling model (runs_conceded, deliveries, wickets_taken).

Thin wrapper around TrainingPipeline — defines bowling-specific ModelSpec.
All shared logic lives in training_pipeline.py.

Usage:
  python -m ml.train_bowling                          # CSV from GO_APP_OUTPUT_DIR
  python -m ml.train_bowling --from-api --cutoff ...  # fetch from go-app API
  python -m ml.train_bowling --all-formats            # all formats from config
"""

from __future__ import annotations

import logging
from typing import Dict

import numpy as np
import pandas as pd

from .training_pipeline import ModelSpec, TrainingPipeline

logger = logging.getLogger(__name__)

# Raw windowed stats (v2) from configs/feature_vectors.json (single source of truth).
try:
    from app.feature_config import get_raw_stat_feature_names

    BOWL_RAW_STAT_COLS = get_raw_stat_feature_names("bowling")
except (ImportError, RuntimeError):  # noqa: S110 (allow broad except for optional app dependency at import)
    # Fallback when app not available (e.g. some test envs); must match Go contract.
    BOWL_RAW_STAT_COLS = [
        "bowling_mean_w3",
        "bowling_mean_w5",
        "bowling_mean_w10",
        "bowling_mean_w20",
        "bowling_std_w5",
        "bowling_std_w10",
        "bowling_max_w10",
        "bowling_min_w10",
        "bowling_median_w10",
        "bowling_last_1",
        "bowling_last_2",
        "bowling_last_3",
        "bowling_career_mean",
        "bowling_career_count",
        "bowling_pct_zero_w10",
        "bowling_trend_w5",
        "bowling_days_since_last",
        "bowling_innings_in_last_90d",
    ]

# ── Column definitions ───────────────────────────────────────────────────

BOWL_SEQ_COLS = [
    "bowl_prev_wkt_rate",
    "bowl_window_econ_24_death",
    "bowl_window_wkt_rate_24_death",
    "bowl_extras_wide_rate_pp",
    "bowl_react_after_boundary_wkt_rate_next",
    "bowl_spell_first_over_wkt_rate",
    "bowl_over_ball1_wkt_rate",
    "bowl_over_ball6_wkt_rate",
]

FEATURE_COLS = (
    BOWL_RAW_STAT_COLS
    + [
        "inning",
        "bowling_session",
        "toss",
        "bowling_venue",
        "bowling_opposition",
        "match_month_sin",
        "match_month_cos",
        "match_day_of_week_sin",
        "match_day_of_week_cos",
    ]
    + BOWL_SEQ_COLS
)

TARGET_COLS = [
    "runs",
    "balls",
    "wickets",
]

TARGET_COLS_SHARE = ["runs_share", "balls", "wickets_share"]


# ── DataFrame preparation hook ───────────────────────────────────────────


def _bowling_col_map() -> Dict[str, str]:
    m = {
        "temp": "temp",
        "wind": "wind",
        "rain": "rain",
        "humidity": "humidity",
        "cloud": "cloud",
        "pressure": "pressure",
        "viscosity": "viscosity",
        "inning": "inning",
        "bowling_session": "bowling_session",
        "toss": "toss",
        "bowling_venue": "bowling_venue",
        "bowling_opposition": "bowling_opposition",
    }
    m.update({c: c for c in BOWL_RAW_STAT_COLS})
    m.update(
        {
            "runs": "runs",
            "balls": "balls",
            "wickets": "wickets",
            "econ": "econ",
            "innings_runs": "innings_runs",
            "innings_wickets": "innings_wickets",
        }
    )
    for c in BOWL_SEQ_COLS:
        m[c] = c
    return m


def _prepare_bowling_df(df: pd.DataFrame, spec: ModelSpec) -> pd.DataFrame:
    """Normalize column names and fill missing columns for bowling data."""
    df = df.rename(columns=_bowling_col_map())
    # Normalize toss: CSV/API may have "bat"/"field" strings; model expects 0/1
    if "toss" in df.columns and df["toss"].dtype == object:
        df["toss"] = df["toss"].astype(str).str.strip().str.lower().map(lambda x: 1.0 if x == "bat" else 0.0)
    # seq cols are handled by TrainingPipeline.prepare_dataframe
    return df


def _build_bowling_share_targets(df: pd.DataFrame, spec: ModelSpec) -> pd.DataFrame:
    """Compute runs_share and wickets_share for Phase 3 share-target training."""
    for col in ("innings_runs", "innings_wickets"):
        if col not in df.columns:
            raise ValueError(f"share_targets requires {col} column (run export-dataset with updated schema)")
    df = df.copy()
    df["innings_runs"] = pd.to_numeric(df["innings_runs"], errors="coerce").fillna(0)
    df["innings_wickets"] = pd.to_numeric(df["innings_wickets"], errors="coerce").fillna(0)
    df = df[(df["innings_runs"] > 0) & (df["innings_wickets"] >= 0)]
    df["runs_share"] = (df["runs"].astype(float) / df["innings_runs"]).clip(upper=1.0)
    df["wickets_share"] = np.where(
        df["innings_wickets"] > 0,
        (df["wickets"].astype(float) / df["innings_wickets"]).clip(upper=1.0),
        0.0,
    )
    return df


# ── ModelSpec ────────────────────────────────────────────────────────────

BOWLING_SPEC = ModelSpec(
    name="bowling",
    feature_cols=FEATURE_COLS,
    target_cols=TARGET_COLS,
    seq_cols=BOWL_SEQ_COLS,
    target_cols_share=TARGET_COLS_SHARE,
    artifact_prefix="bowling",
    api_section="bowling",
    target_names_for_clip=["runs", "balls", "wickets"],
    use_scaler=True,
    use_multi_output=True,
    prepare_dataframe=_prepare_bowling_df,
    build_share_targets=_build_bowling_share_targets,
)


# ── Public API (backward-compatible) ─────────────────────────────────────

_pipeline = TrainingPipeline(BOWLING_SPEC)

load_dataset = _pipeline.load_dataset
load_dataset_from_memory = _pipeline.load_dataset_from_memory
fetch_bowling_from_api = _pipeline.fetch_from_api
train_and_save = _pipeline.train_and_save


def main():
    TrainingPipeline.run_cli(BOWLING_SPEC)


if __name__ == "__main__":
    main()
