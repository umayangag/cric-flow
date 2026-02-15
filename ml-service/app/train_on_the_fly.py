"""Train batting and bowling models on the fly from go-app training data (no baseline fallback).

Used when format + features are provided but no pre-trained artifacts are loaded.
Fetches GET {GO_APP_URL}/api/backtest/training-data?format=X&cutoff=Y, builds X/Y like
train_batting/train_bowling, trains in memory, returns (scaler_bat, model_bat, scaler_bowl, model_bowl).
"""

import json
import os
import urllib.error
import urllib.request
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestRegressor
from sklearn.multioutput import MultiOutputRegressor
from sklearn.preprocessing import StandardScaler

from app.logging import get_struct_logger

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


def _get_rf_params() -> dict:
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            cfg = json.load(f)
    except Exception:
        cfg = {}
    ml_cfg = cfg.get("ml", {}) if isinstance(cfg, dict) else {}
    return {
        "n_estimators": int(os.environ.get("ML_N_ESTIMATORS") or ml_cfg.get("n_estimators", 100)),
        "max_depth": ml_cfg.get("max_depth"),
        "random_state": int(os.environ.get("ML_RANDOM_STATE") or ml_cfg.get("random_state", 42)),
    }


def _batting_rows_to_xy(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray]:
    """Build X, Y from batting headers + rows (same logic as train_batting.load_dataset)."""
    if not headers or not rows:
        return np.zeros((0, len(BATTING_FEATURE_COLS))), np.zeros((0, 6))
    df = pd.DataFrame(rows, columns=headers)
    # Normalize column names (export may use same names)
    for c in BATTING_FEATURE_COLS + BATTING_TARGET_COLS:
        if c not in df.columns and c.replace("_", " ") in df.columns:
            df = df.rename(columns={c.replace("_", " "): c})
    # Toss: Go batting export already sends 0/1
    if "toss" in df.columns and df["toss"].dtype == object:
        df = df.assign(toss=df["toss"].apply(lambda x: 1 if str(x).strip().lower().startswith("bat") else 0))
    df = df.dropna(subset=[c for c in BATTING_FEATURE_COLS if c in df.columns])
    if df.empty:
        return np.zeros((0, len(BATTING_FEATURE_COLS))), np.zeros((0, 6))
    for c in BATTING_FEATURE_COLS:
        if c in df.columns:
            df = df.assign(**{c: pd.to_numeric(df[c], errors="coerce")})
    df = df.dropna(subset=BATTING_FEATURE_COLS)
    if df.empty:
        return np.zeros((0, len(BATTING_FEATURE_COLS))), np.zeros((0, 6))
    X = df[BATTING_FEATURE_COLS].astype(float).values
    y_cols = [c for c in BATTING_TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if Y.shape[1] < len(BATTING_TARGET_COLS):
        pad = np.zeros((Y.shape[0], len(BATTING_TARGET_COLS) - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
    balls = df.get("balls", pd.Series(np.ones(len(df)))).astype(float).values
    sr = np.where(balls > 0, (runs / balls) * 100.0, 0.0).reshape(-1, 1)
    Y = np.concatenate([Y, sr], axis=1)
    return X, Y


def _bowling_rows_to_xy(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray]:
    """Build X, Y from bowling headers + rows (same logic as train_bowling.load_dataset)."""
    if not headers or not rows:
        return np.zeros((0, len(BOWLING_FEATURE_COLS))), np.zeros((0, 4))
    df = pd.DataFrame(rows, columns=headers)
    for c in BOWLING_FEATURE_COLS + BOWLING_TARGET_COLS:
        if c not in df.columns and c.replace("_", " ") in df.columns:
            df = df.rename(columns={c.replace("_", " "): c})
    # Go export: toss is raw toss_decision; bowling_session can be NULL
    if "toss" in df.columns and df["toss"].dtype == object:
        df = df.assign(toss=df["toss"].apply(lambda x: 1 if str(x).strip().lower().startswith("bat") else 0))
    if "bowling_session" in df.columns:
        df = df.assign(bowling_session=pd.to_numeric(df["bowling_session"], errors="coerce").fillna(0))
    df = df.dropna(subset=[c for c in BOWLING_FEATURE_COLS if c in df.columns])
    if df.empty:
        return np.zeros((0, len(BOWLING_FEATURE_COLS))), np.zeros((0, 4))
    for c in BOWLING_FEATURE_COLS:
        if c in df.columns:
            df = df.assign(**{c: pd.to_numeric(df[c], errors="coerce")})
    df = df.dropna(subset=BOWLING_FEATURE_COLS)
    if df.empty:
        return np.zeros((0, len(BOWLING_FEATURE_COLS))), np.zeros((0, 4))
    X = df[BOWLING_FEATURE_COLS].astype(float).values
    y_cols = [c for c in BOWLING_TARGET_COLS if c in df.columns]
    Y = df[y_cols].astype(float).values
    if Y.shape[1] < len(BOWLING_TARGET_COLS):
        pad = np.zeros((Y.shape[0], len(BOWLING_TARGET_COLS) - Y.shape[1]))
        Y = np.concatenate([Y, pad], axis=1)
    runs = df.get("runs", pd.Series(np.zeros(len(df)))).astype(float).values
    balls = df.get("balls", pd.Series(np.ones(len(df)) * 6)).astype(float).values
    overs = np.where(balls > 0, balls / 6.0, 1.0)
    econ = np.where(overs > 0, runs / overs, 0.0).reshape(-1, 1)
    Y = np.concatenate([Y, econ], axis=1)
    return X, Y


def _train_batting_in_memory(X: np.ndarray, Y: np.ndarray) -> Tuple[StandardScaler, Any]:
    rf_params = _get_rf_params()
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    n_estimators = int(rf_params.get("n_estimators", 100))
    random_state = int(rf_params.get("random_state", 42))
    max_depth = rf_params.get("max_depth")
    if max_depth is not None:
        try:
            max_depth = int(max_depth)
        except Exception:
            max_depth = None
    model = MultiOutputRegressor(
        RandomForestRegressor(n_estimators=n_estimators, random_state=random_state, max_depth=max_depth)
    )
    model.fit(Xs, Y)
    return scaler, model


def _train_bowling_in_memory(X: np.ndarray, Y: np.ndarray) -> Tuple[StandardScaler, Any]:
    rf_params = _get_rf_params()
    scaler = StandardScaler()
    Xs = scaler.fit_transform(X)
    n_estimators = int(rf_params.get("n_estimators", 100))
    random_state = int(rf_params.get("random_state", 42))
    max_depth = rf_params.get("max_depth")
    if max_depth is not None:
        try:
            max_depth = int(max_depth)
        except Exception:
            max_depth = None
    model = MultiOutputRegressor(
        RandomForestRegressor(n_estimators=n_estimators, random_state=random_state, max_depth=max_depth)
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
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
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
