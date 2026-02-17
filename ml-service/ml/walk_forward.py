"""
Walk-forward training and evaluation for sequential cricket data.

Trains on data before a cutoff, predicts the next X matches (holdout), evaluates (e.g. MAE),
then absorbs that window into the training set and repeats. Records each model's parameters
and metrics in a registry for feedback loops and optional integration with auto_tune.

Usage:
  GO_APP_URL=http://localhost:8080 python -m ml.walk_forward \\
    --initial-cutoff 2020-01-01T00:00:00Z --window-x 50 --format T20 --model batting
  python -m ml.walk_forward --initial-cutoff 2020-01-01 --window-x 50 --format T20 --model all --auto-tune-initial
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional, Tuple

import numpy as np

_ML_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _ML_ROOT not in sys.path:
    sys.path.insert(0, _ML_ROOT)

from ml.config import default_artifacts_dir, get_training_params

logger = logging.getLogger(__name__)


def _fetch_json(
    base_url: str,
    path: str,
    api_key: Optional[str] = None,
    timeout: int = 600,
) -> Dict[str, Any]:
    url = base_url.rstrip("/") + path
    req = urllib.request.Request(url)
    if api_key:
        req.add_header("X-API-Key", api_key)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode())


def fetch_matches_after(
    go_app_url: str,
    after_iso: str,
    format_code: str,
    limit: int,
    api_key: Optional[str] = None,
) -> List[Dict[str, Any]]:
    """List match_id and match_date for matches strictly after cutoff (for walk-forward windows)."""
    path = f"/api/backtest/matches?after={after_iso}&format={format_code}&limit={limit}"
    data = _fetch_json(go_app_url, path, api_key)
    return data.get("matches") or []


def fetch_training_data(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    api_key: Optional[str] = None,
) -> Dict[str, Any]:
    """Fetch training data (match_date < cutoff). Use format= to get per-format rows."""
    path = f"/api/backtest/training-data?format={format_code}&cutoff={cutoff_iso}"
    return _fetch_json(go_app_url, path, api_key)


def fetch_holdout_data(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    limit: int,
    api_key: Optional[str] = None,
) -> Dict[str, Any]:
    """Fetch holdout data (matches after cutoff, features at cutoff). Same shape as training-data."""
    path = f"/api/backtest/holdout-data?format={format_code}&cutoff={cutoff_iso}&limit={limit}"
    return _fetch_json(go_app_url, path, api_key)


def _rows_to_xy_batting(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray]:
    from app.train_on_the_fly import _batting_rows_to_xy

    return _batting_rows_to_xy(headers, rows)


def _rows_to_xy_bowling(headers: List[str], rows: List[List[str]]) -> Tuple[np.ndarray, np.ndarray]:
    from app.train_on_the_fly import _bowling_rows_to_xy

    return _bowling_rows_to_xy(headers, rows)


def _train_batting_from_xy(X: np.ndarray, Y: np.ndarray) -> Tuple[Any, Any]:
    from app.train_on_the_fly import _train_batting_in_memory

    return _train_batting_in_memory(X, Y)


def _train_bowling_from_xy(X: np.ndarray, Y: np.ndarray) -> Tuple[Any, Any]:
    from app.train_on_the_fly import _train_bowling_in_memory

    return _train_bowling_in_memory(X, Y)


def _mae(y_true: np.ndarray, y_pred: np.ndarray) -> float:
    if y_true.size == 0:
        return float("nan")
    return float(np.abs(y_true - y_pred).mean())


def _compute_metrics_batting(y_true: np.ndarray, y_pred: np.ndarray) -> Dict[str, float]:
    """y_true/y_pred: (n, 6) runs, balls, fours, sixes, batting_position, strike_rate."""
    metrics = {}
    names = ["runs", "balls", "fours", "sixes", "batting_position", "strike_rate"]
    for i, name in enumerate(names):
        if i < y_true.shape[1] and i < y_pred.shape[1]:
            metrics[f"mae_{name}"] = _mae(y_true[:, i], y_pred[:, i])
    metrics["mae_overall"] = float(np.abs(y_true - y_pred).mean())
    return metrics


def _compute_metrics_bowling(y_true: np.ndarray, y_pred: np.ndarray) -> Dict[str, float]:
    """y_true/y_pred: (n, 4) runs, balls, wickets, economy."""
    metrics = {}
    names = ["runs", "balls", "wickets", "economy"]
    for i, name in enumerate(names):
        if i < y_true.shape[1] and i < y_pred.shape[1]:
            metrics[f"mae_{name}"] = _mae(y_true[:, i], y_pred[:, i])
    metrics["mae_overall"] = float(np.abs(y_true - y_pred).mean())
    return metrics


def run_walk_forward_window_batting(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    window_x: int,
    api_key: Optional[str],
    save_artifacts_dir: Optional[str],
    run_id: str,
    window_index: int,
) -> Tuple[Dict[str, Any], Optional[str]]:
    """Train batting on data before cutoff, predict holdout, return (registry_entry, next_cutoff_iso)."""
    train_data = fetch_training_data(go_app_url, format_code, cutoff_iso, api_key)
    bat = train_data.get("batting") or {}
    train_headers = bat.get("headers") or []
    train_rows = bat.get("rows") or []
    X_train, Y_train = _rows_to_xy_batting(train_headers, train_rows)
    if X_train.shape[0] == 0:
        return {"error": "no_training_data", "cutoff": cutoff_iso}, None

    scaler, model = _train_batting_from_xy(X_train, Y_train)
    params = get_training_params("batting")

    holdout = fetch_holdout_data(go_app_url, format_code, cutoff_iso, window_x, api_key)
    bat_h = holdout.get("batting") or {}
    h_headers = bat_h.get("headers") or []
    h_rows = bat_h.get("rows") or []
    if not h_rows:
        return {"error": "no_holdout_data", "cutoff": cutoff_iso}, None

    X_hold, Y_hold = _rows_to_xy_batting(h_headers, h_rows)
    if X_hold.shape[0] == 0:
        return {"error": "no_holdout_rows_after_parse", "cutoff": cutoff_iso}, None

    X_hold_scaled = scaler.transform(X_hold)
    Y_pred = model.predict(X_hold_scaled)
    if hasattr(Y_pred, "reshape") and Y_pred.ndim == 1:
        Y_pred = Y_pred.reshape(-1, 1)
    metrics = _compute_metrics_batting(Y_hold, Y_pred)

    matches = fetch_matches_after(go_app_url, cutoff_iso, format_code, window_x, api_key)
    window_start = cutoff_iso
    window_end = matches[-1]["match_date"] if matches else cutoff_iso
    try:
        last_dt = datetime.fromisoformat(window_end.replace("Z", "+00:00"))
        next_dt = last_dt + timedelta(days=1)
        next_cutoff_iso = next_dt.strftime("%Y-%m-%dT%H:%M:%SZ")
    except Exception:
        next_cutoff_iso = None

    entry: Dict[str, Any] = {
        "run_id": run_id,
        "model_type": "batting",
        "format": format_code,
        "cutoff_trained_before": cutoff_iso,
        "window_x": window_x,
        "window_start_date": window_start,
        "window_end_date": window_end,
        "training_params": {k: v for k, v in params.items() if k in ("n_estimators", "max_depth", "random_state")},
        "auto_tune_used": False,
        "metrics": metrics,
        "n_training_samples": int(X_train.shape[0]),
        "n_holdout_samples": int(X_hold.shape[0]),
        "window_index": window_index,
        "created_at": datetime.now(timezone.utc).isoformat(),
    }
    if save_artifacts_dir:
        import joblib

        os.makedirs(save_artifacts_dir, exist_ok=True)
        safe_cutoff = cutoff_iso.replace(":", "-").replace(".", "-")
        scaler_path = os.path.join(save_artifacts_dir, f"batting_scaler_wf_{format_code}_{safe_cutoff}.joblib")
        model_path = os.path.join(save_artifacts_dir, f"batting_model_wf_{format_code}_{safe_cutoff}.joblib")
        joblib.dump(scaler, scaler_path, compress=params.get("joblib_compress", 3))
        joblib.dump(model, model_path, compress=params.get("joblib_compress", 3))
        entry["artifact_paths"] = {"scaler": scaler_path, "model": model_path}
    else:
        entry["artifact_paths"] = None

    return entry, next_cutoff_iso


def run_walk_forward_window_bowling(
    go_app_url: str,
    format_code: str,
    cutoff_iso: str,
    window_x: int,
    api_key: Optional[str],
    save_artifacts_dir: Optional[str],
    run_id: str,
    window_index: int,
) -> Tuple[Dict[str, Any], Optional[str]]:
    """Train bowling on data before cutoff, predict holdout, return (registry_entry, next_cutoff_iso)."""
    train_data = fetch_training_data(go_app_url, format_code, cutoff_iso, api_key)
    bowl = train_data.get("bowling") or {}
    train_headers = bowl.get("headers") or []
    train_rows = bowl.get("rows") or []
    X_train, Y_train = _rows_to_xy_bowling(train_headers, train_rows)
    if X_train.shape[0] == 0:
        return {"error": "no_training_data", "cutoff": cutoff_iso}, None

    scaler, model = _train_bowling_from_xy(X_train, Y_train)
    params = get_training_params("bowling")

    holdout = fetch_holdout_data(go_app_url, format_code, cutoff_iso, window_x, api_key)
    bowl_h = holdout.get("bowling") or {}
    h_headers = bowl_h.get("headers") or []
    h_rows = bowl_h.get("rows") or []
    if not h_rows:
        return {"error": "no_holdout_data", "cutoff": cutoff_iso}, None

    X_hold, Y_hold = _rows_to_xy_bowling(h_headers, h_rows)
    if X_hold.shape[0] == 0:
        return {"error": "no_holdout_rows_after_parse", "cutoff": cutoff_iso}, None

    X_hold_scaled = scaler.transform(X_hold)
    Y_pred = model.predict(X_hold_scaled)
    if hasattr(Y_pred, "reshape") and Y_pred.ndim == 1:
        Y_pred = Y_pred.reshape(-1, 1)
    metrics = _compute_metrics_bowling(Y_hold, Y_pred)

    matches = fetch_matches_after(go_app_url, cutoff_iso, format_code, window_x, api_key)
    window_end = matches[-1]["match_date"] if matches else cutoff_iso
    try:
        last_dt = datetime.fromisoformat(window_end.replace("Z", "+00:00"))
        next_dt = last_dt + timedelta(days=1)
        next_cutoff_iso = next_dt.strftime("%Y-%m-%dT%H:%M:%SZ")
    except Exception:
        next_cutoff_iso = None

    entry = {
        "run_id": run_id,
        "model_type": "bowling",
        "format": format_code,
        "cutoff_trained_before": cutoff_iso,
        "window_x": window_x,
        "window_start_date": cutoff_iso,
        "window_end_date": window_end,
        "training_params": {k: v for k, v in params.items() if k in ("n_estimators", "max_depth", "random_state")},
        "auto_tune_used": False,
        "metrics": metrics,
        "n_training_samples": int(X_train.shape[0]),
        "n_holdout_samples": int(X_hold.shape[0]),
        "window_index": window_index,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "artifact_paths": None,
    }
    if save_artifacts_dir:
        import joblib

        os.makedirs(save_artifacts_dir, exist_ok=True)
        safe_cutoff = cutoff_iso.replace(":", "-").replace(".", "-")
        scaler_path = os.path.join(save_artifacts_dir, f"bowling_scaler_wf_{format_code}_{safe_cutoff}.joblib")
        model_path = os.path.join(save_artifacts_dir, f"bowling_model_wf_{format_code}_{safe_cutoff}.joblib")
        joblib.dump(scaler, scaler_path, compress=params.get("joblib_compress", 3))
        joblib.dump(model, model_path, compress=params.get("joblib_compress", 3))
        entry["artifact_paths"] = {"scaler": scaler_path, "model": model_path}

    return entry, next_cutoff_iso


def run_walk_forward(
    go_app_url: str,
    initial_cutoff_iso: str,
    window_x: int,
    format_code: str,
    model_type: str,
    api_key: Optional[str] = None,
    registry_path: Optional[str] = None,
    save_artifacts: bool = True,
    auto_tune_initial: bool = False,
    max_windows: Optional[int] = None,
) -> List[Dict[str, Any]]:
    """Run walk-forward loop; return list of registry entries (one per window)."""
    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    out_dir = default_artifacts_dir()
    save_dir = os.path.join(out_dir, "walk_forward", run_id) if save_artifacts else None
    registry: List[Dict[str, Any]] = []
    current_cutoff = initial_cutoff_iso
    window_index = 0
    models = ["batting", "bowling"] if model_type == "all" else [model_type]

    next_cutoff: Optional[str] = None
    while True:
        if max_windows is not None and window_index >= max_windows:
            break
        matches = fetch_matches_after(go_app_url, current_cutoff, format_code, window_x, api_key)
        if not matches:
            logger.info("walk_forward.no_more_matches cutoff=%s", current_cutoff)
            break

        for m in models:
            if m == "batting":
                entry, next_cutoff = run_walk_forward_window_batting(
                    go_app_url, format_code, current_cutoff, window_x, api_key, save_dir, run_id, window_index
                )
            elif m == "bowling":
                entry, next_cutoff = run_walk_forward_window_bowling(
                    go_app_url, format_code, current_cutoff, window_x, api_key, save_dir, run_id, window_index
                )
            else:
                continue
            registry.append(entry)
            if entry.get("error"):
                logger.warning("walk_forward.window_error %s", entry)
            else:
                logger.info(
                    "walk_forward.window model=%s window=%s mae_overall=%.4f",
                    m,
                    window_index,
                    entry.get("metrics", {}).get("mae_overall", float("nan")),
                )
        if next_cutoff is None:
            break
        current_cutoff = next_cutoff
        window_index += 1
        if registry_path:
            with open(registry_path, "w", encoding="utf-8") as f:
                json.dump(
                    {
                        "run_id": run_id,
                        "windows": registry,
                        "config": {"initial_cutoff": initial_cutoff_iso, "window_x": window_x, "format": format_code},
                    },
                    f,
                    indent=2,
                )
    if registry_path:
        with open(registry_path, "w", encoding="utf-8") as f:
            json.dump(
                {
                    "run_id": run_id,
                    "windows": registry,
                    "config": {"initial_cutoff": initial_cutoff_iso, "window_x": window_x, "format": format_code},
                },
                f,
                indent=2,
            )
    return registry


def main() -> None:
    parser = argparse.ArgumentParser(description="Walk-forward training and evaluation (sequential data)")
    parser.add_argument(
        "--initial-cutoff", required=True, help="RFC3339 or YYYY-MM-DD; train base model on data before this"
    )
    parser.add_argument("--window-x", type=int, default=50, help="Number of matches per holdout window")
    parser.add_argument("--format", default="T20", help="Format code (T20, ODI, etc.)")
    parser.add_argument("--model", choices=["batting", "bowling", "all"], default="batting")
    parser.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", "http://localhost:8080"))
    parser.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY"))
    parser.add_argument("--registry", default="", help="Path to write walk_forward_registry.json")
    parser.add_argument("--save-artifacts", action="store_true", default=True)
    parser.add_argument("--no-save-artifacts", action="store_false", dest="save_artifacts")
    parser.add_argument("--auto-tune-initial", action="store_true", help="Run auto_tune on first window (optional)")
    parser.add_argument("--max-windows", type=int, default=None, help="Cap number of windows (for testing)")
    args = parser.parse_args()

    cutoff = args.initial_cutoff
    if len(cutoff) == 10 and "T" not in cutoff:
        cutoff = cutoff + "T00:00:00Z"

    registry_path = args.registry or os.path.join(default_artifacts_dir(), "walk_forward_registry.json")
    os.makedirs(os.path.dirname(registry_path) or ".", exist_ok=True)

    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    registry = run_walk_forward(
        go_app_url=args.go_app_url,
        initial_cutoff_iso=cutoff,
        window_x=args.window_x,
        format_code=args.format,
        model_type=args.model,
        api_key=args.api_key,
        registry_path=registry_path,
        save_artifacts=args.save_artifacts,
        auto_tune_initial=args.auto_tune_initial,
        max_windows=args.max_windows,
    )
    print("Registry written to", registry_path)
    print("Total windows:", len(registry))


if __name__ == "__main__":
    main()
