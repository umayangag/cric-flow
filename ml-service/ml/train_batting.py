import argparse
import json
import logging
import os
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
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

# Minimal training script to produce placeholder artifacts compatible with app.main
# Supports training per-format; artifacts saved with format suffixes when provided.
# By default consumes the Go export from ../../output/go-app/batting_encoded.csv (legacy)
# or batting_encoded_<FORMAT>.csv when --format is set.
# Feature order must match configs/feature_vectors.json (batting) for prediction.
# Seq columns: when absent in CSV, filled with 0.

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

FEATURE_COLS = [
    "batting_consistency",
    "batting_form",
    "batting_form_short",
    "batting_form_long",
    "batting_momentum",
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
] + BAT_SEQ_COLS

TARGET_COLS = [
    "runs",  # runs_scored
    "balls",  # balls_faced
    "fours",  # fours_scored
    "sixes",  # sixes_scored
    "batting_position",
    # strike_rate may be absent; derive if missing
]


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
        "batting_consistency": "batting_consistency",
        "batting_form": "batting_form",
        "batting_form_short": "batting_form_short",
        "batting_form_long": "batting_form_long",
        "batting_momentum": "batting_momentum",
        "runs": "runs",
        "balls": "balls",
        "fours": "fours",
        "sixes": "sixes",
        "batting_position": "batting_position",
        "strike_rate": "strike_rate",
    }
    for c in BAT_SEQ_COLS:
        m[c] = c
    return m


def _prepare_batting_df(df: pd.DataFrame) -> pd.DataFrame:
    """Normalize column names and fill missing columns (for CSV or API-sourced DataFrame)."""
    df = df.rename(columns=_batting_col_map())
    for col in ("batting_form_short", "batting_form_long"):
        if col not in df.columns and "batting_form" in df.columns:
            df[col] = df["batting_form"]
    if "batting_momentum" not in df.columns:
        df["batting_momentum"] = 0.0
    for col in BAT_SEQ_COLS:
        if col not in df.columns:
            df[col] = 0.0
        else:
            df[col] = df[col].fillna(0.0)
    return df


