"""
Train batting model (runs, balls, fours, sixes, batting_position).

Thin wrapper around TrainingPipeline — defines batting-specific ModelSpec
(feature cols, target cols, seq cols, DataFrame preparation, share targets).
All shared logic lives in training_pipeline.py.

Usage:
  python -m ml.train_batting                          # CSV from GO_APP_OUTPUT_DIR
  python -m ml.train_batting --from-api --cutoff ...  # fetch from go-app API
  python -m ml.train_batting --all-formats            # all formats from config
"""

from __future__ import annotations

import logging
from typing import Dict

import pandas as pd

from .training_pipeline import ModelSpec, TrainingPipeline

logger = logging.getLogger(__name__)

# Raw windowed stats (v2) from configs/feature_vectors.json (single source of truth).
try:
    from app.feature_config import get_raw_stat_feature_names

    BAT_RAW_STAT_COLS = get_raw_stat_feature_names("batting")
except (ImportError, RuntimeError):  # noqa: S110 (allow broad except for optional app dependency at import)
    # Fallback when app not available (e.g. some test envs); must match Go contract.
    BAT_RAW_STAT_COLS = [
        "batting_mean_w3",
        "batting_mean_w5",
        "batting_mean_w10",
        "batting_mean_w20",
        "batting_std_w5",
        "batting_std_w10",
        "batting_max_w10",
        "batting_min_w10",
        "batting_median_w10",
        "batting_last_1",
        "batting_last_2",
        "batting_last_3",
        "batting_career_mean",
        "batting_career_count",
        "batting_pct_zero_w10",
        "batting_trend_w5",
        "batting_days_since_last",
        "batting_innings_in_last_90d",
    ]

# ── Column definitions ───────────────────────────────────────────────────

BAT_SEQ_COLS = [
    "bat_prev_sr",
    "bat_prev_out_rate",
    "bat_window_sr_12_pp",
    "bat_window_boundary_rate_12_pp",
    "bat_entry_sr_1_6",
    "bat_set_sr_13_30",
    "bat_react_after_dot_sr",
    "bat_after_k_dots_boundary_p_k2",
]

FEATURE_COLS = (
    [
        "batting_consistency",
        "batting_form",
        "batting_form_short",
        "batting_form_long",
        "batting_momentum",
    ]
    + BAT_RAW_STAT_COLS
    + [
        "temp",
        "wind",
        "rain",
        "humidity",
        "cloud",
        "pressure",
        "viscosity",
        "inning",
        "batting_session",
        "toss",
        "batting_venue",
        "batting_opposition",
        "season_id",
        "match_date_unix",
    ]
    + BAT_SEQ_COLS
)

TARGET_COLS = [
    "runs",
    "balls",
    "fours",
    "sixes",
    "batting_position",
]

TARGET_COLS_SHARE = ["runs_share", "balls", "fours", "sixes", "batting_position"]


# ── DataFrame preparation hook ───────────────────────────────────────────


def _batting_col_map() -> Dict[str, str]:
    m = {
        "temp": "temp",
        "wind": "wind",
        "rain": "rain",
        "humidity": "humidity",
        "cloud": "cloud",
        "pressure": "pressure",
        "viscosity": "viscosity",
        "inning": "inning",
        "batting_session": "batting_session",
        "toss": "toss",
        "batting_venue": "batting_venue",
        "batting_opposition": "batting_opposition",
        "season_id": "season_id",
        "match_date_unix": "match_date_unix",
        "batting_consistency": "batting_consistency",
        "batting_form": "batting_form",
        "batting_form_short": "batting_form_short",
        "batting_form_long": "batting_form_long",
        "batting_momentum": "batting_momentum",
    }
    m.update({c: c for c in BAT_RAW_STAT_COLS})
    m.update(
        {
            "runs": "runs",
            "balls": "balls",
            "fours": "fours",
            "sixes": "sixes",
            "batting_position": "batting_position",
            "strike_rate": "strike_rate",
            "innings_runs": "innings_runs",
        }
    )
    for c in BAT_SEQ_COLS:
        m[c] = c
    return m


def _prepare_batting_df(df: pd.DataFrame, spec: ModelSpec) -> pd.DataFrame:
    """Normalize column names and fill missing columns for batting data."""
    df = df.rename(columns=_batting_col_map())
    for col in ("batting_form_short", "batting_form_long"):
        if col not in df.columns and "batting_form" in df.columns:
            df[col] = df["batting_form"]
    if "batting_momentum" not in df.columns:
        df["batting_momentum"] = 0.0
    # seq cols are handled by TrainingPipeline.prepare_dataframe
    return df


def _build_batting_share_targets(df: pd.DataFrame, spec: ModelSpec) -> pd.DataFrame:
    """Compute runs_share = runs / innings_runs for Phase 3 share-target training."""
    if "innings_runs" not in df.columns:
        raise ValueError("share_targets requires innings_runs column (run export-dataset with updated schema)")
    df = df.copy()
    df["innings_runs"] = pd.to_numeric(df["innings_runs"], errors="coerce").fillna(0)
    df = df[df["innings_runs"] > 0]
    df["runs_share"] = (df["runs"].astype(float) / df["innings_runs"]).clip(upper=1.0)
    return df


# ── ModelSpec ────────────────────────────────────────────────────────────

BATTING_SPEC = ModelSpec(
    name="batting",
    feature_cols=FEATURE_COLS,
    target_cols=TARGET_COLS,
    seq_cols=BAT_SEQ_COLS,
    target_cols_share=TARGET_COLS_SHARE,
    artifact_prefix="batting",
    api_section="batting",
    target_names_for_clip=["runs", "balls", "fours", "sixes", "batting_position"],
    use_scaler=True,
    use_multi_output=True,
    prepare_dataframe=_prepare_batting_df,
    build_share_targets=_build_batting_share_targets,
)


# ── Public API (backward-compatible) ─────────────────────────────────────

_pipeline = TrainingPipeline(BATTING_SPEC)

load_dataset = _pipeline.load_dataset
load_dataset_from_memory = _pipeline.load_dataset_from_memory
fetch_batting_from_api = _pipeline.fetch_from_api
train_and_save = _pipeline.train_and_save


def main():
    TrainingPipeline.run_cli(BATTING_SPEC)


if __name__ == "__main__":
    main()
