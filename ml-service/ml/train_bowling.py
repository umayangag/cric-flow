import argparse
import json
import logging
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional, Tuple

import joblib
import numpy as np
import pandas as pd
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

from . import config as svc_config  # ml.config: loads config.json from ml-service root
from .config import get_training_params
from .feature_transforms import apply_transforms, get_transform_config
from .utils import make_base_estimator

logger = logging.getLogger(__name__)

# Minimal training script to produce placeholder artifacts for bowling
# Supports training per-format; artifacts saved with format suffixes when provided.
# By default consumes the Go export from ../../output/go-app/bowling_encoded.csv
# or bowling_encoded_<FORMAT>.csv when --format is set.
# Feature order must match configs/feature_vectors.json (bowling) for prediction.
# Seq columns: when absent in CSV, filled with 0.

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

FEATURE_COLS = [
    "bowling_consistency",
    "bowling_form",
    "bowling_form_short",
    "bowling_form_long",
    "bowling_momentum",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
    "inning",  # batting_inning in contracts; export column is inning
    "bowling_session",
    "toss",
    "bowling_venue",
    "bowling_opposition",
    "season_id",
] + BOWL_SEQ_COLS

TARGET_COLS = [
    "runs",  # runs_conceded
    "balls",  # deliveries
    "wickets",  # wickets_taken
    # econ may be absent; derive if missing
]


def _prepare_bowling_df(df: pd.DataFrame) -> pd.DataFrame:
    col_map = {
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
        "season_id": "season_id",
        "bowling_consistency": "bowling_consistency",
        "bowling_form": "bowling_form",
        "bowling_form_short": "bowling_form_short",
        "bowling_form_long": "bowling_form_long",
        "bowling_momentum": "bowling_momentum",
        "runs": "runs",
        "balls": "balls",
        "wickets": "wickets",
        "econ": "econ",
    }
    for c in BOWL_SEQ_COLS:
        col_map[c] = c
    df = df.rename(columns=col_map)
    for col in ("bowling_form_short", "bowling_form_long"):
        if col not in df.columns and "bowling_form" in df.columns:
            df[col] = df["bowling_form"]
    if "bowling_momentum" not in df.columns:
        df["bowling_momentum"] = 0.0
    for col in BOWL_SEQ_COLS:
        if col not in df.columns:
            df[col] = 0.0
        else:
            df[col] = df[col].fillna(0.0)
    return df


def _df_to_xy(df: pd.DataFrame) -> Tuple[np.ndarray, np.ndarray, List[str]]:
    """Build X, Y and feature_names from a prepared bowling DataFrame (align with train_batting)."""
    required = [c for c in FEATURE_COLS if c not in BOWL_SEQ_COLS]
    df = df.dropna(subset=[c for c in required if c in df.columns])
    X_raw = df[FEATURE_COLS].astype(float).values
    transform_config = get_transform_config("bowling")
    if transform_config.get("add_interactions") or transform_config.get("add_log1p"):
        X, feature_names_used = apply_transforms(X_raw, list(FEATURE_COLS), transform_config, "bowling")
