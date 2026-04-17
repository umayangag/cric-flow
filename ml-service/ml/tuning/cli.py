"""CLI entrypoint for auto-tune (python -m ml.tuning or python -m ml.auto_tune)."""

from __future__ import annotations

import argparse
import json
import logging
import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Dict, List, Optional

import numpy as np

_ML_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _ML_ROOT not in sys.path:
    sys.path.insert(0, _ML_ROOT)

from ml.config import (
    default_go_app_export_dir,
    save_tuned_params_to_go_app,
)
from ml.tuning.data_loaders import (
    LoaderResult,
    _load_via_csv_or_api,
    load_batting_csv,
    load_batting_from_api,
    load_bowling_csv,
    load_bowling_from_api,
    load_extras_csv,
    load_extras_from_api,
    load_fielding_csv,
    load_fielding_from_api,
    load_innings_from_api,
    load_win_csv,
    load_win_from_api,
)
from ml.tuning.runners import (
    run_auto_tune,
    run_auto_tune_extras,
    run_auto_tune_win,
)

_EXTRAS_LEGACY_KEY = "_LEGACY_"


def _extras_pop_legacy(by_f: Dict[str, LoaderResult]) -> Optional[LoaderResult]:
    """Pop the aggregated legacy pack from the extras by_format dict.

    ``ml.train_extras.rows_to_xy_by_format`` emits a special ``_LEGACY_`` entry
    containing the unified pool (``format_is_*`` retained, single low-variance
    drop). CLI callers must separate this from real format entries before
    iterating.
    """
    return by_f.pop(_EXTRAS_LEGACY_KEY, None)


def _shared_feature_names(by_f: Dict[str, LoaderResult]) -> Optional[List[str]]:
    """Return the shared feature_names list when every entry has the same list; else None.

    Required when the caller will ``np.vstack`` per-format matrices — a mismatch
    in column count or ordering would silently produce a wrong-shape matrix or
    raise at stack time.
    """
    if not by_f:
        return None
    results = list(by_f.values())
    first = results[0].feature_names
    if first is not None and all(r.feature_names == first for r in results):
        return list(first)
    return None


try:
    from ml import auto_tune_progress as _progress
except ImportError:
    _progress = None

