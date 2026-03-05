"""
Train win model (match-level team1_wins 0/1) from go-app CSV export.

Uses enhanced feature set with per-player distribution statistics (mean, std,
max, min, top3_mean, count) and derived features (matchup ratios, bowling depth).
Trains a GradientBoosting classifier per format with walk-forward cross-validation.

Usage:
  python -m ml.train_win                             # CSV from GO_APP_OUTPUT_DIR
  python -m ml.train_win --csv path/to/win_export.csv
  GO_APP_URL=http://localhost:8080 python -m ml.train_win --cutoff 2024-12-01T00:00:00Z
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Optional

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import GradientBoostingClassifier
from sklearn.metrics import accuracy_score, brier_score_loss, log_loss
from sklearn.model_selection import TimeSeriesSplit

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from ml.config import (
    default_artifacts_dir,
    default_go_app_export_dir,
    get_pipeline_common_config,
    get_training_data_fetch_timeout_sec,
    get_training_params,
)
from ml.pipeline_common import compute_time_decay_weights
from ml.win_features import (
    DERIVED_FEATURE_COLS,
    WIN_ENHANCED_FEATURE_COLS,
    WIN_TARGET_COL,
    compute_derived_features,
)

# Backward-compatible alias: other modules (tuning/cv_metrics, model_metadata)
# reference train_win.WIN_FEATURE_COLS.
WIN_FEATURE_COLS = WIN_ENHANCED_FEATURE_COLS

logger = logging.getLogger(__name__)


def _concat_weights_win(weights_list: list[Optional[np.ndarray]]) -> Optional[np.ndarray]:
    """Concatenate per-format weights for unified model. Returns None if any format lacks weights."""
    if not weights_list or any(w is None for w in weights_list):
        return None
    return np.concatenate(weights_list)


def fetch_win_data(go_app_url: str, cutoff_iso: str, api_key=None):
    """Fetch training data from go-app; return dict with win headers and rows."""
    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format=all&cutoff={urllib.parse.quote(cutoff_iso)}&sections=win"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=get_training_data_fetch_timeout_sec()) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        logger.error(
            "train_win.fetch_win_data.http_error url=%s code=%s body_preview=%s",
            url,
            e.code,
            (body[:200] + "..." if len(body) > 200 else body),
        )
        raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
    except OSError as e:
        err_msg = str(e).strip()
        logger.error("train_win.fetch_win_data.os_error url=%s error=%s", url, e)
        hint = (
            "Go-app may have closed the connection before the response finished (e.g. server write timeout). "
            "Increase go-app server.http_write_timeout_sec (e.g. 600) in go-app/config.json and restart go-app."
        )
        if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
            raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
        raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
    return data.get("win") or {"headers": [], "rows": []}


def _add_derived_features(df: pd.DataFrame) -> pd.DataFrame:
    """Compute derived features (matchup ratios, depth, spreads) row by row and add as columns."""
    derived_rows = []
    for _, row in df.iterrows():
        derived_rows.append(compute_derived_features(row.to_dict()))
    derived_df = pd.DataFrame(derived_rows, index=df.index)
    return pd.concat([df, derived_df], axis=1)


def _available_feature_cols(df: pd.DataFrame) -> list[str]:
    """Return the subset of WIN_ENHANCED_FEATURE_COLS present in df (handles old exports gracefully)."""
    return [c for c in WIN_ENHANCED_FEATURE_COLS if c in df.columns]


def rows_to_xy_by_format(
    headers: list, rows: list[list]
) -> dict[str, tuple[np.ndarray, np.ndarray, Optional[np.ndarray], list[str]]]:
    """Build X, Y, weights, feature_names per format_code.

    Returns dict format_code -> (X, Y, sample_weight, feature_cols).
    """
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    all_possible = list(set(WIN_ENHANCED_FEATURE_COLS) | {WIN_TARGET_COL, "match_date", "format_code", "match_date_unix"})
    for c in all_possible:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")

    if any(c not in df.columns for c in DERIVED_FEATURE_COLS):
        df = _add_derived_features(df)

    feature_cols = _available_feature_cols(df)
    for c in feature_cols:
        df[c] = df[c].fillna(0.0)

    pipe_cfg = get_pipeline_common_config()
    halflife = pipe_cfg.get("time_decay_halflife_years", 2.0)

    def _weights(g: pd.DataFrame) -> Optional[np.ndarray]:
        if "match_date" not in g.columns:
            return None
        return compute_time_decay_weights(g["match_date"], halflife_years=halflife)

    if "format_code" not in df.columns:
        df = df.dropna(subset=[c for c in feature_cols if c in df.columns] + [WIN_TARGET_COL])
        if df.empty:
            return {}
        X = df[feature_cols].astype(float).values
        Y = df[WIN_TARGET_COL].astype(int).values
        w = _weights(df)
        return {"_ALL_": (X, Y, w, feature_cols)}

    out = {}
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in feature_cols if c in g.columns] + [WIN_TARGET_COL])
        if g.empty or len(g) < 10:
            continue
        X = g[feature_cols].astype(float).values
        Y = g[WIN_TARGET_COL].astype(int).values
        w = _weights(g)
        out[fmt] = (X, Y, w, feature_cols)
    return out


def _walk_forward_cv(
    X: np.ndarray,
    Y: np.ndarray,
    sample_weight: Optional[np.ndarray],
    params: dict,
    n_splits: int = 5,
) -> dict:
    """Run walk-forward (time series) cross-validation and return summary metrics."""
    tscv = TimeSeriesSplit(n_splits=n_splits)
    accuracies, briers, log_losses_list = [], [], []

    for train_idx, test_idx in tscv.split(X):
        X_train, X_test = X[train_idx], X[test_idx]
        Y_train, Y_test = Y[train_idx], Y[test_idx]
        w_train = sample_weight[train_idx] if sample_weight is not None else None

        model = GradientBoostingClassifier(
            n_estimators=params.get("n_estimators", 200),
            max_depth=params.get("max_depth", 4),
            learning_rate=params.get("learning_rate", 0.1),
            subsample=params.get("subsample", 0.8),
            random_state=params.get("random_state", 42),
        )
        if w_train is not None:
            model.fit(X_train, Y_train, sample_weight=w_train)
        else:
            model.fit(X_train, Y_train)

        proba = model.predict_proba(X_test)
        preds = model.predict(X_test)
        accuracies.append(accuracy_score(Y_test, preds))

        if proba.shape[1] == 2:
            briers.append(brier_score_loss(Y_test, proba[:, 1]))
            log_losses_list.append(log_loss(Y_test, proba[:, 1], labels=[0, 1]))

    return {
        "cv_accuracy_mean": float(np.mean(accuracies)),
        "cv_accuracy_std": float(np.std(accuracies)),
        "cv_brier_mean": float(np.mean(briers)) if briers else None,
        "cv_log_loss_mean": float(np.mean(log_losses_list)) if log_losses_list else None,
        "n_splits": n_splits,
        "per_fold_accuracy": [round(a, 4) for a in accuracies],
    }


def _get_gb_params(format_code: str) -> dict:
    """Get GradientBoosting parameters from config, with sensible defaults."""
    base_params = get_training_params("win", format_code)
    return {
        "n_estimators": base_params.get("n_estimators", 200),
        "max_depth": min(base_params.get("max_depth", 4), 6),
        "learning_rate": base_params.get("learning_rate", 0.1),
        "subsample": base_params.get("subsample", 0.8),
        "random_state": base_params.get("random_state", 42),
        "joblib_compress": base_params.get("joblib_compress", 3),
    }


def _fit_gradient_boosting(
    X: np.ndarray,
    Y: np.ndarray,
    params: dict,
    sample_weight: Optional[np.ndarray] = None,
) -> GradientBoostingClassifier:
    """Fit a GradientBoosting classifier with the given params and optional weights."""
    model = GradientBoostingClassifier(
        n_estimators=params["n_estimators"],
        max_depth=params["max_depth"],
        learning_rate=params["learning_rate"],
        subsample=params["subsample"],
        random_state=params["random_state"],
    )
    if sample_weight is not None:
        model.fit(X, Y, sample_weight=sample_weight)
    else:
        model.fit(X, Y)
    return model


def _build_model_metadata(
    model: GradientBoostingClassifier,
    X: np.ndarray,
    feature_cols: list[str],
    format_code: str,
    params: dict,
    cv_metrics: Optional[dict] = None,
) -> dict:
    """Build metadata dict for a trained win model."""
    importance = dict(zip(feature_cols, model.feature_importances_.tolist()))
    sorted_importance = dict(sorted(importance.items(), key=lambda x: x[1], reverse=True))
    meta: dict = {
        "format_code": format_code,
        "model_type": "GradientBoostingClassifier",
        "n_samples": int(X.shape[0]),
        "n_features": int(X.shape[1]),
        "feature_cols": feature_cols,
        "params": {k: v for k, v in params.items() if k != "joblib_compress"},
        "feature_importance_top20": dict(list(sorted_importance.items())[:20]),
    }
    if cv_metrics is not None:
        meta["cv_metrics"] = cv_metrics
    return meta


def _save_model_and_metadata(
    model: GradientBoostingClassifier,
    metadata: dict,
    out_dir: str,
    model_filename: str,
    metadata_filename: str,
    compress: int,
) -> None:
    """Persist model joblib and metadata JSON to out_dir."""
    os.makedirs(out_dir, exist_ok=True)
    joblib.dump(model, os.path.join(out_dir, model_filename), compress=compress)
    with open(os.path.join(out_dir, metadata_filename), "w") as f:
        json.dump(metadata, f, indent=2)


def train_and_save(
    X: np.ndarray,
    Y: np.ndarray,
    out_dir: str,
    format_code: str,
    feature_cols: list[str],
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train GradientBoosting win classifier, run walk-forward CV, save model + metadata."""
    params = _get_gb_params(format_code)

    cv_metrics = _walk_forward_cv(X, Y, sample_weight, params)
    logger.info(
        "train_win.walk_forward_cv format=%s cv_accuracy=%.4f±%.4f cv_brier=%s folds=%s",
        format_code,
        cv_metrics["cv_accuracy_mean"],
        cv_metrics["cv_accuracy_std"],
        cv_metrics.get("cv_brier_mean"),
        cv_metrics["per_fold_accuracy"],
    )

    model = _fit_gradient_boosting(X, Y, params, sample_weight)
    metadata = _build_model_metadata(model, X, feature_cols, format_code, params, cv_metrics)
    code = format_code.replace(" ", "_")
    _save_model_and_metadata(
        model, metadata, out_dir,
        f"win_model_{code}.joblib", f"win_model_{code}_metadata.json",
        params["joblib_compress"],
    )