<<<<<<< HEAD
    else:
        X = X_raw
        feature_names_used = list(FEATURE_COLS)
    y_cols = [c for c in TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if "econ" in df.columns:
        econ = df["econ"].astype(float).values.reshape(-1, 1)
    else:
        runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
        balls = df.get("balls", pd.Series(np.ones(len(df)) * 6)).astype(float).values
        overs = np.where(balls > 0, balls / 6.0, 1.0)
        econ = np.where(overs > 0, runs / overs, 0.0).reshape(-1, 1)
    needed = len(TARGET_COLS)
    if Y.shape[1] < needed:
        pad = np.zeros((Y.shape[0], needed - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    Y = np.concatenate([Y, econ], axis=1)
    return X, Y, feature_names_used
        X, _ = apply_transforms(X_raw, list(FEATURE_COLS), transform_config, "bowling")
=======
>>>>>>> 9256564 (fix review)
    else:
        X = X_raw
        feature_names_used = list(FEATURE_COLS)
    y_cols = [c for c in TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if "econ" in df.columns:
        econ = df["econ"].astype(float).values.reshape(-1, 1)
    else:
        runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
        balls = df.get("balls", pd.Series(np.ones(len(df)) * 6)).astype(float).values
        overs = np.where(balls > 0, balls / 6.0, 1.0)
        econ = np.where(overs > 0, runs / overs, 0.0).reshape(-1, 1)
    needed = len(TARGET_COLS)
    if Y.shape[1] < needed:
        pad = np.zeros((Y.shape[0], needed - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    Y = np.concatenate([Y, econ], axis=1)
    return X, Y, feature_names_used


def load_dataset(path: str) -> Tuple[np.ndarray, np.ndarray, List[str]]:
    if not os.path.exists(path):
        logger.error("train_bowling.load_dataset.file_not_found path=%s", path)
        raise FileNotFoundError(path)
    df = pd.read_csv(path)
    df = _prepare_bowling_df(df)
    return _df_to_xy(df)


def load_dataset_from_memory(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray, List[str]]:
    if not headers or not rows:
        return np.zeros((0, len(FEATURE_COLS))), np.zeros((0, 4)), list(FEATURE_COLS)
    df = pd.DataFrame(rows, columns=headers)
    df = _prepare_bowling_df(df)
    return _df_to_xy(df)


def fetch_bowling_from_api(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    api_key: Optional[str] = None,
) -> Dict[str, Any]:
    """Fetch bowling training data from go-app GET /api/backtest/training-data. Returns {headers, rows}."""
    from .config import get_training_data_fetch_timeout_sec

    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format={urllib.parse.quote(format_code)}&cutoff={urllib.parse.quote(cutoff_iso)}"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=get_training_data_fetch_timeout_sec()) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        logger.error("train_bowling.fetch_bowling_from_api.http_error url=%s code=%s", url, e.code)
        raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
    except OSError as e:
        err_msg = str(e).strip()
        logger.error("train_bowling.fetch_bowling_from_api.os_error url=%s error=%s", url, e)
        hint = (
            "Go-app may have closed the connection before the response finished (e.g. server write timeout). "
            "Increase go-app server.http_write_timeout_sec (e.g. 600) in go-app/config.json and restart go-app."
        )
        if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
            raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
        raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
    return data.get("bowling") or {"headers": [], "rows": []}


def train_and_save(
    X,
    Y,
    out_dir: str,
    training_params: dict,
    suffix: Optional[str] = None,
    metadata: Optional[dict] = None,
):
    """Train and save artifacts. training_params must come from get_training_params("bowling") (config only).

    Normalizes X with StandardScaler (fit on provided data); Y kept in raw units.
    See docs/ml-and-training.md.
    """
    os.makedirs(out_dir, exist_ok=True)
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    compress = training_params["joblib_compress"]
    base_est = make_base_estimator(training_params)
    model = MultiOutputRegressor(base_est)
    model.fit(Xs, Y)

    # Extract and store feature importance (average across MultiOutputRegressor estimators)
    feature_names_for_importance = metadata.get("feature_names") if metadata else None
    if feature_names_for_importance is None:
        feature_names_for_importance = FEATURE_COLS
    feature_importance = None
    if hasattr(model, "estimators_") and len(model.estimators_) > 0:
        imps = []
        for est in model.estimators_:
            if hasattr(est, "feature_importances_"):
                imps.append(est.feature_importances_)
        if imps:
            n_f = min(len(feature_names_for_importance), len(imps[0]))
            feature_importance = {
                feature_names_for_importance[i]: float(np.mean([arr[i] for arr in imps])) for i in range(n_f)
            }
            top = sorted(feature_importance.items(), key=lambda x: -x[1])[:5]
            logger.info("train_bowling.feature_importance_top5 %s", top)

    if suffix:
        joblib.dump(scaler, os.path.join(out_dir, f"bowling_scaler_{suffix}.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, f"bowling_model_{suffix}.joblib"), compress=compress)
    else:
        joblib.dump(scaler, os.path.join(out_dir, "bowling_scaler.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, "bowling_model.joblib"), compress=compress)
    # Save training metadata if provided (include feature importance)
    if metadata is not None:
        if feature_importance is not None:
            metadata["feature_importance"] = feature_importance
        meta_path = os.path.join(out_dir, f"bowling_metadata_{suffix or 'LEGACY'}.json")
        try:
            with open(meta_path, "w", encoding="utf-8") as f:
                json.dump(metadata, f, indent=2)
        except OSError as e:
            logger.warning("train_bowling.train_and_save.metadata_save_failed path=%s error=%s", meta_path, e)


def _config_formats() -> list[str]:
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f)
            fmts = data.get("ml", {}).get("formats") or []
            return [str(x).upper() for x in fmts if isinstance(x, (str, int))]
    except Exception:
        return []


def main():
    parser = argparse.ArgumentParser()
    # Default input CSV from GO_APP_OUTPUT_DIR or config.json
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
    # Default output dir from ML_SERVICE_OUTPUT_DIR or config.json
    default_out_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

    parser.add_argument(
        "--csv",
        default="",
        help="Path to bowling CSV (overrides format-based resolution)",
    )
    parser.add_argument(
        "--out",
        default=default_out_dir,
        help="Output dir for artifacts (default from ML_SERVICE_OUTPUT_DIR or ../../output/ml-service)",
    )
    parser.add_argument(
        "--format",
        default="",
        help="Single format code (e.g., ODI, T20I). When set, reads bowling_encoded_<FORMAT>.csv.",
    )
    parser.add_argument(
        "--formats",
        default="",
        help="Comma-separated list of formats to train. Overrides --format.",
    )
    parser.add_argument(
        "--all-formats",
        action="store_true",
        help="Train for all formats from config (ml.formats).",
    )
    parser.add_argument(
        "--from-api",
        action="store_true",
        help="Fetch training data from go-app API (GO_APP_URL + cutoff) instead of CSV. Same contract as fielding/extras/win.",
    )
    parser.add_argument(
        "--cutoff",
        default="",
        help="RFC3339 cutoff for API fetch (required if --from-api).",
    )
    parser.add_argument(
        "--go-app-url",
        default=os.environ.get("GO_APP_URL", ""),
        help="Go-app base URL for --from-api (default: GO_APP_URL).",
    )
    parser.add_argument(
        "--api-key",
        default=os.environ.get("GO_APP_API_KEY", ""),
        help="Optional API key for go-app (default: GO_APP_API_KEY).",
    )
    args = parser.parse_args()

    targets: list[str] = []
    if args.all_formats:
        targets = _config_formats()
    elif args.formats:
        targets = [s.strip().upper() for s in args.formats.split(",") if s.strip()]
    elif args.format:
        targets = [args.format.strip().upper()]

    # Consistent data source: fetch from go-app API (same as fielding/extras/win)
    if args.from_api:
        cutoff = (args.cutoff or "").strip()
        go_app_url = (args.go_app_url or os.environ.get("GO_APP_URL", "")).strip()
        if not cutoff or not go_app_url:
            logger.error(
                "train_bowling.from_api_requires cutoff and go_app_url (or GO_APP_URL)",
                has_cutoff=bool(cutoff),
                has_go_app_url=bool(go_app_url),
            )
            raise SystemExit(1)
        if not targets:
            targets = _config_formats()
        api_key = (args.api_key or os.environ.get("GO_APP_API_KEY", "")).strip() or None
        saved_count = 0
        for fmt in targets:
            bowl = fetch_bowling_from_api(go_app_url, fmt, cutoff, api_key)
            headers = bowl.get("headers") or []
            rows = bowl.get("rows") or []
            if not headers or not rows:
                logger.warning("train_bowling.skip_format_no_data_from_api format=%s", fmt)
                continue
            try:
                X, Y, feature_names_used = load_dataset_from_memory(headers, rows)
            except Exception as e:
                logger.error("train_bowling.load_from_api_failed format=%s error=%s", fmt, e)
                continue
            if X.size == 0 or Y.size == 0:
                logger.warning("train_bowling.skip_format_no_data format=%s", fmt)
                continue
            training_params = get_training_params("bowling", fmt)
            meta = {
                "source": "api",
                "format": fmt,
                "cutoff": cutoff,
                "rows": int(X.shape[0]),
                "n_features": int(X.shape[1]),
                "n_targets": int(Y.shape[1]),
                "model": "RandomForestRegressor",
                "hyperparams": training_params,
                "feature_names": feature_names_used,
            }
            train_and_save(X, Y, args.out, training_params, fmt, meta)
            logger.info("train_bowling.saved_format format=%s out_dir=%s rows=%s", fmt, args.out, int(X.shape[0]))
            saved_count += 1
        if targets and saved_count == 0:
            logger.error("train_bowling.no_models_saved from_api=True cutoff=%s", cutoff)
            raise SystemExit(1)
        return

    # Auto-detect formats when none explicitly provided
    if not targets and not args.csv:
        # 1) Prefer formats from config that actually exist on disk
        cfg_fmts = _config_formats()
        existing_cfg_fmts = [
            f for f in cfg_fmts if os.path.exists(os.path.join(default_csv_dir, f"bowling_encoded_{f}.csv"))
        ]
        if existing_cfg_fmts:
            targets = existing_cfg_fmts
        else:
            # 2) Otherwise, glob for bowling_encoded_*.csv in the export directory
            try:
                for name in os.listdir(default_csv_dir):
                    if name.startswith("bowling_encoded_") and name.endswith(".csv"):
                        suffix = name[len("bowling_encoded_") : -len(".csv")]
                        if suffix:
                            targets.append(str(suffix).upper())
            except Exception:
                pass

    # If still no targets detected, fall back to legacy single CSV path
    if not targets:
        training_params = get_training_params("bowling", None)
        csv_path = args.csv or os.path.join(default_csv_dir, "bowling_encoded.csv")
        try:
            X, Y, feature_names_used = load_dataset(csv_path)
        except FileNotFoundError as e:
            logger.error("train_bowling.legacy_csv_not_found path=%s error=%s", csv_path, e)
            raise SystemExit(1) from e
        if X.size == 0 or Y.size == 0:
            logger.error("train_bowling.no_data path=%s", csv_path)
            return
        meta = {
            "csv_path": csv_path,
            "rows": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "n_targets": int(Y.shape[1]),
            "format": None,
            "model": "RandomForestRegressor",
            "hyperparams": training_params,
            "feature_names": feature_names_used,
        }
        train_and_save(X, Y, args.out, training_params, None, meta)
        logger.info("train_bowling.saved_legacy out_dir=%s", args.out)
        return

    saved_count = 0
    for fmt in targets:
        training_params = get_training_params("bowling", fmt)
        csv_path = args.csv or os.path.join(default_csv_dir, f"bowling_encoded_{fmt}.csv")
        if not os.path.exists(csv_path):
            logger.warning("train_bowling.skip_format_csv_not_found format=%s path=%s", fmt, csv_path)
            continue
        try:
            X, Y, feature_names_used = load_dataset(csv_path)
        except Exception as e:
            logger.error("train_bowling.load_dataset_failed format=%s path=%s error=%s", fmt, csv_path, e)
            continue
        if X.size == 0 or Y.size == 0:
            logger.warning("train_bowling.skip_format_no_data format=%s path=%s", fmt, csv_path)
            continue
        meta = {
            "csv_path": csv_path,
            "rows": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "n_targets": int(Y.shape[1]),
            "format": fmt,
            "model": "RandomForestRegressor",
            "hyperparams": training_params,
            "feature_names": feature_names_used,
        }
        train_and_save(X, Y, args.out, training_params, fmt, meta)
        logger.info("train_bowling.saved_format format=%s out_dir=%s rows=%s", fmt, args.out, int(X.shape[0]))
        saved_count += 1
    if targets and saved_count == 0:
        logger.error("train_bowling.no_models_saved csv_dir=%s out_dir=%s", default_csv_dir, args.out)
        raise SystemExit(1)


if __name__ == "__main__":
    main()
