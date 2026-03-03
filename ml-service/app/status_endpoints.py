from __future__ import annotations

import json
import os
import time
from typing import Any, Dict, List, Optional, Sequence, Tuple


def _artifacts_info(models_dir: str, prefix: str, logger) -> List[dict]:
    out = []
    try:
        for fname in os.listdir(models_dir):
            if fname.lower().startswith(prefix) and fname.lower().endswith(".joblib"):
                fpath = os.path.join(models_dir, fname)
                try:
                    st = os.stat(fpath)
                    out.append(
                        {
                            "file": fname,
                            "size_bytes": st.st_size,
                            "modified": int(st.st_mtime),
                        }
                    )
                except OSError as e:
                    logger.warning("health.artifacts_info.stat_failed", file=fname, error=str(e))
                    out.append({"file": fname})
    except OSError as e:
        logger.warning("health.artifacts_info.listdir_failed", models_dir=models_dir, error=str(e))
    return sorted(out, key=lambda x: x.get("file", ""))


def _metadata_info(models_dir: str, prefix: str, logger) -> List[str]:
    names: List[str] = []
    try:
        for fname in os.listdir(models_dir):
            lf = fname.lower()
            if lf.startswith(prefix) and lf.endswith(".json"):
                names.append(fname)
    except OSError as e:
        logger.warning("health.metadata_info.listdir_failed", models_dir=models_dir, error=str(e))
    return sorted(names)


def build_health_response(models_dir: str, model_registries: Dict[str, Dict[str, Any]], logger) -> Dict[str, Any]:
    response: dict = {
        "status": "ok",
        "models_dir": models_dir,
        "artifacts": {},
        "metadata": {},
        "counters": {},
    }
    for name, registry in model_registries.items():
        loaded = sorted([k for k in registry.keys() if k != "_LEGACY_"])
        response[f"loaded_{name}_formats"] = loaded
        response[f"legacy_{name}_available"] = "_LEGACY_" in registry
        response["artifacts"][name] = _artifacts_info(models_dir, f"{name}_", logger)
        if name in ("batting", "bowling", "fielding"):
            response["metadata"][name] = _metadata_info(models_dir, f"{name}_metadata_", logger)
        response["counters"][f"{name}_formats"] = len(loaded)
    return response


def _find_per_format_artifact(
    models_dir: str,
    fmt: str,
    kind: str,
    logger,
) -> Optional[Tuple[str, float]]:
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.debug(
            "artifacts_status.find_per_format.listdir_failed",
            models_dir=models_dir,
            fmt=fmt,
            kind=kind,
            error=str(e),
        )
        return None
    if kind == "batting":
        scaler_name = f"batting_scaler_{fmt}.joblib"
        model_name = f"batting_model_{fmt}.joblib"
    elif kind == "bowling":
        scaler_name = f"bowling_scaler_{fmt}.joblib"
        model_name = f"bowling_model_{fmt}.joblib"
    elif kind == "fielding":
        scaler_name = f"fielding_scaler_{fmt}.joblib"
        model_name = f"fielding_model_{fmt}.joblib"
    elif kind == "extras":
        model_name = f"extras_model_{fmt}.joblib"
        scaler_name = None
    elif kind == "win":
        model_name = f"win_model_{fmt}.joblib"
        scaler_name = None
    else:
        return None
    if scaler_name and scaler_name not in entries:
        return None
    if model_name not in entries:
        return None
    path = os.path.join(models_dir, model_name)
    try:
        st = os.stat(path)
        if not os.path.isfile(path):
            return None
        return path, st.st_mtime
    except Exception:
        return None


def _find_legacy_artifact(models_dir: str, kind: str, logger) -> Optional[Tuple[str, float]]:
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.debug(
            "artifacts_status.find_legacy.listdir_failed",
            models_dir=models_dir,
            kind=kind,
            error=str(e),
        )
        return None
    if kind == "batting":
        if "batting_scaler.joblib" not in entries or "batting_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "batting_model.joblib")
    elif kind == "bowling":
        if "bowling_scaler.joblib" not in entries or "bowling_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "bowling_model.joblib")
    elif kind == "fielding":
        if "fielding_scaler.joblib" not in entries or "fielding_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "fielding_model.joblib")
    elif kind == "extras":
        if "extras_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "extras_model.joblib")
    elif kind == "win":
        if "win_model.joblib" not in entries:
            return None
        path = os.path.join(models_dir, "win_model.joblib")
    else:
        return None
    try:
        st = os.stat(path)
        if not os.path.isfile(path):
            return None
        return path, st.st_mtime
    except Exception:
        return None


def _legacy_status_obj(
    models_dir: str,
    kind: str,
    loaded: bool,
    logger,
) -> Dict[str, Any]:
    out: Dict[str, Any] = {"exists": False}
    hit = _find_legacy_artifact(models_dir, kind, logger)
    if hit is not None:
        p, mt = hit
        out["exists"] = True
        out["path"] = p
        out["modified"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mt))
    out["loaded"] = loaded
    return out


