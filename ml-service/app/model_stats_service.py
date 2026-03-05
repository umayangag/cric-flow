"""Model statistics scanning and reporting for the ML service.

Extracted from app.main. Provides:
- Model artifact filename parsing
- Per-model stats (size, modified date, tuning report enrichment)
- Unified model stats builder for /model-stats endpoint
"""

import json
import os
import re
import time
from typing import Any, Dict, List, Optional, Tuple

from .logging import get_struct_logger

logger = get_struct_logger()

# Algorithm key -> display name for model-stats UI
ALGORITHM_NAMES: Dict[str, str] = {
    "rf": "Random Forest",
    "gb": "Gradient Boosting",
    "quantile": "Quantile Regressor",
    "stacked": "Stacking Regressor",
    "ridge": "Ridge",
}

# Matches any <kind>_model[_<FORMAT>].joblib filename.
# Group 1 = kind (e.g. "batting", "batting_share"), group 2 = optional format code.
_MODEL_FILENAME_RE = re.compile(r"^(.+)_model(?:_([^.]+))?\.joblib$", re.IGNORECASE)


def parse_model_filename(fname: str) -> Optional[Tuple[str, Optional[str]]]:
    """Parse model artifact filename dynamically. Return (kind, format) or None.

    Recognises any ``<kind>_model[_<FORMAT>].joblib`` file so newly-added model
    kinds (e.g. innings, batting_share) are picked up automatically.
    """
    m = _MODEL_FILENAME_RE.match(fname)
    if not m:
        return None
    kind = m.group(1).lower()
    fmt = m.group(2).upper() if m.group(2) else None
    return (kind, fmt)


def _kind_display_name(kind: str) -> str:
    """Human-readable display name: 'batting_share' → 'Batting Share'."""
    return kind.replace("_", " ").title()


def get_model_artifact_stats(
    models_dir: str,
    entries: List[str],
    kind: str,
    fmt: Optional[str],
    fname: str,
) -> Optional[Dict[str, Any]]:
    """Process a single model artifact file to get basic stats (size, modified date).

    Returns None if the file is not accessible; otherwise a dict with model_kind, model_name,
    match_format, size_bytes, modified.
    """
    model_path = os.path.join(models_dir, fname)
    try:
        st = os.stat(model_path)
        if not os.path.isfile(model_path):
            return None
    except OSError:
        return None

    size_bytes = st.st_size
    modified_ts = int(st.st_mtime)
    modified_iso = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(modified_ts))

    # Include scaler size if a companion scaler artifact exists
    scaler_suffix = f"_{fmt}" if fmt else ""
    scaler_name = f"{kind}_scaler{scaler_suffix}.joblib"
    scaler_path = os.path.join(models_dir, scaler_name)
    if scaler_name in entries and os.path.isfile(scaler_path):
        try:
            size_bytes += os.path.getsize(scaler_path)
        except OSError:
            pass

    return {
        "model_kind": kind,
        "model_name": _kind_display_name(kind),
        "match_format": fmt or "Unified",
        "size_bytes": size_bytes,
        "modified": modified_iso,
    }


def flatten_metrics_for_display(metrics: Dict[str, Any]) -> Dict[str, Any]:
    """Flatten nested metric dicts so the UI can display key=value chips.

    Expands per_target_mae (e.g. mae_runs, mae_balls, mae_wickets) to top-level.
    Skips nested dicts that would render as [object Object].
    Joins lists into comma-separated strings.
    """
    out: Dict[str, Any] = {}
    for k, v in metrics.items():
        if k == "per_target_mae" and isinstance(v, dict):
            for sk, sv in v.items():
                out[sk] = sv
            continue
        if isinstance(v, list):
            out[k] = ", ".join(map(str, v))
            continue
        if isinstance(v, dict):
            continue
        out[k] = v
    return out


