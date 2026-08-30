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

from ml.artifact_sidecar import meta_filename
from ml.config import get_mlqa_config

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


def _accuracy_display_from_metrics(metrics: Dict[str, Any]) -> Optional[str]:
    """Build the one-line score for the model-stats table, or None when nothing is scored.

    Shared by the tuning-report and training-sidecar readers so a holdout MAE and a
    tuned CV MAE are at least formatted identically; `score_source` is what tells the
    reader they were measured differently.
    """
    if "accuracy_pct" in metrics:
        return f"{metrics['accuracy_pct']}%"
    if "mae" in metrics:
        parts = [f"MAE={metrics['mae']}"]
        if "rmse" in metrics:
            parts.append(f"RMSE={metrics['rmse']}")
        if "r2_pct" in metrics:
            parts.append(f"R²={metrics['r2_pct']}%")
        return ", ".join(parts)
    if "r2_pct" in metrics:
        return f"R²={metrics['r2_pct']}%"
    return None


def _recompute_mlqa_audit_from_report(report: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """Recompute MLQA audit using current thresholds and stored metrics.

    This avoids re-running tuning: it interprets the existing delta/std/fairness
    metrics under the latest ml.mlqa config. Falls back to the original audit
    when metrics are missing or config cannot be loaded.
    """
    original = report.get("mlqa_audit")
    if not isinstance(original, dict):
        return None

    checks = original.get("checks") or {}
    over = checks.get("overfitting") or {}
    stab = checks.get("stability") or {}

    delta = over.get("delta")
    fold_std = stab.get("cv_std")
    cv_fold_scores = stab.get("cv_fold_scores")

    fairness = report.get("fairness_metrics") or {}
    dip = fairness.get("disparate_impact_ratio")

    feature_importance = report.get("feature_importance") or {}

    if delta is None and fold_std is None and dip is None and not feature_importance:
        # Not enough structured metrics to safely recompute; keep existing audit.
        return original

    try:
        mlqa_cfg = get_mlqa_config()
    except Exception as e:  # pragma: no cover - defensive, config is tested elsewhere
        logger.debug("model_stats.mlqa_config_failed", error=str(e))
        return original

    # Thresholds come only from ml.mlqa (merged config + defaults in get_mlqa_config); not duplicated here.
    delta_thresh = float(mlqa_cfg.get("overfitting_delta_threshold", 0.10))
    std_thresh = float(mlqa_cfg.get("stability_fold_std_threshold", 0.08))
    dip_low = float(mlqa_cfg.get("bias_dip_low", 0.8))
    dip_high = float(mlqa_cfg.get("bias_dip_high", 1.25))
    top_weight_thresh = float(mlqa_cfg.get("sensitivity_top_weight_threshold", 0.70))

    findings: List[str] = []
    status_flags: List[str] = []
    bias_report = original.get("bias_report", "No protected groups defined; fairness audit skipped.")

    # Reference score for relative threshold computation (same logic as cv_metrics._to_relative).
    val_score = report.get("best_cv_score")
    score_magnitude = abs(float(val_score)) if val_score is not None else 0.0

    def _to_relative(absolute_value: float) -> float:
        """Convert absolute metric to relative fraction of score magnitude."""
        if score_magnitude < 1e-9:
            return float("inf") if abs(absolute_value) > 1e-9 else 0.0
        return abs(absolute_value) / score_magnitude

    # 1. Overfitting: train–validation delta vs relative threshold
    overfitting_risk = False
    if delta is not None:
        delta_f = float(delta)
        rel_delta = _to_relative(delta_f)
        overfitting_risk = rel_delta > delta_thresh
        if overfitting_risk:
            findings.append(
                f"High Overfitting Risk: Train–Validation Δ = {delta_f:.4f} "
                f"(relative {rel_delta:.2%} > {delta_thresh:.0%})."
            )
            status_flags.append("overfitting")
        else:
            findings.append(f"Overfitting check OK: Δ = {delta_f:.4f} (relative {rel_delta:.2%} ≤ {delta_thresh:.0%}).")

    # 2. Stability: CV fold std vs relative threshold
    unstable = False
    if fold_std is not None:
        fold_std_f = float(fold_std)
        rel_std = _to_relative(fold_std_f)
        unstable = rel_std > std_thresh
        if unstable:
            findings.append(f"Unstable: CV fold σ = {fold_std_f:.4f} (relative {rel_std:.2%} > {std_thresh:.0%}).")
            status_flags.append("unstable")
        else:
            findings.append(f"Stability OK: CV fold σ = {fold_std_f:.4f} (relative {rel_std:.2%} ≤ {std_thresh:.0%}).")

    # 3. Bias & Fairness: disparate impact ratio
    if dip is not None:
        dip_f = float(dip)
        biased = dip_f < dip_low or dip_f > dip_high
        if biased:
            findings.append(f"Biased Model: disparate_impact_ratio = {dip_f:.4f} outside [{dip_low}, {dip_high}].")
            status_flags.append("biased")
            bias_report = "Model shows disparate impact; review protected group treatment before deployment."
        else:
            findings.append(f"Fairness OK: disparate_impact_ratio = {dip_f:.4f} in [{dip_low}, {dip_high}].")
            bias_report = "Model treats subgroups equitably within defined fairness bounds."

    # 4. Sensitivity: top feature weight from feature_importance dict (if present)
    if isinstance(feature_importance, dict) and feature_importance:
        try:
            # Use absolute importance values to guard against negative importances.
            items = list(feature_importance.items())
            # Sort by descending importance
            items.sort(key=lambda kv: abs(float(kv[1])), reverse=True)
            total = sum(abs(float(v)) for _, v in items)
            if total > 0:
                top_name, top_val = items[0]
                top_weight = abs(float(top_val)) / total
                if top_weight > top_weight_thresh:
                    findings.append(
                        f"Potential Data Leakage / Low Robustness: top feature '{top_name}' = {top_weight * 100:.1f}%."
                    )
                    status_flags.append("sensitivity")
                else:
                    findings.append(
                        f"Sensitivity OK: top feature weight = {top_weight * 100:.1f}% ≤ {top_weight_thresh * 100:.0f}%."
                    )
        except Exception:
            # Do not fail model-stats if feature importance is malformed; just skip sensitivity.
            pass

    # Aggregate status and verdict using the same rules as cv_metrics._compute_mlqa_audit
    if status_flags:
        audit_status = "FAIL" if any(f in ("overfitting", "unstable", "biased") for f in status_flags) else "WARNING"
    else:
        audit_status = "PASS"
    if audit_status == "FAIL":
        final_verdict = "Rollback & Re-tune"
    else:
        final_verdict = "Proceed to Deployment"

    # Build updated checks block.
    new_checks: Dict[str, Any] = {}
    if delta is not None:
        delta_f = float(delta)
        rel_delta = _to_relative(delta_f)
        new_checks["overfitting"] = {
            "delta": delta_f,
            "relative_delta": round(rel_delta, 4),
            "threshold": delta_thresh,
            "flagged": overfitting_risk,
        }
    if fold_std is not None or cv_fold_scores is not None:
        stab_block: Dict[str, Any] = {}
        if fold_std is not None:
            fold_std_f = float(fold_std)
            rel_std = _to_relative(fold_std_f)
            stab_block["cv_std"] = fold_std_f
            stab_block["relative_cv_std"] = round(rel_std, 4)
            stab_block["threshold"] = std_thresh
            stab_block["flagged"] = unstable
        if cv_fold_scores is not None:
            # Keep any existing scores for UI display.
            stab_block["cv_fold_scores"] = cv_fold_scores
        new_checks["stability"] = stab_block

    # Preserve original findings as a fallback if we could not recompute anything.
    if not findings and isinstance(original.get("key_findings"), list):
        findings = list(original["key_findings"])

    return {
        "audit_status": audit_status,
        "key_findings": findings,
        "bias_report": bias_report,
        "final_verdict": final_verdict,
        "checks": new_checks or checks,
    }


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
        # Re-interpret stored MLQA metrics under the current thresholds so that
        # existing models reflect updated audit config without re-running tuning.
        mlqa = _recompute_mlqa_audit_from_report(report)
        if mlqa and isinstance(mlqa, dict):
            rec["mlqa_audit"] = mlqa
        # Set accuracy_display from metrics (preferred) or fallback for neg_mean_absolute_error
        scoring = rec.get("scoring", "neg_mean_absolute_error")
        display = _accuracy_display_from_metrics(metrics or {})
        if display:
            rec["accuracy_display"] = display
            rec["score_source"] = "tuning_cv"
        elif scoring == "neg_mean_absolute_error" and rec.get("best_cv_score") is not None:
            mae_val = abs(float(rec["best_cv_score"]))
            rec["accuracy_display"] = f"MAE={mae_val:.2f} (neg_MAE={rec['best_cv_score']:.4f})"
            rec["score_source"] = "tuning_cv"
    except Exception as e:
        logger.debug("model_stats.read_report_failed", path=report_path, error=str(e))
        rec["tuned"] = False


#: The two sidecars a trained model may have, in the order they are preferred.
#:
#: `<kind>_metadata_<fmt>.json` is written by TrainingPipeline (batting, bowling,
#: fielding) and `<kind>_meta_<fmt>.json` by artifact_sidecar (extras, innings, and
#: win's own builder). Two names because two writers, and unifying them would rename
#: files that inference already reads by name.
def _sidecar_candidates(kind: str, fmt: Optional[str]) -> List[str]:
    suffix = fmt or "LEGACY"
    return [
        f"{kind}_metadata_{suffix}.json",
        meta_filename(kind, fmt),
    ]


def _read_sidecars(models_dir: str, kind: str, fmt: Optional[str]) -> List[Dict[str, Any]]:
    """Return every readable sidecar for this model, in preference order.

    A missing or unparseable sidecar is skipped rather than fatal: it means one less
    thing we can say about the model, not that the model is unreportable.
    """
    sidecars: List[Dict[str, Any]] = []
    for name in _sidecar_candidates(kind, fmt):
        path = os.path.join(models_dir, name)
        try:
            with open(path, "r", encoding="utf-8") as f:
                sidecar = json.load(f)
        except (FileNotFoundError, NotADirectoryError):
            continue
        except (json.JSONDecodeError, OSError) as e:
            logger.debug("model_stats.sidecar_unreadable", path=path, error=str(e))
            continue
        if isinstance(sidecar, dict):
            sidecars.append(sidecar)
    return sidecars


def enrich_with_provenance(
    rec: Dict[str, Any],
    models_dir: str,
    kind: str,
    fmt: Optional[str],
) -> None:
    """Attach the dataset a model was trained on, from its sidecar (ops plan P-2).

    Absent when the model predates P-1 or was trained from CSVs with no export
    manifest. That is reported by omission rather than as an empty block: "we do not
    know what this model was trained on" is the answer, and it is the one worth
    flagging.
    """
    for sidecar in _read_sidecars(models_dir, kind, fmt):
        provenance = sidecar.get("provenance")
        if isinstance(provenance, dict) and provenance:
            rec["provenance"] = provenance
            return


def _sidecar_metrics(sidecar: Dict[str, Any]) -> Dict[str, Any]:
    """Pull the scored metrics out of a training sidecar.

    Two keys because two writers: TrainingPipeline writes its trailing-holdout scores
    under `metrics`, train_win its walk-forward scores under `cv_metrics`.
    """
    for key in ("metrics", "cv_metrics"):
        block = sidecar.get(key)
        if isinstance(block, dict) and block:
            return block
    return {}


def enrich_with_training_metrics(
    rec: Dict[str, Any],
    models_dir: str,
    kind: str,
    fmt: Optional[str],
) -> None:
    """Report what a single-train run measured, for models that were never auto-tuned.

    A tuning report always wins: its cross-validated score is the stronger measurement,
    and overwriting it with a holdout would be a downgrade. This only fills the gap left
    when there is no report — previously every column but size and mtime, which made a
    trained-from-config model indistinguishable from a broken one in the UI.

    `tuned` is deliberately left False. The score is labelled with `score_source` so a
    holdout MAE and a tuned CV MAE are not read as the same number in the same column.
    """
    if rec.get("tuned"):
        return
    for sidecar in _read_sidecars(models_dir, kind, fmt):
        metrics = _sidecar_metrics(sidecar)
        if not metrics:
            continue
        rec["metrics"] = flatten_metrics_for_display(metrics)
        rec["score_source"] = sidecar.get("score_source", "holdout")
        display = _accuracy_display_from_metrics(metrics)
        if display:
            rec["accuracy_display"] = display
        for key in ("validation_method", "n_samples", "n_features", "trained_at", "duration_seconds"):
            value = sidecar.get(key)
            if value is not None:
                rec[key] = value
        algorithm = sidecar.get("algorithm")
        if algorithm is not None:
            rec["algorithm"] = ALGORITHM_NAMES.get(str(algorithm).lower(), str(algorithm))
        return


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
        enrich_with_training_metrics(rec, models_dir, kind, fmt)
        enrich_with_provenance(rec, models_dir, kind, fmt)
        stats.append(rec)

    stats.sort(key=lambda x: (x["model_name"], x["match_format"]))
    return {"models_dir": models_dir, "models": stats}