logger = logging.getLogger(__name__)


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
    parser = argparse.ArgumentParser(description="Auto-tune ML models (batting, bowling, fielding, extras, win)")
    parser.add_argument(
        "--model",
        choices=["batting", "bowling", "fielding", "extras", "win", "innings", "all"],
        default="batting",
    )
    parser.add_argument("--csv", default="", help="Path to CSV (for batting/bowling/fielding)")
    parser.add_argument("--from-api", action="store_true", help="Fetch data from go-app training-data API")
    parser.add_argument("--cutoff", default="", help="RFC3339 cutoff (required with --from-api)")
    parser.add_argument(
        "--format", default="", help="Format code (e.g. T20, ODI); used for artifact suffix and API filter"
    )
    parser.add_argument("--all-formats", action="store_true", help="Loop over ml.formats and tune each (CSV only)")
    parser.add_argument(
        "--unified",
        action="store_true",
        help="Tune one unified model on all formats combined (saves to legacy artifact names, no format suffix).",
    )
    parser.add_argument("--out", default="", help="Output dir (default: ML_SERVICE_OUTPUT_DIR or config)")
    parser.add_argument("--go-app-url", default=os.environ.get("GO_APP_URL", ""))
    parser.add_argument("--api-key", default=os.environ.get("GO_APP_API_KEY", ""))
    parser.add_argument(
        "--algorithms",
        default="",
        help="Comma-separated algorithms to tune: rf, gb, quantile (regression only), stacked (batting/bowling/fielding only). Default: all from config.",
    )
    parser.add_argument(
        "--validation-method",
        choices=["kfold", "walk_forward"],
        default="",
        help="Validation method: kfold or walk_forward (temporal). Default: walk_forward (from config).",
    )
    parser.add_argument(
        "--parallel",
        action="store_true",
        help="Run multiple (model, format) tasks in parallel, using up to 80%% of available CPUs (each task uses 1 job).",
    )
    parser.add_argument(
        "--no-pycaret",
        action="store_true",
        help="Skip PyCaret algorithm ranking (use config algorithms only).",
    )
    parser.add_argument(
        "--fast",
        action="store_true",
        help="Fast mode: reduce Optuna trials to 15, skip PyCaret and AutoGluon.",
    )
    parser.add_argument(
        "--no-autogluon",
        action="store_true",
        help="Skip AutoGluon accuracy boost stage.",
    )
    parser.add_argument(
        "--rescreen",
        action="store_true",
        help="Force full algorithm screening even when prior tuned params exist (ignore fine-tune-only mode).",
    )
    args = parser.parse_args()

    logger.info(
        "pipeline: auto_tune starting model=%s from_api=%s format=%s all_formats=%s unified=%s out_dir=%s",
        args.model,
        args.from_api,
        args.format or "(detected)",
        args.all_formats,
        args.unified,
        args.out or "(default)",
    )

    algorithms_override: Optional[List[str]] = None
    if args.algorithms:
        algorithms_override = [a.strip().lower() for a in args.algorithms.split(",") if a.strip()]
    validation_method_override: Optional[str] = args.validation_method or None
    use_pycaret: Optional[bool] = False if (args.no_pycaret or args.fast) else None
    fast_mode: bool = bool(args.fast)
    use_autogluon: Optional[bool] = False if (args.no_autogluon or args.fast) else None

    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR")
    if not out_dir:
        from ml.config import default_artifacts_dir

        out_dir = default_artifacts_dir()

    progress_path = os.environ.get("AUTO_TUNE_PROGRESS_FILE") or os.path.join(out_dir, "auto_tune_progress.json")
    _progress.set_progress_file(progress_path)

    def _config_formats() -> List[str]:
        try:
            with open(os.path.join(_ML_ROOT, "config.json"), "r", encoding="utf-8") as f:
                data = json.load(f)
                fmts = data.get("ml", {}).get("formats") or []
                return [str(x).upper() for x in fmts if isinstance(x, (str, int))]
        except Exception:
            return ["T20", "ODI", "T20I"]

    models = ["batting", "bowling", "fielding", "extras", "win", "innings"] if args.model == "all" else [args.model]
    formats_to_run: List[Optional[str]] = [None]
    if args.unified:
        formats_to_run = [None]
    elif args.all_formats:
        formats_to_run = _config_formats()
    elif args.format:
        formats_to_run = [args.format.strip().upper()]

    def _maybe_save_tuned_params(
        go_app_url: str,
        model: str,
        format_suffix: Optional[str],
        report: Dict[str, Any],
        api_key: Optional[str],
    ) -> None:
        """Save best params to go-app per (model, format) so they are stored in DB and used when retraining (GO_APP_URL must be set)."""
        if not go_app_url or not report.get("config_snippet"):
            return
        params_to_save = dict(report["config_snippet"])
        params_to_save["algorithms"] = report.get("algorithms", [])
        params_to_save["validation_method"] = report.get("validation_method", "walk_forward")
        # Build metrics for DB: always include best_cv_score and tuning context for debugging.
        # Include mlqa_audit so model-stats UI can display audit data for all models (batting, win, etc.).
        metrics_to_save: Dict[str, Any] = dict(report.get("metrics") or {})
        if report.get("best_cv_score") is not None:
            metrics_to_save["best_cv_score"] = report["best_cv_score"]
        for key in ("scoring", "cv_splits", "validation_method", "n_samples", "n_features", "n_targets"):
            if report.get(key) is not None:
                metrics_to_save[key] = report[key]
        mlqa = report.get("mlqa_audit")
        if mlqa and isinstance(mlqa, dict):
            metrics_to_save["mlqa_audit"] = mlqa
        for key in ("consistency_penalty", "base_mae", "score_combined_mae"):
            if report.get(key) is not None:
                metrics_to_save[key] = report[key]
        try:
            save_tuned_params_to_go_app(
                go_app_url, model, format_suffix or "", params_to_save, api_key, metrics=metrics_to_save
            )
            logger.info("auto_tune.params_saved_to_db model=%s format=%s", model, format_suffix or "(unified)")
        except ValueError as e:
            logger.warning(
                "auto_tune.save_tuned_params_failed model=%s format=%s error=%s",
                model,
                format_suffix,
                e,
            )

    def _build_parallel_tasks() -> List[List[str]]:
        """Build list of argv for each (model, format) to run as subprocess (AUTO_TUNE_N_JOBS=1)."""
        cmds: List[List[str]] = []
        for model_kind in models:
            for fmt in formats_to_run:
                argv = [sys.executable, "-m", "ml.auto_tune", "--model", model_kind, "--out", out_dir]
                if args.unified:
                    argv.append("--unified")
                elif fmt:
                    argv.extend(["--format", fmt])
                if args.from_api:
                    argv.extend(["--from-api", "--cutoff", args.cutoff or "", "--go-app-url", args.go_app_url or ""])
                    if args.api_key:
                        argv.extend(["--api-key", args.api_key])
                if args.csv:
                    argv.extend(["--csv", args.csv])
                if algorithms_override:
                    argv.extend(["--algorithms", ",".join(algorithms_override)])
                if validation_method_override:
                    argv.extend(["--validation-method", validation_method_override])
                if args.no_pycaret:
                    argv.append("--no-pycaret")
                if args.fast:
                    argv.append("--fast")
                if args.no_autogluon:
                    argv.append("--no-autogluon")
                if args.rescreen:
                    argv.append("--rescreen")
                cmds.append(argv)
        return cmds

    try:
        if args.parallel:
            parallel_tasks = _build_parallel_tasks()
            cpu_count = os.cpu_count() or 1
            max_workers = min(len(parallel_tasks), max(1, int(cpu_count * 0.8)))
            if len(parallel_tasks) <= 1:
                logger.info("auto_tune.parallel only one task, running sequentially")
            else:
                logger.info(
                    "auto_tune.parallel running %s tasks with max_workers=%s (80%% of %s CPUs)",
                    len(parallel_tasks),
                    max_workers,
                    cpu_count,
                )
                env = os.environ.copy()
                env["AUTO_TUNE_N_JOBS"] = "1"
                _config_path = os.path.join(_ML_ROOT, "config.json")
                if os.path.isfile(_config_path):
                    env["ML_SERVICE_CONFIG"] = _config_path
                failed = 0
                with ThreadPoolExecutor(max_workers=max_workers) as executor:
                    futures = {
                        executor.submit(subprocess.run, argv, env=env, cwd=_ML_ROOT, capture_output=False): argv
                        for argv in parallel_tasks
                    }
                    for future in as_completed(futures):
                        argv = futures[future]
                        try:
                            result = future.result()
                            if result.returncode != 0:
                                failed += 1
                                logger.warning("auto_tune.parallel task failed: %s", " ".join(argv[:10]))
                        except Exception as e:
                            failed += 1
                            logger.warning("auto_tune.parallel task error: %s", e)
                if failed:
                    sys.exit(1)
                return

        task_idx = 0
        total_tasks = len(models) * len(formats_to_run)
        default_dir = os.environ.get("GO_APP_OUTPUT_DIR", default_go_app_export_dir())
        api_available = bool(args.go_app_url and args.cutoff)
        if args.from_api and not api_available:
            logger.error("auto_tune.from_api_requires_go_app_url_and_cutoff")
            sys.exit(1)

        for model_kind in models:
            for fmt in formats_to_run:
                format_suffix = fmt if fmt else None
                feat_names: Optional[List[str]] = None
                task_idx += 1
                _progress.write_progress(
                    phase="loading",
                    model_kind=model_kind,
                    format_suffix=format_suffix or "",
                    task_index=task_idx,
                    task_total=max(1, total_tasks),
                    message="Loading training data",
                    algorithms_requested=algorithms_override or [],
                    activity="loading_data",
                )
                if args.from_api:
                    try:
                        if model_kind == "extras":
                            csv_path = args.csv or os.path.join(default_dir, "extras_encoded_all.csv")
                            by_f = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_extras_csv(csv_path, fmt),
                                lambda: load_extras_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt),
                                can_fallback_to_api=api_available,
                            )
                            if not by_f:
                                logger.warning("auto_tune.no_extras_data format=%s", fmt)
                                continue
                            legacy_pack = _extras_pop_legacy(by_f)
                            if args.unified:
                                # Prefer the loader's aggregated legacy pack (retains format_is_*, single
                                # low-variance drop). Fall back to vstack when the loader did not emit one.
                                if legacy_pack is not None:
                                    all_X, all_Y = legacy_pack.X, legacy_pack.Y
                                    unified_feature_names = legacy_pack.feature_names
                                else:
                                    all_X = np.vstack([lr.X for lr in by_f.values()])
                                    all_Y = np.vstack([lr.Y for lr in by_f.values()])
                                    unified_feature_names = _shared_feature_names(by_f)
                                if all_X.size == 0 or all_Y.size == 0:
                                    logger.warning("auto_tune.no_extras_data unified empty")
                                    continue
                                report = run_auto_tune_extras(
                                    all_X,
                                    all_Y,
                                    None,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    use_autogluon=use_autogluon,
                                    rescreen=args.rescreen,
                                    algorithms_explicitly_passed=bool(algorithms_override),
                                    feature_names=unified_feature_names,
                                )
                                _maybe_save_tuned_params(args.go_app_url, "extras", None, report, args.api_key or None)
                                logger.info(
                                    "auto_tune.done model=extras format=unified n=%s best_cv_score=%s",
                                    all_X.shape[0],
                                    report["best_cv_score"],
                                )
                            else:
                                for fcode, pack in by_f.items():
                                    X, Y = pack.X, pack.Y
                                    if X.size == 0 or Y.size == 0:
                                        continue
                                    report = run_auto_tune_extras(
                                        X,
                                        Y,
                                        fcode,
                                        out_dir,
                                        algorithms_override,
                                        validation_method_override,
                                        use_pycaret=use_pycaret,
                                        fast_mode=fast_mode,
                                        use_autogluon=use_autogluon,
                                        rescreen=args.rescreen,
                                        algorithms_explicitly_passed=bool(algorithms_override),
                                        feature_names=pack.feature_names,
                                    )
                                    _maybe_save_tuned_params(
                                        args.go_app_url, "extras", fcode, report, args.api_key or None
                                    )
                                    logger.info(
                                        "auto_tune.done model=extras format=%s n=%s best_cv_score=%s",
                                        fcode,
                                        X.shape[0],
                                        report["best_cv_score"],
                                    )
                            continue
                        if model_kind == "win":
                            csv_path = args.csv or os.path.join(default_dir, "win_encoded_all.csv")
                            by_f = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_win_csv(csv_path, fmt),
                                lambda: load_win_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt),
                                can_fallback_to_api=api_available,
                            )
                            if not by_f:
                                logger.warning("auto_tune.no_win_data format=%s", fmt)
                                continue
                            if args.unified:
                                all_X = np.vstack([lr.X for lr in by_f.values()])
                                all_Y = np.concatenate([lr.Y.ravel() for lr in by_f.values()])
                                if all_X.size == 0 or all_Y.size == 0:
                                    logger.warning("auto_tune.no_win_data unified empty")
                                    continue
                                report = run_auto_tune_win(
                                    all_X,
                                    all_Y,
                                    None,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    use_autogluon=use_autogluon,
                                    rescreen=args.rescreen,
                                    algorithms_explicitly_passed=bool(algorithms_override),
                                )
                                _maybe_save_tuned_params(args.go_app_url, "win", None, report, args.api_key or None)
                                logger.info(
                                    "auto_tune.done model=win format=unified n=%s best_cv_score=%s",
                                    all_X.shape[0],
                                    report["best_cv_score"],
                                )
                            else:
                                for fcode, lr in by_f.items():
                                    X, Y = lr.X, lr.Y
                                    if X.size == 0 or Y.size == 0:
                                        continue
                                    report = run_auto_tune_win(
                                        X,
                                        Y,
                                        fcode,
                                        out_dir,
                                        algorithms_override,
                                        validation_method_override,
                                        use_pycaret=use_pycaret,
                                        fast_mode=fast_mode,
                                        use_autogluon=use_autogluon,
                                        rescreen=args.rescreen,
                                        algorithms_explicitly_passed=bool(algorithms_override),
                                    )
                                    _maybe_save_tuned_params(
                                        args.go_app_url, "win", fcode, report, args.api_key or None
                                    )
                                    logger.info(
                                        "auto_tune.done model=win format=%s n=%s best_cv_score=%s",
                                        fcode,
                                        X.shape[0],
                                        report["best_cv_score"],
                                    )
                            continue
                        if model_kind == "innings":
                            by_f = load_innings_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt)
                            if not by_f:
                                logger.warning("auto_tune.no_innings_data format=%s", fmt)
                                continue
                            if args.unified:
                                all_X = np.vstack([lr.X for lr in by_f.values()])
                                all_Y = np.vstack([lr.Y for lr in by_f.values()])
                                if all_X.size == 0 or all_Y.size == 0:
                                    logger.warning("auto_tune.no_innings_data unified empty")
                                    continue
                                report = run_auto_tune(
                                    "innings",
                                    all_X,
                                    all_Y,
                                    None,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    rescreen=args.rescreen,
                                    algorithms_explicitly_passed=bool(algorithms_override),
                                )
                                _maybe_save_tuned_params(args.go_app_url, "innings", None, report, args.api_key or None)
                                logger.info(
                                    "auto_tune.done model=innings format=unified n=%s best_cv_score=%s",
                                    all_X.shape[0],
                                    report["best_cv_score"],
                                )
                            else:
                                for fcode, lr in by_f.items():
                                    X, Y = lr.X, lr.Y
                                    if X.size == 0 or Y.size == 0:
                                        continue
                                    report = run_auto_tune(
                                        "innings",
                                        X,
                                        Y,
                                        fcode,
                                        out_dir,
                                        algorithms_override,
                                        validation_method_override,
                                        use_pycaret=use_pycaret,
                                        fast_mode=fast_mode,
                                        rescreen=args.rescreen,
                                        algorithms_explicitly_passed=bool(algorithms_override),
                                    )
                                    _maybe_save_tuned_params(
                                        args.go_app_url, "innings", fcode, report, args.api_key or None
                                    )
                                    logger.info(
                                        "auto_tune.done model=innings format=%s n=%s best_cv_score=%s",
                                        fcode,
                                        X.shape[0],
                                        report["best_cv_score"],
                                    )
                            continue
                        if model_kind == "batting":
                            if args.csv:
                                csv_path = args.csv
                            else:
                                csv_path = os.path.join(default_dir, f"batting_encoded_{fmt or 'all'}.csv")
                                if not os.path.isfile(csv_path):
                                    csv_path = os.path.join(default_dir, "batting_encoded_all.csv")
                            _lr: Optional[LoaderResult] = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_batting_csv(csv_path),
                                lambda: load_batting_from_api(
                                    args.go_app_url, fmt or "all", args.cutoff, args.api_key or None
                                ),
                                can_fallback_to_api=api_available,
                            )
                            if _lr is None:
                                continue
                            X, Y, feat_names = _lr.X, _lr.Y, _lr.feature_names
                        elif model_kind == "bowling":
                            if args.csv:
                                csv_path = args.csv
                            else:
                                csv_path = os.path.join(default_dir, f"bowling_encoded_{fmt or 'all'}.csv")
                                if not os.path.isfile(csv_path):
                                    csv_path = os.path.join(default_dir, "bowling_encoded_all.csv")
                            _lr = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_bowling_csv(csv_path),
                                lambda: load_bowling_from_api(
                                    args.go_app_url, fmt or "all", args.cutoff, args.api_key or None
                                ),
                                can_fallback_to_api=api_available,
                            )
                            if _lr is None:
                                continue
                            X, Y, feat_names = _lr.X, _lr.Y, _lr.feature_names
                        else:
                            if args.csv:
                                csv_path = args.csv
                            else:
                                csv_path = os.path.join(default_dir, f"fielding_encoded_{fmt or 'all'}.csv")
                                if not os.path.isfile(csv_path):
                                    csv_path = os.path.join(default_dir, "fielding_encoded_all.csv")
                            by_f = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_fielding_csv(csv_path, fmt),
                                lambda: load_fielding_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt),
                                can_fallback_to_api=api_available,
                            )
                            if not by_f:
                                logger.warning("auto_tune.no_fielding_data format=%s", fmt)
                                continue
                            if args.unified:
                                all_X = np.vstack([lr.X for lr in by_f.values()])
                                all_Y = np.vstack([lr.Y for lr in by_f.values()])
                                if all_X.size == 0 or all_Y.size == 0:
                                    logger.warning("auto_tune.no_fielding_data unified empty")
                                    continue
                                report = run_auto_tune(
                                    model_kind,
                                    all_X,
                                    all_Y,
                                    None,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    rescreen=args.rescreen,
                                    algorithms_explicitly_passed=bool(algorithms_override),
                                )
                                _maybe_save_tuned_params(
                                    args.go_app_url, model_kind, None, report, args.api_key or None
                                )
                                logger.info(
                                    "auto_tune.done model=%s format=unified n=%s best_cv_score=%s",
                                    model_kind,
                                    all_X.shape[0],
                                    report["best_cv_score"],
                                )
                            else:
                                for fcode, lr in by_f.items():
                                    X, Y = lr.X, lr.Y
                                    if X.size == 0 or Y.size == 0:
                                        continue
                                    report = run_auto_tune(
                                        model_kind,
                                        X,
                                        Y,
                                        fcode,
                                        out_dir,
                                        algorithms_override,
                                        validation_method_override,
                                        use_pycaret=use_pycaret,
                                        fast_mode=fast_mode,
                                        rescreen=args.rescreen,
                                        algorithms_explicitly_passed=bool(algorithms_override),
                                    )
                                    _maybe_save_tuned_params(
                                        args.go_app_url, model_kind, fcode, report, args.api_key or None
                                    )
                                    logger.info(
                                        "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                                        model_kind,
                                        fcode,
                                        X.shape[0],
                                        report["best_cv_score"],
                                    )
                            continue
                    except (ValueError, RuntimeError) as e:
                        logger.error("auto_tune.from_api_load_failed model=%s format=%s error=%s", model_kind, fmt, e)
                        raise SystemExit(1) from e
                    if X.size == 0 or Y.size == 0:
                        logger.warning("auto_tune.no_data model=%s format=%s", model_kind, fmt)
                        continue
                    report = run_auto_tune(
                        model_kind,
                        X,
                        Y,
                        format_suffix,
                        out_dir,
                        algorithms_override,
                        validation_method_override,
                        use_pycaret=use_pycaret,
                        fast_mode=fast_mode,
                        rescreen=args.rescreen,
                        algorithms_explicitly_passed=bool(algorithms_override),
                        feature_names=feat_names,
                    )
                    _maybe_save_tuned_params(args.go_app_url, model_kind, format_suffix, report, args.api_key or None)
                    logger.info(
                        "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                        model_kind,
                        format_suffix,
                        X.shape[0],
                        report["best_cv_score"],
                    )
                else:
                    # Prefer CSV; fallback to API with warning when CSV not found
                    default_dir = os.environ.get("GO_APP_OUTPUT_DIR", default_go_app_export_dir())
                    api_available = bool(args.go_app_url and args.cutoff)
                    if model_kind == "extras":
                        csv_path = args.csv or os.path.join(default_dir, "extras_encoded_all.csv")
                        try:
                            by_f = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_extras_csv(csv_path, fmt),
                                lambda: load_extras_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt),
                                can_fallback_to_api=api_available,
                            )
                        except (ValueError, RuntimeError, FileNotFoundError) as e:
                            logger.error("auto_tune.load_extras_failed path=%s error=%s", csv_path, e)
                            continue
                        if by_f is None:
                            continue
                        if not by_f:
                            logger.warning("auto_tune.no_extras_data format=%s", fmt)
                            continue
                        legacy_pack = _extras_pop_legacy(by_f)
                        if args.unified:
                            if legacy_pack is not None:
                                all_X, all_Y = legacy_pack.X, legacy_pack.Y
                                unified_feature_names = legacy_pack.feature_names
                            else:
                                all_X = np.vstack([lr.X for lr in by_f.values()])
                                all_Y = np.vstack([lr.Y for lr in by_f.values()])
                                unified_feature_names = _shared_feature_names(by_f)
                            if all_X.size == 0 or all_Y.size == 0:
                                logger.warning("auto_tune.no_extras_data unified empty")
                                continue
                            report = run_auto_tune_extras(
                                all_X,
                                all_Y,
                                None,
                                out_dir,
                                algorithms_override,
                                validation_method_override,
                                use_pycaret=use_pycaret,
                                fast_mode=fast_mode,
                                use_autogluon=use_autogluon,
                                rescreen=args.rescreen,
                                algorithms_explicitly_passed=bool(algorithms_override),
                                feature_names=unified_feature_names,
                            )
                            _maybe_save_tuned_params(args.go_app_url, "extras", None, report, args.api_key or None)
                            logger.info(
                                "auto_tune.done model=extras format=unified n=%s best_cv_score=%s",
                                all_X.shape[0],
                                report["best_cv_score"],
                            )
                        else:
                            for fcode, pack in by_f.items():
                                X, Y = pack.X, pack.Y
                                if X.size == 0 or Y.size == 0:
                                    continue
                                report = run_auto_tune_extras(
                                    X,
                                    Y,
                                    fcode,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    use_autogluon=use_autogluon,
                                    rescreen=args.rescreen,
                                    algorithms_explicitly_passed=bool(algorithms_override),
                                    feature_names=pack.feature_names,
                                )
                                _maybe_save_tuned_params(args.go_app_url, "extras", fcode, report, args.api_key or None)
                                logger.info(
                                    "auto_tune.done model=extras format=%s n=%s best_cv_score=%s",
                                    fcode,
                                    X.shape[0],
                                    report["best_cv_score"],
                                )
                        continue
                    if model_kind == "win":
                        csv_path = args.csv or os.path.join(default_dir, "win_encoded_all.csv")
                        try:
                            by_f = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_win_csv(csv_path, fmt),
                                lambda: load_win_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt),
                                can_fallback_to_api=api_available,
                            )
                        except (ValueError, RuntimeError, FileNotFoundError) as e:
                            logger.error("auto_tune.load_win_failed path=%s error=%s", csv_path, e)
                            continue
                        if by_f is None:
                            continue
                        if not by_f:
                            logger.warning("auto_tune.no_win_data format=%s", fmt)
                            continue
                        if args.unified:
                            all_X = np.vstack([lr.X for lr in by_f.values()])
                            all_Y = np.concatenate([lr.Y.ravel() for lr in by_f.values()])
                            if all_X.size == 0 or all_Y.size == 0:
                                logger.warning("auto_tune.no_win_data unified empty")
                                continue
                            report = run_auto_tune_win(
                                all_X,
                                all_Y,
                                None,
                                out_dir,
                                algorithms_override,
                                validation_method_override,
                                use_pycaret=use_pycaret,
                                fast_mode=fast_mode,
                                use_autogluon=use_autogluon,
                                rescreen=args.rescreen,
                                algorithms_explicitly_passed=bool(algorithms_override),
                            )
                            _maybe_save_tuned_params(args.go_app_url, "win", None, report, args.api_key or None)
                            logger.info(
                                "auto_tune.done model=win format=unified n=%s best_cv_score=%s",
                                all_X.shape[0],
                                report["best_cv_score"],
                            )
                        else:
                            for fcode, lr in by_f.items():
                                X, Y = lr.X, lr.Y
                                if X.size == 0 or Y.size == 0:
                                    continue
                                report = run_auto_tune_win(
                                    X,
                                    Y,
                                    fcode,
                                    out_dir,
                                    algorithms_override,
                                    validation_method_override,
                                    use_pycaret=use_pycaret,
                                    fast_mode=fast_mode,
                                    use_autogluon=use_autogluon,
                                    rescreen=args.rescreen,
                                    algorithms_explicitly_passed=bool(algorithms_override),
                                )
                                _maybe_save_tuned_params(args.go_app_url, "win", fcode, report, args.api_key or None)
                                logger.info(
                                    "auto_tune.done model=win format=%s n=%s best_cv_score=%s",
                                    fcode,
                                    X.shape[0],
                                    report["best_cv_score"],
                                )
                        continue
                    if model_kind == "fielding":
                        csv_path = args.csv or os.path.join(default_dir, f"fielding_encoded_{fmt or 'ALL'}.csv")
                        if not os.path.isfile(csv_path) and not args.csv:
                            csv_path = os.path.join(default_dir, "fielding_encoded_all.csv")
                        try:
                            by_f = _load_via_csv_or_api(
                                csv_path,
                                lambda: load_fielding_csv(csv_path, fmt),
                                lambda: load_fielding_from_api(args.go_app_url, args.cutoff, args.api_key or None, fmt),
                                can_fallback_to_api=api_available,
                            )
                        except (ValueError, RuntimeError, FileNotFoundError) as e:
                            logger.error("auto_tune.load_fielding_failed path=%s error=%s", csv_path, e)
                            continue
                        if by_f is None:
                            continue
                        for fcode, lr in by_f.items():
                            X, Y = lr.X, lr.Y
                            if X.size == 0 or Y.size == 0:
                                continue
                            report = run_auto_tune(
                                model_kind,
                                X,
                                Y,
                                fcode,
                                out_dir,
                                algorithms_override,
                                validation_method_override,
                                use_pycaret=use_pycaret,
                                fast_mode=fast_mode,
                                rescreen=args.rescreen,
                                algorithms_explicitly_passed=bool(algorithms_override),
                            )
                            _maybe_save_tuned_params(args.go_app_url, model_kind, fcode, report, args.api_key or None)
                            logger.info(
                                "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                                model_kind,
                                fcode,
                                X.shape[0],
                                report["best_cv_score"],
                            )
                        continue
                    csv_path = args.csv or os.path.join(default_dir, f"{model_kind}_encoded_{fmt or 'LEGACY'}.csv")
                    if not os.path.isfile(csv_path) and not args.csv:
                        csv_path = os.path.join(default_dir, f"{model_kind}_encoded.csv")
                    if model_kind == "batting":
                        load_csv = lambda: load_batting_csv(csv_path)
                        load_api = lambda: load_batting_from_api(
                            args.go_app_url, fmt or "all", args.cutoff, args.api_key or None
                        )
                    else:
                        load_csv = lambda: load_bowling_csv(csv_path)
                        load_api = lambda: load_bowling_from_api(
                            args.go_app_url, fmt or "all", args.cutoff, args.api_key or None
                        )
                    try:
                        result = _load_via_csv_or_api(
                            csv_path,
                            load_csv,
                            load_api,
                            can_fallback_to_api=api_available,
                        )
                    except (ValueError, RuntimeError, Exception) as e:
                        logger.error("auto_tune.load_failed model=%s path=%s error=%s", model_kind, csv_path, e)
                        continue
                    if result is None:
                        continue
                    X, Y, csv_feat_names = result.X, result.Y, result.feature_names
                    if X.size == 0 or Y.size == 0:
                        logger.warning("auto_tune.no_data_in_csv path=%s", csv_path)
                        continue
                    report = run_auto_tune(
                        model_kind,
                        X,
                        Y,
                        format_suffix,
                        out_dir,
                        algorithms_override,
                        validation_method_override,
                        use_pycaret=use_pycaret,
                        fast_mode=fast_mode,
                        rescreen=args.rescreen,
                        algorithms_explicitly_passed=bool(algorithms_override),
                        feature_names=csv_feat_names,
                    )
                    _maybe_save_tuned_params(args.go_app_url, model_kind, format_suffix, report, args.api_key or None)
                    logger.info(
                        "auto_tune.done model=%s format=%s n=%s best_cv_score=%s",
                        model_kind,
                        format_suffix,
                        X.shape[0],
                        report["best_cv_score"],
                    )
    finally:
        _progress.clear_progress()


if __name__ == "__main__":
    main()
