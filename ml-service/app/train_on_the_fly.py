"""Train batting and bowling models on the fly from go-app training data (no baseline fallback).

Used when format + features are provided but no pre-trained artifacts are loaded.
Fetches GET {GO_APP_URL}/api/backtest/training-data?format=X&cutoff=Y, builds X/Y like
train_batting/train_bowling, trains in memory, returns (scaler_bat, model_bat, scaler_bowl, model_bowl).
Uses persistent filesystem cache keyed by (format, cutoff) when ML_TRAIN_CACHE_DIR is set.
"""

import hashlib
import json
import os
import threading
import urllib.error
import urllib.request
from typing import Any, Callable, Dict, List, Optional, Tuple

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

from app.logging import get_struct_logger
from ml.config import get_training_params

logger = get_struct_logger()

# Align with ml/ml/train_batting.py and train_bowling.py
BATTING_FEATURE_COLS = [
    "batting_consistency",
    "batting_form",
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
]
BATTING_TARGET_COLS = [
    "runs",
    "balls",
    "fours",
    "sixes",
    "batting_position",
]

BOWLING_FEATURE_COLS = [
    "bowling_consistency",
    "bowling_form",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
    "inning",
    "bowling_session",
    "toss",
    "bowling_venue",
    "bowling_opposition",
    "season_id",
]
BOWLING_TARGET_COLS = [
    "runs",
    "balls",
    "wickets",
]


def _get_training_params(model: str) -> dict:
    """Training parameters from config (ml.training.<model>) only; used for train-on-the-fly and cache save."""
    return get_training_params(model)