def train_and_save_legacy(
    X: np.ndarray,
    Y: np.ndarray,
    out_dir: str,
    feature_cols: list[str],
    sample_weight: Optional[np.ndarray] = None,
) -> None:
    """Train one unified GradientBoosting win model on all data and save as legacy (win_model.joblib)."""
    params = _get_gb_params("_ALL_")
    model = _fit_gradient_boosting(X, Y, params, sample_weight)
    metadata = _build_model_metadata(model, X, feature_cols, "_ALL_", params)
    _save_model_and_metadata(
        model, metadata, out_dir,
        "win_model.joblib", "win_model_metadata.json",
        params["joblib_compress"],
    )
    logger.info("train_win.saved_unified out_dir=%s rows=%s", out_dir, X.shape[0])


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    ap = argparse.ArgumentParser(description="Train enhanced win model from go-app export CSV (preferred) or training-data API")
    ap.add_argument("--cutoff", default="", help="RFC3339 cutoff (required for API fallback)")
    ap.add_argument(
        "--csv",
        default="",
        help="Path to win CSV (optional; else use GO_APP_OUTPUT_DIR/win_encoded_all.csv or API)",
    )
    ap.add_argument("--out", default="", help="Artifacts output dir (default from config)")
    ap.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""), help="Go-app base URL")
    ap.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""), help="Optional API key")
    args = ap.parse_args()
    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR") or default_artifacts_dir()
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR") or default_go_app_export_dir()
    csv_path = args.csv or os.path.join(default_csv_dir, "win_encoded_all.csv")

    if os.path.isfile(csv_path):
        logger.info("train_win.loading_csv path=%s (prefer CSV over API)", csv_path)
        df = pd.read_csv(csv_path)
        headers = list(df.columns)
        rows = df.values.astype(str).tolist()
        by_format = rows_to_xy_by_format(headers, rows)
    else:
        if not args.go_app_url or not args.cutoff:
            logger.error(
                "train_win.csv_not_found path=%s hint=Run export-dataset first, or provide --go-app-url and --cutoff for API fallback",
                csv_path,
            )
            sys.exit(1)
        logger.warning(
            "train_win.csv_not_found path=%s falling_back_to_api hint=Run export-dataset first for faster training",
            csv_path,
        )
        logger.info("train_win.fetching_api go_app_url=%s cutoff=%s", args.go_app_url, args.cutoff)
        try:
            win = fetch_win_data(args.go_app_url, args.cutoff, args.api_key or None)
        except ValueError as e:
            logger.error("train_win.fetch_failed error=%s", e)
            sys.exit(1)
        headers = win.get("headers") or []
        rows = win.get("rows") or []
        by_format = rows_to_xy_by_format(headers, rows)

    if not by_format:
        logger.error("train_win.no_data hint=empty or insufficient rows")
        sys.exit(1)

    feature_cols: list[str] = []
    for fmt, (X, Y, w, fc) in by_format.items():
        feature_cols = fc
        logger.info("pipeline: train_win processing format=%s n=%s features=%s", fmt, X.shape[0], X.shape[1])
        train_and_save(X, Y, out_dir, fmt, fc, sample_weight=w)
        logger.info("train_win.saved format=%s n=%s out_dir=%s", fmt, X.shape[0], out_dir)

    all_X = np.vstack([X for _, (X, _, _, _) in by_format.items()])
    all_Y = np.concatenate([Y.ravel() for _, (_, Y, _, _) in by_format.items()])
    all_weights = _concat_weights_win([w for _, (_, _, w, _) in by_format.items()])
    if all_X.shape[0] >= 10:
        train_and_save_legacy(all_X, all_Y, out_dir, feature_cols, sample_weight=all_weights)


if __name__ == "__main__":
    main()
