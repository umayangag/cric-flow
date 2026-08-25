from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Tuple

import joblib  # type: ignore
import numpy as np
from sklearn.linear_model import LogisticRegression  # type: ignore
from sklearn.pipeline import Pipeline  # type: ignore
from sklearn.preprocessing import StandardScaler  # type: ignore

from ml.datasets import (
    BOWLING_SEQ_COLUMNS,
    build_feature_matrix,
    load_bowling_dataframe,
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
    feature_names: Tuple[str, ...] = tuple(BOWLING_SEQ_COLUMNS),
    target_column: str = "balls",
    model_out_path: str = "output/ml-service/bowling_baseline.joblib",
    random_state: int = 42,
) -> TrainResult:
    """
    Train a tiny baseline model using the given CSV and feature names.
    - Default dummy target: balls > 6 → 1 else 0 when target column is present.
    - If target missing, uses zeros; still fits deterministically.
    """
    df = load_bowling_dataframe(csv_path)
    X, used = build_feature_matrix(df, feature_names, fill_value=0.0)

    if target_column in df.columns:
        y = (df[target_column].values.astype(float) > 6).astype(int)
    else:
        y = np.zeros((X.shape[0],), dtype=int)

    pipe = Pipeline(
        [
            ("scaler", StandardScaler(with_mean=True, with_std=True)),
            ("clf", LogisticRegression(max_iter=100, random_state=random_state)),
        ]
    )
    pipe.fit(X, y)

    _ensure_output_dir(model_out_path)
    joblib.dump({"pipeline": pipe, "features": list(used)}, model_out_path)
    return TrainResult(pipeline=pipe, n_rows=X.shape[0], n_features=X.shape[1])