def enrich_with_tuning_report(
    rec: Dict[str, Any],
    models_dir: str,
    entries: List[str],
    kind: str,
    fmt: Optional[str],
) -> None:
    """Load and parse tuning report if present; enrich rec with tuned params, algorithm, metrics, etc."""
    report_suffix = f"_{fmt}" if fmt else ""
    report_name = f"tuning_report_{kind}{report_suffix}.json"
    report_path = os.path.join(models_dir, report_name)
    if report_name not in entries or not os.path.isfile(report_path):
        rec["tuned"] = False
        return
    try:
        with open(report_path, "r", encoding="utf-8") as f:
            report = json.load(f)
        rec["tuned"] = True
        rec["best_cv_score"] = report.get("best_cv_score")
        rec["scoring"] = report.get("scoring", "neg_mean_absolute_error")
        config = report.get("config_snippet") or report.get("best_params") or {}
        params: Dict[str, Any] = {}
        for k, v in config.items():
            k_clean = k
            if k.startswith("est__estimator__"):
                k_clean = k[len("est__estimator__") :]
            elif k.startswith("est__"):
                k_clean = k[len("est__") :]
            params[k_clean] = v
        rec["tuned_parameters"] = params
        rec["algorithms_requested"] = report.get("algorithms_requested")
        # Show only the selected algorithm (used for final model), not Phase 2 finalists
        selected_algo = params.get("algorithm")
        if selected_algo is not None:
            rec["algorithm"] = ALGORITHM_NAMES.get(str(selected_algo).lower(), str(selected_algo))
        else:
            algorithms = report.get("algorithms") or []
            best_key = algorithms[0] if algorithms else None
            rec["algorithm"] = ALGORITHM_NAMES.get(str(best_key).lower(), str(best_key)) if best_key else None
        rec["cv_splits"] = report.get("cv_splits")
        rec["validation_method"] = report.get("validation_method")
        rec["n_samples"] = report.get("n_samples")
        rec["n_features"] = report.get("n_features")
        metrics = report.get("metrics") or {}
        if metrics:
            rec["metrics"] = flatten_metrics_for_display(metrics)
        feature_importance = report.get("feature_importance")
        if feature_importance and isinstance(feature_importance, dict):
            rec["feature_importance"] = feature_importance
        mlqa = report.get("mlqa_audit")
        if mlqa and isinstance(mlqa, dict):
            rec["mlqa_audit"] = mlqa
        # Set accuracy_display from metrics (preferred) or fallback for neg_mean_absolute_error
        scoring = rec.get("scoring", "neg_mean_absolute_error")
        if "accuracy_pct" in (metrics or {}):
            rec["accuracy_display"] = f"{metrics['accuracy_pct']}%"
        elif "mae" in (metrics or {}):
            parts = [f"MAE={metrics['mae']}"]
            if "rmse" in metrics:
                parts.append(f"RMSE={metrics['rmse']}")
            if "r2_pct" in metrics:
                parts.append(f"R²={metrics['r2_pct']}%")
            rec["accuracy_display"] = ", ".join(parts)
        elif "r2_pct" in (metrics or {}):
            rec["accuracy_display"] = f"R²={metrics['r2_pct']}%"
        elif scoring == "neg_mean_absolute_error" and rec.get("best_cv_score") is not None:
            mae_val = abs(float(rec["best_cv_score"]))
            rec["accuracy_display"] = f"MAE={mae_val:.2f} (neg_MAE={rec['best_cv_score']:.4f})"
    except Exception as e:
        logger.debug("model_stats.read_report_failed", path=report_path, error=str(e))
        rec["tuned"] = False


def build_model_stats(models_dir: str) -> Dict[str, Any]:
    """Scan MODELS_DIR for model artifacts and tuning reports; return unified model stats."""
    stats: List[Dict[str, Any]] = []
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.warning("model_stats.listdir_failed", models_dir=models_dir, error=str(e))
        return {"models_dir": models_dir, "models": []}

    seen: set = set()
    for fname in entries:
        parsed = parse_model_filename(fname)
        if not parsed:
            continue
        kind, fmt = parsed
        key = (kind, fmt or "Unified")
        if key in seen:
            continue
        seen.add(key)

        rec = get_model_artifact_stats(models_dir, entries, kind, fmt, fname)
        if rec is None:
            continue

        enrich_with_tuning_report(rec, models_dir, entries, kind, fmt)
        stats.append(rec)

    stats.sort(key=lambda x: (x["model_name"], x["match_format"]))
    return {"models_dir": models_dir, "models": stats}
