import logging
import os
from typing import Iterable, List, Tuple

import numpy as np
import pandas as pd

logger = logging.getLogger(__name__)

# Default T20 sequence columns as documented in README (Plan 1.11)
BOWLING_SEQ_COLUMNS: List[str] = [
    "bowl_prev_bowler_id",
    "bowl_prev_phase",
    "bowl_prev_wkt_rate",
    "bowl_window_econ_24_death",
    "bowl_window_wkt_rate_24_death",
    "bowl_extras_wide_rate_pp",
    "bowl_react_after_boundary_wkt_rate_next",
    "bowl_spell_first_over_wkt_rate",
    "bowl_over_ball1_wkt_rate",
    "bowl_over_ball6_wkt_rate",
]

BATTING_SEQ_COLUMNS: List[str] = [
    "bat_prev_batter_id",
    "bat_prev_phase",
    "bat_prev_sr",
    "bat_prev_out_rate",
    "bat_window_sr_12_pp",
    "bat_window_boundary_rate_12_pp",
    "bat_entry_sr_1_6",
    "bat_set_sr_13_30",
    "bat_react_after_dot_sr",
    "bat_after_k_dots_boundary_p_k2",
]


def load_dataframe(path: str) -> pd.DataFrame:
    """Load a CSV into a pandas DataFrame.

    This is a thin wrapper so tests can rely on a single import path.
    """
    if not os.path.exists(path):
        raise FileNotFoundError(path)
    df = pd.read_csv(path)
    # Ensure column names are strings and trimmed
    df.columns = [str(c).strip() for c in df.columns]
    return df


def load_batting_dataframe(path: str) -> pd.DataFrame:
    return load_dataframe(path)


def load_bowling_dataframe(path: str) -> pd.DataFrame:
    return load_dataframe(path)


def build_feature_matrix(
    df: pd.DataFrame, feature_names: Iterable[str], fill_value: float = 0.0
) -> Tuple[np.ndarray, List[str]]:
    """Build a 2D numpy array X with columns in the given order.

    - If a feature is missing in the DataFrame, create a zero-filled column (fill_value).
    - Returns (X, used_feature_names) where used_feature_names is the final ordered list.
    - All outputs are float64 for compatibility with scikit-learn.
    """
    names = list(feature_names)
    cols: List[np.ndarray] = []
    missing_cols: List[str] = []
    non_numeric_cols: List[str] = []
    for name in names:
        if name in df.columns:
            col = df[name].to_numpy()
            try:
                col = col.astype(float, copy=False)
            except (TypeError, ValueError) as e:
                non_numeric_cols.append(name)
                logger.warning(
                    "seq_reader.build_feature_matrix non_numeric name=%s error=%s; filling with zeros",
                    name,
                    e,
                )
                col = np.zeros(shape=(len(df),), dtype=np.float64)
        else:
            missing_cols.append(name)
            col = np.full(shape=(len(df),), fill_value=float(fill_value), dtype=np.float64)
    if missing_cols:
        logger.info(
            "seq_reader.build_feature_matrix missing_columns count=%d names=%s (filled with %.2f)",
            len(missing_cols),
            missing_cols[:10] + (["..."] if len(missing_cols) > 10 else []),
            fill_value,
        )
        cols.append(col.reshape(-1, 1))
    if len(cols) == 0:
        # No requested columns; return empty matrix with correct n_rows
        return np.zeros((len(df), 0), dtype=np.float64), []
    X = np.hstack(cols).astype(np.float64, copy=False)
    return X, names