def _df_to_xy(df: pd.DataFrame) -> Tuple[np.ndarray, np.ndarray, List[str]]:
    """Build X, Y and feature_names from a prepared batting DataFrame."""
    required = [c for c in FEATURE_COLS if c not in BAT_SEQ_COLS]
    required_in_df = [c for c in required if c in df.columns]
    # Fill NaN in feature columns with 0 so export with NULL form/consistency/venue/opposition (e.g. precompute not run) still yields trainable rows
    for c in required_in_df:
        df[c] = df[c].fillna(0.0)
    # Drop only rows missing essential targets (runs/balls) so we don't train on invalid labels
    target_subset = [c for c in TARGET_COLS if c in df.columns]
    if target_subset:
        df = df.dropna(subset=target_subset)
    X_raw = df[FEATURE_COLS].astype(float).values
    transform_config = get_transform_config("batting")
    if transform_config.get("add_interactions") or transform_config.get("add_log1p"):
        X, feature_names_used = apply_transforms(X_raw, list(FEATURE_COLS), transform_config, "batting")
    else:
        X = X_raw
        feature_names_used = list(FEATURE_COLS)
    y_cols = [c for c in TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if "strike_rate" in df.columns:
        sr = df["strike_rate"].astype(float).values.reshape(-1, 1)
    else:
        runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
        balls = df.get("balls", pd.Series(np.ones(len(df)))).astype(float).values
        sr = np.where(balls > 0, (runs / balls) * 100.0, 0.0).reshape(-1, 1)
    needed = len(TARGET_COLS)
    if Y.shape[1] < needed:
        pad = np.zeros((Y.shape[0], needed - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    Y = np.concatenate([Y, sr], axis=1)
    return X, Y, feature_names_used


def load_dataset(path: str) -> Tuple[np.ndarray, np.ndarray, List[str]]:
    if not os.path.exists(path):
        logger.error("train_batting.load_dataset.file_not_found path=%s", path)
        raise FileNotFoundError(path)
    df = pd.read_csv(path)
    df = _prepare_batting_df(df)
    return _df_to_xy(df)


def load_dataset_from_memory(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray, List[str]]:
    """Build X, Y from API-style (headers, rows). Same contract as load_dataset."""
    if not headers or not rows:
        return np.zeros((0, len(FEATURE_COLS))), np.zeros((0, 6)), list(FEATURE_COLS)
    df = pd.DataFrame(rows, columns=headers)
    df = _prepare_batting_df(df)
    return _df_to_xy(df)


def fetch_batting_from_api(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    api_key: Optional[str] = None,
) -> Dict[str, Any]:
    """Fetch batting training data from go-app GET /api/backtest/training-data. Returns {headers, rows}."""
    from .config import get_training_data_fetch_timeout_sec

    base = go_app_url.rstrip("/")
    url = f"{base}/api/backtest/training-data?format={urllib.parse.quote(format_code)}&cutoff={urllib.parse.quote(cutoff_iso)}&sections=batting"
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=get_training_data_fetch_timeout_sec()) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        logger.error("train_batting.fetch_batting_from_api.http_error url=%s code=%s", url, e.code)
        raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
    except OSError as e:
        err_msg = str(e).strip()
        logger.error("train_batting.fetch_batting_from_api.os_error url=%s error=%s", url, e)
        hint = (
            "Go-app may have closed the connection before the response finished (e.g. server write timeout). "
            "Increase go-app server.http_write_timeout_sec (e.g. 600) in go-app/config.json and restart go-app."
        )
        if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
            raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
        raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
    return data.get("batting") or {"headers": [], "rows": []}


def train_and_save(
    X,
    Y,
    out_dir: str,
    training_params: dict,
    suffix: Optional[str] = None,
    metadata: Optional[dict] = None,
    transform_config: Optional[dict] = None,
):
    """Train and save artifacts. training_params must come from get_training_params("batting") (config only).

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
            logger.info("train_batting.feature_importance_top5 %s", top)

    if suffix:
        joblib.dump(scaler, os.path.join(out_dir, f"batting_scaler_{suffix}.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, f"batting_model_{suffix}.joblib"), compress=compress)
    else:
        joblib.dump(scaler, os.path.join(out_dir, "batting_scaler.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, "batting_model.joblib"), compress=compress)
    # Save training metadata if provided (include feature importance and feature_transforms)
    if metadata is not None:
        if feature_importance is not None:
            metadata["feature_importance"] = feature_importance
        if transform_config:
            metadata["feature_transforms"] = transform_config
        meta_path = os.path.join(out_dir, f"batting_metadata_{suffix or 'LEGACY'}.json")
        try:
            with open(meta_path, "w", encoding="utf-8") as f:
                json.dump(metadata, f, indent=2)
        except OSError as e:
            logger.warning("train_batting.train_and_save.metadata_save_failed path=%s error=%s", meta_path, e)


def _config_formats() -> list[str]:
    # Try to read ml.formats from ml-service/config.json via raw JSON
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f)
            fmts = data.get("ml", {}).get("formats") or []
            return [str(x).upper() for x in fmts if isinstance(x, (str, int))]
    except Exception:
        return []


def main():
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    parser = argparse.ArgumentParser()
    # Default input CSV from GO_APP_OUTPUT_DIR or ../../output/go-app
    default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
    default_out_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

    parser.add_argument(
        "--csv",
        default="",
        help="Path to batting CSV (overrides format-based resolution)",
    )
    parser.add_argument(
        "--out",
        default=default_out_dir,
        help="Output dir for artifacts (default from ML_SERVICE_OUTPUT_DIR or ../../output/ml-service)",
    )
    parser.add_argument(
        "--format",
        default="",
        help="Single format code (e.g., ODI, T20I). When set, reads batting_encoded_<FORMAT>.csv.",
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

    logger.info(
        "pipeline: train_batting starting out_dir=%s from_api=%s all_formats=%s",
        args.out,
        args.from_api,
        args.all_formats,
    )

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
                "train_batting.from_api_requires cutoff and go_app_url (or GO_APP_URL)",
                has_cutoff=bool(cutoff),
                has_go_app_url=bool(go_app_url),
            )
            raise SystemExit(1)
        if not targets:
            targets = _config_formats()
        api_key = (args.api_key or os.environ.get("GO_APP_API_KEY", "")).strip() or None
        logger.info("pipeline: train_batting fetching data from API formats=%s cutoff=%s", targets, cutoff)

        def _train_one_api(fmt: str) -> int:
            logger.info("pipeline: train_batting processing format=%s", fmt)
            bat = fetch_batting_from_api(go_app_url, fmt, cutoff, api_key)
            headers = bat.get("headers") or []
            rows = bat.get("rows") or []
            if not headers or not rows:
                logger.warning("train_batting.skip_format_no_data_from_api format=%s", fmt)
                return 0
            try:
                X, Y, feature_names_used = load_dataset_from_memory(headers, rows)
            except Exception as e:
                logger.error("train_batting.load_from_api_failed format=%s error=%s", fmt, e)
                return 0
            if X.size == 0 or Y.size == 0:
                logger.warning("train_batting.skip_format_no_data format=%s", fmt)
                return 0
            training_params = get_training_params("batting", fmt)
            transform_config = get_transform_config("batting")
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
            train_and_save(X, Y, args.out, training_params, fmt, meta, transform_config)
            logger.info("train_batting.saved_format format=%s out_dir=%s rows=%s", fmt, args.out, int(X.shape[0]))
            return 1

        max_workers = min(
            len(targets),
            max(1, int(os.environ.get("ML_TRAIN_FORMAT_WORKERS", "4"))),
        )
        saved_count = 0
        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            futures = {executor.submit(_train_one_api, fmt): fmt for fmt in targets}
            for fut in as_completed(futures):
                try:
                    saved_count += fut.result()
                except Exception:
                    raise
        if targets and saved_count == 0:
            logger.error("train_batting.no_models_saved from_api=True cutoff=%s", cutoff)
            raise SystemExit(1)
        return

    # Auto-detect formats when none explicitly provided
    if not targets and not args.csv:
        # 1) Prefer formats from config that actually exist on disk
        cfg_fmts = _config_formats()
        existing_cfg_fmts = [
            f for f in cfg_fmts if os.path.exists(os.path.join(default_csv_dir, f"batting_encoded_{f}.csv"))
        ]
        if existing_cfg_fmts:
            targets = existing_cfg_fmts
        else:
            # 2) Otherwise, glob for batting_encoded_*.csv in the export directory
            try:
                for name in os.listdir(default_csv_dir):
                    if name.startswith("batting_encoded_") and name.endswith(".csv"):
                        suffix = name[len("batting_encoded_") : -len(".csv")]
                        if suffix:
                            targets.append(str(suffix).upper())
            except Exception:
                pass

    # If still no targets detected, fall back to legacy single CSV path
    if not targets:
        training_params = get_training_params("batting", None)
        csv_path = args.csv or os.path.join(default_csv_dir, "batting_encoded.csv")
        try:
            X, Y, feature_names_used = load_dataset(csv_path)
        except FileNotFoundError as e:
            logger.error("train_batting.legacy_csv_not_found path=%s error=%s", csv_path, e)
            raise SystemExit(1) from e
        if X.size == 0 or Y.size == 0:
            logger.error("train_batting.no_data path=%s", csv_path)
            return
        transform_config = get_transform_config("batting")
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
        train_and_save(X, Y, args.out, training_params, None, meta, transform_config)
        logger.info("train_batting.saved_legacy out_dir=%s", args.out)
        return

    # Per-format training loop (concurrent where possible)
    logger.info("pipeline: train_batting loading from CSV formats=%s csv_dir=%s", targets, default_csv_dir)

    def _train_one_csv(fmt: str) -> int:
        logger.info("pipeline: train_batting processing format=%s (CSV)", fmt)
        training_params = get_training_params("batting", fmt)
        csv_path = args.csv or os.path.join(default_csv_dir, f"batting_encoded_{fmt}.csv")
        if not os.path.exists(csv_path):
            logger.warning("train_batting.skip_format_csv_not_found format=%s path=%s", fmt, csv_path)
            return 0
        try:
            X, Y, feature_names_used = load_dataset(csv_path)
        except Exception as e:
            logger.error("train_batting.load_dataset_failed format=%s path=%s error=%s", fmt, csv_path, e)
            return 0
        if X.size == 0 or Y.size == 0:
            logger.warning("train_batting.skip_format_no_data format=%s path=%s", fmt, csv_path)
            return 0
        transform_config = get_transform_config("batting")
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
        train_and_save(X, Y, args.out, training_params, fmt, meta, transform_config)
        logger.info("train_batting.saved_format format=%s out_dir=%s rows=%s", fmt, args.out, int(X.shape[0]))
        return 1

    max_workers = min(
        len(targets),
        max(1, int(os.environ.get("ML_TRAIN_FORMAT_WORKERS", "4"))),
    )
    saved_count = 0
    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        futures = {executor.submit(_train_one_csv, fmt): fmt for fmt in targets}
        for fut in as_completed(futures):
            try:
                saved_count += fut.result()
            except Exception:
                raise
    if targets and saved_count == 0:
        logger.error("train_batting.no_models_saved csv_dir=%s out_dir=%s", default_csv_dir, args.out)
        raise SystemExit(1)


if __name__ == "__main__":
    main()
