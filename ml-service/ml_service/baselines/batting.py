from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Tuple

import joblib  # type: ignore
import numpy as np
import pandas as pd
from sklearn.linear_model import LogisticRegression  # type: ignore
from sklearn.preprocessing import StandardScaler  # type: ignore
from sklearn.pipeline import Pipeline  # type: ignore

from ml_service.datasets import (
    load_batting_dataframe,
    build_feature_matrix,
    BATTING_SEQ_COLUMNS,
)


@dataclass
class TrainResult:
    pipeline: Pipeline
    n_rows: int
    n_features: int


def _ensure_output_dir(path: str) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)


def train_from_csv(
    csv_path: str,
    feature_names: Tuple[str, ...] = tuple(BATTING_SEQ_COLUMNS),
    target_column: str = "runs",
    model_out_path: str = "output/ml-service/batting_baseline.joblib",
    random_state: int = 42,
) -> TrainResult:
    """
    Train a tiny baseline model using the given CSV and feature names.
    - Target is a simple derived binary: runs > 0 → 1 else 0, if target column exists.
    - If target column missing, uses a dummy zero vector (still fits but not meaningful).
    """
    df = load_batting_dataframe(csv_path)
    X, used = build_feature_matrix(df, feature_names, fill_value=0.0)

    if target_column in df.columns:
        y = (df[target_column].values.astype(float) > 0).astype(int)
    else:
        y = np.zeros((X.shape[0],), dtype=int)

    # Very small, deterministic pipeline
    pipe = Pipeline([
        ("scaler", StandardScaler(with_mean=True, with_std=True)),
        ("clf", LogisticRegression(max_iter=100, random_state=random_state)),
    ])
    pipe.fit(X, y)

    _ensure_output_dir(model_out_path)
    joblib.dump({"pipeline": pipe, "features": list(used)}, model_out_path)
    return TrainResult(pipeline=pipe, n_rows=X.shape[0], n_features=X.shape[1])
