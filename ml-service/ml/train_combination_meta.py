"""
Train a meta-model for combining batting/bowling/fielding scores in team selection.

Learns optimal weights from backtest/evaluate outcomes. Uses Ridge regression on
(bat_score, bowl_score, field_score, is_keeper, format_encoded) -> target (actual
contribution). Outputs JSON coefficients that can be used by go-app predict_team
via selection.meta_model_path or manually copied into score_weights.

Expected CSV columns: bat_score, bowl_score, field_score, is_keeper, format, target
- Scores should be normalized (0-1) as in predict_team.
- is_keeper: 0 or 1.
- format: T20, ODI, TEST (one-hot encoded internally).
- target: actual contribution metric (e.g. runs_scored/divisor, wickets/divisor, or
  a composite from evaluate-db outcomes).

Usage:
  python -m ml.train_combination_meta --csv path/to/backtest_contributions.csv --out combination_meta.json
  python -m ml.train_combination_meta --csv data.csv --out combo.json --alpha 1.0 --per-format
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys

import numpy as np
import pandas as pd
from sklearn.linear_model import Ridge
from sklearn.preprocessing import OneHotEncoder

_ML_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _ML_ROOT not in sys.path:
    sys.path.insert(0, _ML_ROOT)

logger = logging.getLogger(__name__)

REQUIRED_COLS = ["bat_score", "bowl_score", "field_score", "is_keeper", "format", "target"]


def load_dataset(path: str) -> tuple[np.ndarray, np.ndarray, list[str], list[str] | None]:
    """Load CSV, build X (numeric + one-hot format), Y. Returns (X, Y, feature_names, format_encoder_categories)."""
    df = pd.read_csv(path)
    for c in REQUIRED_COLS:
        if c not in df.columns:
            raise ValueError(f"CSV must have column {c!r}")
    df = df.dropna(subset=REQUIRED_COLS)
    X_base = df[["bat_score", "bowl_score", "field_score", "is_keeper"]].astype(float).values
    formats = df["format"].astype(str).str.strip().str.upper()
    enc = OneHotEncoder(sparse_output=False, handle_unknown="ignore")
    fmt_encoded = enc.fit_transform(formats.values.reshape(-1, 1))
    fmt_cats = enc.categories_[0].tolist() if hasattr(enc, "categories_") else []
    X = np.hstack([X_base, fmt_encoded])
    Y = df["target"].astype(float).values
    base_names = ["bat_score", "bowl_score", "field_score", "is_keeper"]
    fmt_names = [f"format_{c}" for c in fmt_cats]
    feature_names = base_names + fmt_names
    return X, Y, feature_names, fmt_cats if fmt_cats else None


def train_and_export(
    csv_path: str,
    out_path: str,
    alpha: float = 1.0,
    per_format: bool = False,
    normalize_weights: bool = True,
) -> dict:
    """
    Train Ridge meta-model and export coefficients to JSON.
    per_format: if True, fit separate Ridge per format and output per-format weights.
    """
    X, Y, feature_names, format_cats = load_dataset(csv_path)
    if X.shape[0] < 10:
        logger.warning("train_combination_meta.low_samples n=%d", X.shape[0])

    if per_format and format_cats:
        # Per-format models: X_base only (bat, bowl, field, is_keeper)
        X_base = X[:, :4]
        formats_col = pd.read_csv(csv_path)["format"].astype(str).str.strip().str.upper()
        out_weights: dict = {"per_format": {}}
        for fmt in format_cats:
            mask = formats_col == fmt
            if mask.sum() < 5:
                continue
            X_fmt = X_base[mask]
            Y_fmt = Y[mask]
            model = Ridge(alpha=alpha, random_state=42).fit(X_fmt, Y_fmt)
            coef = model.coef_
            out_weights["per_format"][fmt] = {
                "bat": float(coef[0]),
                "bowl": float(coef[1]),
                "field": float(coef[2]),
                "keeper_bonus": float(coef[3]),
                "intercept": float(model.intercept_),
            }
        result = out_weights
    else:
        # Single global model
        model = Ridge(alpha=alpha, random_state=42).fit(X, Y)
        coef = model.coef_
        # Map to score_weights: bat, bowl, field, keeper_bonus
        bat = float(coef[0])
        bowl = float(coef[1])
        field = float(coef[2])
        keeper_bonus = float(coef[3])
        if normalize_weights:
            total = abs(bat) + abs(bowl) + abs(field) + abs(keeper_bonus)
            if total > 0:
                bat, bowl, field, keeper_bonus = bat / total, bowl / total, field / total, keeper_bonus / total
        result = {
            "bat": float(bat),
            "bowl": float(bowl),
            "field": float(field),
            "keeper_bonus": float(keeper_bonus),
            "intercept": float(model.intercept_),
            "feature_names": feature_names,
            "n_samples": int(X.shape[0]),
        }

    os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(result, f, indent=2)
    logger.info("train_combination_meta.saved path=%s", out_path)
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description="Train meta-model for combining bat/bowl/field scores")
    parser.add_argument(
        "--csv", required=True, help="CSV with bat_score, bowl_score, field_score, is_keeper, format, target"
    )
    parser.add_argument("--out", required=True, help="Output JSON path for coefficients")
    parser.add_argument("--alpha", type=float, default=1.0, help="Ridge alpha (default 1.0)")
    parser.add_argument("--per-format", action="store_true", help="Fit separate model per format")
    parser.add_argument(
        "--no-normalize-weights",
        action="store_false",
        dest="normalize_weights",
        default=True,
        help="If set, output raw Ridge coefficients; otherwise normalize so weights sum to ~1 (default)",
    )
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    train_and_export(
        args.csv, args.out, alpha=args.alpha, per_format=args.per_format, normalize_weights=args.normalize_weights
    )
    print("Coefficients written to", args.out)


if __name__ == "__main__":
    main()