def _rows_to_xy(
    headers: List[str],
    rows: List[List[str]],
    feature_cols: List[str],
    target_cols: List[str],
    n_y_final: int,
    preprocess: Optional[Callable[[pd.DataFrame], pd.DataFrame]] = None,
    extra_y_column: Optional[Callable[[pd.DataFrame], np.ndarray]] = None,
) -> Tuple[np.ndarray, np.ndarray]:
    """Shared data processing: build X, Y from go-app training rows.

    Used by both batting and bowling to avoid duplicating feature cleaning,
    dropna, and numeric coercion. Aligns with offline train_batting/train_bowling
    semantics where applicable.
    """
    if not headers or not rows:
        return np.zeros((0, len(feature_cols))), np.zeros((0, n_y_final))
    df = pd.DataFrame(rows, columns=headers)
    all_cols = feature_cols + target_cols
    for c in all_cols:
        if c not in df.columns and c.replace("_", " ") in df.columns:
            df = df.rename(columns={c.replace("_", " "): c})
    if "toss" in df.columns and df["toss"].dtype == object:

        def _normalize_toss(x):
            s = str(x).strip().lower()
            if s in ("0", "1"):
                return int(s)
            if s.startswith("bat"):
                return 1
            return 0

        df = df.assign(toss=df["toss"].apply(_normalize_toss))
    if preprocess is not None:
        df = preprocess(df)
    df = df.dropna(subset=[c for c in feature_cols if c in df.columns])
    if df.empty:
        return np.zeros((0, len(feature_cols))), np.zeros((0, n_y_final))
    for c in feature_cols:
        if c in df.columns:
            df = df.assign(**{c: pd.to_numeric(df[c], errors="coerce")})
    df = df.dropna(subset=feature_cols)
    if df.empty:
        return np.zeros((0, len(feature_cols))), np.zeros((0, n_y_final))
    X = df[feature_cols].astype(float).values
    y_cols = [c for c in target_cols if c in df.columns]
    Y = df[y_cols].astype(float).values
    if Y.shape[1] < len(target_cols):
        pad = np.zeros((Y.shape[0], len(target_cols) - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    if extra_y_column is not None:
        extra = extra_y_column(df)
        Y = np.concatenate([Y, extra], axis=1)
    return X, Y


def _batting_rows_to_xy(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray]:
    """Build X, Y from batting headers + rows (same logic as train_batting.load_dataset)."""

    def _batting_extra_y(df: pd.DataFrame) -> np.ndarray:
        runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
        balls = df.get("balls", pd.Series(np.ones(len(df)))).astype(float).values
        return np.where(balls > 0, (runs / balls) * 100.0, 0.0).reshape(-1, 1)

    return _rows_to_xy(
        headers,
        rows,
        BATTING_FEATURE_COLS,
        BATTING_TARGET_COLS,
        n_y_final=6,
        extra_y_column=_batting_extra_y,
    )


def _bowling_rows_to_xy(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray]:
    """Build X, Y from bowling headers + rows (same logic as train_bowling.load_dataset)."""

    def _bowling_preprocess(df: pd.DataFrame) -> pd.DataFrame:
        if "bowling_session" in df.columns:
            df = df.assign(bowling_session=pd.to_numeric(df["bowling_session"], errors="coerce").fillna(0))
        return df

    def _bowling_extra_y(df: pd.DataFrame) -> np.ndarray:
        runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
        balls = df.get("balls", pd.Series(np.ones(len(df)) * 6)).astype(float).values
        overs = np.where(balls > 0, balls / 6.0, 1.0)
        return np.where(overs > 0, runs / overs, 0.0).reshape(-1, 1)

    return _rows_to_xy(
        headers,
        rows,
        BOWLING_FEATURE_COLS,
        BOWLING_TARGET_COLS,
        n_y_final=4,
        preprocess=_bowling_preprocess,
        extra_y_column=_bowling_extra_y,
    )


def _train_batting_in_memory(X: np.ndarray, Y: np.ndarray) -> Tuple[StandardScaler, Any]:
    params = _get_training_params("batting")
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    model = MultiOutputRegressor(
        RandomForestRegressor(
            n_estimators=params["n_estimators"],
            random_state=params["random_state"],
            max_depth=params["max_depth"],
        )
    )
    model.fit(Xs, Y)
    return scaler, model


def _train_bowling_in_memory(X: np.ndarray, Y: np.ndarray) -> Tuple[StandardScaler, Any]:
    params = _get_training_params("bowling")
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    model = MultiOutputRegressor(
        RandomForestRegressor(
            n_estimators=params["n_estimators"],
            random_state=params["random_state"],
            max_depth=params["max_depth"],
        )
    )
    model.fit(Xs, Y)
    return scaler, model


def fetch_training_data(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    api_key: Optional[str] = None,
) -> Dict[str, Any]:
    """Fetch training data from go-app. Returns dict with batting/bowling headers and rows."""
    base = go_app_url.rstrip("/")
    # format=all requests all matches before cutoff (no format filter); required for cross-format features.
    url = f"{base}/api/backtest/training-data?format=all&cutoff={cutoff_iso}"
    logger.info(
        "train_on_the_fly.fetch.start",
        url=url,
        format_code=format_code,
        cutoff_iso=cutoff_iso,
        has_api_key=api_key is not None,
    )
    timeout_sec = 600
    env_timeout = os.environ.get("TRAINING_DATA_FETCH_TIMEOUT")
    if env_timeout is not None:
        try:
            timeout_sec = int(env_timeout)
        except ValueError:
            pass
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=timeout_sec) as resp:
            body = resp.read().decode()
    except urllib.error.HTTPError as e:
        body = e.read().decode() if e.fp else ""
        err_msg = "Go-app training-data request failed: HTTP %s %s" % (e.code, body or e.reason)
        logger.error(
            "train_on_the_fly.fetch.http_error",
            url=url,
            status_code=e.code,
            body_preview=(body[:500] + "..." if len(body) > 500 else body),
            error=err_msg,
        )
        raise ValueError(err_msg) from e
    except OSError as e:
        logger.error(
            "train_on_the_fly.fetch.os_error",
            url=url,
            error=str(e),
        )
        raise ValueError("Go-app training-data request failed: %s" % e) from e
    data = json.loads(body)
    logger.info(
        "train_on_the_fly.fetch.success",
        url=url,
        batting_rows=len((data.get("batting") or {}).get("rows") or []),
        bowling_rows=len((data.get("bowling") or {}).get("rows") or []),
    )
    return data


def train_on_the_fly(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    api_key: Optional[str] = None,
) -> Tuple[
    Tuple[StandardScaler, Any],
    Tuple[StandardScaler, Any],
]:
    """
    Fetch training data from go-app, train batting and bowling models in memory, return (bat_pair, bowl_pair).
    Raises on fetch failure or when there is insufficient data to train.
    """
    data = fetch_training_data(go_app_url, format_code, cutoff_iso, api_key)
    bat = data.get("batting") or {}
    bowl = data.get("bowling") or {}
    bat_headers = bat.get("headers") or []
    bat_rows = bat.get("rows") or []
    bowl_headers = bowl.get("headers") or []
    bowl_rows = bowl.get("rows") or []

    X_bat, Y_bat = _batting_rows_to_xy(bat_headers, bat_rows)
    X_bowl, Y_bowl = _bowling_rows_to_xy(bowl_headers, bowl_rows)

    n_bat = X_bat.shape[0] if X_bat.size else 0
    n_bowl = X_bowl.shape[0] if X_bowl.size else 0
    logger.info(
        "train_on_the_fly.rows_parsed",
        format_code=format_code,
        cutoff_iso=cutoff_iso,
        batting_samples=n_bat,
        bowling_samples=n_bowl,
    )

    if X_bat.size == 0 or Y_bat.size == 0:
        msg = "Insufficient batting training data for format=%s cutoff=%s (no rows after filtering)" % (
            format_code,
            cutoff_iso,
        )
        logger.warning(
            "train_on_the_fly.insufficient_batting",
            format_code=format_code,
            cutoff_iso=cutoff_iso,
            batting_headers_len=len(bat_headers),
            batting_rows_len=len(bat_rows),
        )
        raise ValueError(msg)
    if X_bowl.size == 0 or Y_bowl.size == 0:
        msg = "Insufficient bowling training data for format=%s cutoff=%s (no rows after filtering)" % (
            format_code,
            cutoff_iso,
        )
        logger.warning(
            "train_on_the_fly.insufficient_bowling",
            format_code=format_code,
            cutoff_iso=cutoff_iso,
            bowling_headers_len=len(bowl_headers),
            bowling_rows_len=len(bowl_rows),
        )
        raise ValueError(msg)

    logger.info(
        "train_on_the_fly.training.start",
        format_code=format_code,
        cutoff_iso=cutoff_iso,
    )
    scaler_bat, model_bat = _train_batting_in_memory(X_bat, Y_bat)
    scaler_bowl, model_bowl = _train_bowling_in_memory(X_bowl, Y_bowl)
    logger.info(
        "train_on_the_fly.training.done",
        format_code=format_code,
        cutoff_iso=cutoff_iso,
    )
    return (scaler_bat, model_bat), (scaler_bowl, model_bowl)


# Persistent cache for train-on-the-fly models (keyed by format + cutoff)
_train_cache_lock = threading.Lock()
_train_cache: Dict[Tuple[str, str], Tuple[Tuple[StandardScaler, Any], Tuple[StandardScaler, Any]]] = {}


def _cache_dir() -> Optional[str]:
    d = (os.environ.get("ML_TRAIN_CACHE_DIR") or "").strip()
    if d:
        return os.path.expanduser(d)
    return None


def _cache_key(format_code: str, cutoff_iso: str) -> str:
    """Filesystem-safe cache key for (format, cutoff)."""
    raw = f"{format_code}_{cutoff_iso}"
    return hashlib.sha256(raw.encode()).hexdigest()[:32]


def _load_from_cache(
    cache_dir: str, key: str
) -> Optional[Tuple[Tuple[StandardScaler, Any], Tuple[StandardScaler, Any]]]:
    subdir = os.path.join(cache_dir, key)
    bat_scaler_p = os.path.join(subdir, "bat_scaler.joblib")
    bat_model_p = os.path.join(subdir, "bat_model.joblib")
    bowl_scaler_p = os.path.join(subdir, "bowl_scaler.joblib")
    bowl_model_p = os.path.join(subdir, "bowl_model.joblib")
    for p in (bat_scaler_p, bat_model_p, bowl_scaler_p, bowl_model_p):
        if not os.path.isfile(p):
            return None
    try:
        scaler_bat = joblib.load(bat_scaler_p)
        model_bat = joblib.load(bat_model_p)
        scaler_bowl = joblib.load(bowl_scaler_p)
        model_bowl = joblib.load(bowl_model_p)
        return (scaler_bat, model_bat), (scaler_bowl, model_bowl)
    except Exception as e:
        logger.warning("train_on_the_fly.cache_load_failed", key=key, error=str(e))
        return None


def _save_to_cache(
    cache_dir: str,
    key: str,
    bat_pair: Tuple[StandardScaler, Any],
    bowl_pair: Tuple[StandardScaler, Any],
) -> None:
    subdir = os.path.join(cache_dir, key)
    try:
        os.makedirs(subdir, mode=0o750, exist_ok=True)
        scaler_bat, model_bat = bat_pair
        scaler_bowl, model_bowl = bowl_pair
        bat_params = _get_training_params("batting")
        bowl_params = _get_training_params("bowling")
        joblib.dump(
            scaler_bat, os.path.join(subdir, "bat_scaler.joblib"), compress=bat_params["joblib_compress"]
        )
        joblib.dump(
            model_bat, os.path.join(subdir, "bat_model.joblib"), compress=bat_params["joblib_compress"]
        )
        joblib.dump(
            scaler_bowl, os.path.join(subdir, "bowl_scaler.joblib"), compress=bowl_params["joblib_compress"]
        )
        joblib.dump(
            model_bowl, os.path.join(subdir, "bowl_model.joblib"), compress=bowl_params["joblib_compress"]
        )
        logger.info("train_on_the_fly.cache_saved", key=key, subdir=subdir)
    except Exception as e:
        logger.warning("train_on_the_fly.cache_save_failed", key=key, error=str(e))


def train_on_the_fly_cached(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    api_key: Optional[str] = None,
) -> Tuple[
    Tuple[StandardScaler, Any],
    Tuple[StandardScaler, Any],
]:
    """
    train_on_the_fly with persistent cache (filesystem when ML_TRAIN_CACHE_DIR is set).
    Thread-safe: in-memory cache first, then disk, then train.
    """
    cache_dir = _cache_dir()
    key = _cache_key(format_code, cutoff_iso)
    with _train_cache_lock:
        in_mem = _train_cache.get((format_code, cutoff_iso))
        if in_mem is not None:
            logger.info("train_on_the_fly.cache_hit", source="memory", format_code=format_code, cutoff_iso=cutoff_iso)
            return in_mem
    if cache_dir:
        disk_pair = _load_from_cache(cache_dir, key)
        if disk_pair is not None:
            with _train_cache_lock:
                _train_cache[(format_code, cutoff_iso)] = disk_pair
            logger.info("train_on_the_fly.cache_hit", source="disk", format_code=format_code, cutoff_iso=cutoff_iso)
            return disk_pair
    pair = train_on_the_fly(go_app_url, format_code, cutoff_iso, api_key)
    with _train_cache_lock:
        _train_cache[(format_code, cutoff_iso)] = pair
    if cache_dir:
        _save_to_cache(cache_dir, key, pair[0], pair[1])
    return pair
