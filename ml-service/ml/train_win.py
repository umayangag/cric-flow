"""
Train win model (match-level team1_wins 0/1) from go-app training-data API or CSV.

Fetches GET {GO_APP_URL}/api/backtest/training-data?format=all&cutoff=..., extracts win
headers/rows, groups by format_code, trains one classifier per format, saves
win_model_<FMT>.joblib to artifacts dir.

Usage:
  GO_APP_URL=http://localhost:8080 python -m ml.train_win --cutoff 2024-12-01T00:00:00Z
  python -m ml.train_win --csv path/to/win_export.csv
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
from sklearn.ensemble import RandomForestClassifier

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from ml.config import (
    default_artifacts_dir,
    default_go_app_export_dir,
    get_pipeline_common_config,
    get_training_data_fetch_timeout_sec,
    get_training_params,
)
from ml.pipeline_common import compute_time_decay_weights

logger = logging.getLogger(__name__)

# Same feature families as batting/bowling/fielding: format, venue, teams, toss, weather, and
# team-level aggregates of player consistency/form (team1 = batting inn 1, team2 = bowling inn 1).
WIN_FEATURE_COLS = [
    "format_id",
    "venue_id",
    "team1_opposition_id",
    "team2_opposition_id",
    "toss_winner_opposition_id",
    "temp",
    "wind",
    "rain",
    "humidity",
    "cloud",
    "pressure",
    "viscosity",
    "team1_bat_consistency_sum",
    "team1_bowl_consistency_sum",
    "team2_bat_consistency_sum",
    "team2_bowl_consistency_sum",
    "team1_bat_form_sum",
    "team1_bowl_form_sum",
    "team2_bat_form_sum",
    "team2_bowl_form_sum",
]
WIN_TARGET_COL = "team1_wins"


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


def rows_to_xy_by_format(
    headers: list, rows: list[list]
) -> dict[str, tuple[np.ndarray, np.ndarray, Optional[np.ndarray]]]:
    """Build X, Y, weights per format_code. Returns dict format_code -> (X, Y, sample_weight). Y is integer 0/1."""
    if not headers or not rows:
        return {}
    df = pd.DataFrame(rows, columns=headers)
    for c in WIN_FEATURE_COLS + [WIN_TARGET_COL]:
        if c in df.columns:
            df[c] = pd.to_numeric(df[c], errors="coerce")
    for c in WIN_FEATURE_COLS:
        if c in df.columns:
            df[c] = df[c].fillna(0.0)
    pipe_cfg = get_pipeline_common_config()
    halflife = pipe_cfg.get("time_decay_halflife_years", 2.0)

    def _weights(g: pd.DataFrame) -> Optional[np.ndarray]:
        if "match_date" not in g.columns:
            return None
        return compute_time_decay_weights(g["match_date"], halflife_years=halflife)

    if "format_code" not in df.columns:
        df = df.dropna(subset=[c for c in WIN_FEATURE_COLS if c in df.columns] + [WIN_TARGET_COL])
        if df.empty:
            return {}
        X = df[[c for c in WIN_FEATURE_COLS if c in df.columns]].astype(float).values
        Y = df[WIN_TARGET_COL].astype(int).values
        w = _weights(df)
        return {"_ALL_": (X, Y, w)}
    out = {}
    for fmt, g in df.groupby("format_code"):
        fmt = str(fmt).strip().upper() or "_ALL_"
        g = g.dropna(subset=[c for c in WIN_FEATURE_COLS if c in g.columns] + [WIN_TARGET_COL])
        if g.empty or len(g) < 10:
            continue
        X = g[[c for c in WIN_FEATURE_COLS if c in g.columns]].astype(float).values
        Y = g[WIN_TARGET_COL].astype(int).values
        w = _weights(g)
        out[fmt] = (X, Y, w)
    return out


def train_and_save(
    X: np.ndarray, Y: np.ndarray, out_dir: str, format_code: str, sample_weight: Optional[np.ndarray] = None
) -> None:
    """Train win classifier and save model for format_code (no scaler; artifacts loader expects model only).
    Time-decay sample weights when match_date available."""
    params = get_training_params("win", format_code)
    model = RandomForestClassifier(
        n_estimators=params["n_estimators"],
        max_depth=params["max_depth"],
        random_state=params["random_state"],
        n_jobs=params.get("n_jobs", -1),
    )
    if sample_weight is not None:
        model.fit(X, Y, sample_weight=sample_weight)
    else:
        model.fit(X, Y)
    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    code = format_code.replace(" ", "_")
    joblib.dump(model, os.path.join(out_dir, f"win_model_{code}.joblib"), compress=compress)


def train_and_save_legacy(
    X: np.ndarray, Y: np.ndarray, out_dir: str, sample_weight: Optional[np.ndarray] = None
) -> None:
    """Train one unified win model on all data and save as legacy (win_model.joblib).
    Time-decay sample weights when match_date available."""
    params = get_training_params("win", None)
    model = RandomForestClassifier(
        n_estimators=params["n_estimators"],
        max_depth=params["max_depth"],
        random_state=params["random_state"],
        n_jobs=params.get("n_jobs", -1),
    )
    if sample_weight is not None:
        model.fit(X, Y, sample_weight=sample_weight)
    else:
        model.fit(X, Y)
    os.makedirs(out_dir, exist_ok=True)
    compress = params["joblib_compress"]
    joblib.dump(model, os.path.join(out_dir, "win_model.joblib"), compress=compress)
    logger.info("train_win.saved_unified out_dir=%s rows=%s", out_dir, X.shape[0])


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    ap = argparse.ArgumentParser(description="Train win model from go-app export CSV (preferred) or training-data API")
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
    for fmt, (X, Y, w) in by_format.items():
        logger.info("pipeline: train_win processing format=%s n=%s", fmt, X.shape[0])
        train_and_save(X, Y, out_dir, fmt, sample_weight=w)
        logger.info("train_win.saved format=%s n=%s out_dir=%s", fmt, X.shape[0], out_dir)

    # Unified (overall) model: train on all data combined for legacy/fallback
    all_X = np.vstack([X for _, (X, _, _) in by_format.items()])
    all_Y = np.concatenate([Y.ravel() for _, (_, Y, _) in by_format.items()])
    all_weights = _concat_weights_win([w for _, (_, _, w) in by_format.items()])
    if all_X.shape[0] >= 10:
        train_and_save_legacy(all_X, all_Y, out_dir, sample_weight=all_weights)


if __name__ == "__main__":
    main()