def build_artifacts_status_payload(
    models_dir: str,
    formats: Sequence[str],
    loaded_registries: Dict[str, Any],
    logger,
) -> Dict[str, Any]:
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    artifact_kinds = ["batting", "bowling", "fielding", "extras", "win"]
    formats_out: Dict[str, Dict[str, Any]] = {}
    for fmt in formats:
        row: Dict[str, Dict[str, Any]] = {}
        for kind in artifact_kinds:
            obj: Dict[str, Any] = {"exists": False}
            hit = _find_per_format_artifact(models_dir, fmt, kind, logger)
            if hit is not None:
                p, mt = hit
                obj["exists"] = True
                obj["path"] = p
                obj["modified"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(mt))
            try:
                reg = loaded_registries.get(kind)
                if reg is not None and fmt in reg:
                    obj["loaded"] = True
            except Exception as e:
                logger.debug("artifacts_status.loaded_check", fmt=fmt, kind=kind, error=str(e))
            row[kind] = obj
        formats_out[fmt] = row

    legacy: Dict[str, Dict[str, Any]] = {
        "batting": _legacy_status_obj(models_dir, "batting", "_LEGACY_" in loaded_registries.get("batting", {}), logger),
        "bowling": _legacy_status_obj(models_dir, "bowling", "_LEGACY_" in loaded_registries.get("bowling", {}), logger),
        "fielding": _legacy_status_obj(models_dir, "fielding", "_LEGACY_" in loaded_registries.get("fielding", {}), logger),
        "extras": _legacy_status_obj(models_dir, "extras", "_LEGACY_" in loaded_registries.get("extras", {}), logger),
        "win": _legacy_status_obj(models_dir, "win", "_LEGACY_" in loaded_registries.get("win", {}), logger),
    }

    return {"timestamp": ts, "root": models_dir, "formats": formats_out, "legacy": legacy}


_MODEL_STATS_KINDS = ["batting", "bowling", "fielding", "extras", "win"]
_MODEL_STATS_HAS_SCALER = {"batting", "bowling", "fielding"}


def _parse_model_filename(fname: str) -> Optional[Tuple[str, Optional[str]]]:
    lower = fname.lower()
    if not lower.endswith(".joblib") or "_model" not in lower:
        return None
    for kind in _MODEL_STATS_KINDS:
        prefix = f"{kind}_model"
        if lower.startswith(prefix):
            rest = fname[len(prefix) :].lstrip("_").rstrip(".joblib")
            fmt = rest.upper() if rest else None
            return (kind, fmt)
    return None


def _get_model_artifact_stats(
    models_dir: str,
    entries: List[str],
    kind: str,
    fmt: Optional[str],
    fname: str,
) -> Optional[Dict[str, Any]]:
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

    if kind in _MODEL_STATS_HAS_SCALER:
        scaler_suffix = f"_{fmt}" if fmt else ""
        scaler_name = f"{kind}_scaler{scaler_suffix}.joblib"
        scaler_path = os.path.join(models_dir, scaler_name)
        if scaler_name in entries and os.path.isfile(scaler_path):
            try:
                size_bytes += os.path.getsize(scaler_path)
            except OSError:
                pass

    return {
        "model_name": kind.capitalize(),
        "match_format": fmt or "Unified",
        "size_bytes": size_bytes,
        "modified": modified_iso,
    }


def _flatten_metrics_for_display(metrics: Dict[str, Any]) -> Dict[str, Any]:
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


def _enrich_with_tuning_report(
    rec: Dict[str, Any],
    models_dir: str,
    entries: List[str],
    kind: str,
    fmt: Optional[str],
    logger,
) -> None:
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
        selected_algo = params.get("algorithm")
        if selected_algo is not None:
            from .main import _ALGORITHM_NAMES  # type: ignore

            rec["algorithm"] = _ALGORITHM_NAMES.get(str(selected_algo).lower(), str(selected_algo))
        else:
            algorithms = report.get("algorithms") or []
            best_key = algorithms[0] if algorithms else None
            if best_key is not None:
                from .main import _ALGORITHM_NAMES  # type: ignore

                rec["algorithm"] = _ALGORITHM_NAMES.get(str(best_key).lower(), str(best_key))
            else:
                rec["algorithm"] = None
        rec["cv_splits"] = report.get("cv_splits")
        rec["validation_method"] = report.get("validation_method")
        rec["n_samples"] = report.get("n_samples")
        rec["n_features"] = report.get("n_features")
        metrics = report.get("metrics") or {}
        if metrics:
            rec["metrics"] = _flatten_metrics_for_display(metrics)
        feature_importance = report.get("feature_importance")
        if feature_importance and isinstance(feature_importance, dict):
            rec["feature_importance"] = feature_importance
        mlqa = report.get("mlqa_audit")
        if mlqa and isinstance(mlqa, dict):
            rec["mlqa_audit"] = mlqa
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


def build_model_stats(models_dir: str, logger) -> Dict[str, Any]:
    stats: List[Dict[str, Any]] = []
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.warning("model_stats.listdir_failed", models_dir=models_dir, error=str(e))
        return {"models_dir": models_dir, "models": []}

    seen: set = set()
    for fname in entries:
        parsed = _parse_model_filename(fname)
        if not parsed:
            continue
        kind, fmt = parsed
        key = (kind, fmt or "Unified")
        if key in seen:
            continue

        seen.add(key)
        rec = _get_model_artifact_stats(models_dir, entries, kind, fmt, fname)
        if rec is None:
            continue
        _enrich_with_tuning_report(rec, models_dir, entries, kind, fmt, logger)
        stats.append(rec)

    stats.sort(key=lambda x: (x["model_name"], x["match_format"]))
    return {"models_dir": models_dir, "models": stats}

