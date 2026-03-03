"""
Unified training pipeline for player-level models (batting, bowling, fielding).

Eliminates duplication across train_batting.py, train_bowling.py, train_fielding.py
by extracting the shared structure into a single `TrainingPipeline` class driven by
a `ModelSpec` dataclass that captures per-model differences (feature cols, target cols,
seq cols, prepare logic, artifact prefix, API section name).

Each train_*.py becomes a thin wrapper: define ModelSpec → call TrainingPipeline.run().
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from typing import Any, Callable, Dict, List, Optional, Tuple

import joblib
import numpy as np
import pandas as pd
from sklearn.multioutput import MultiOutputRegressor

from . import config as svc_config
from .config import get_pipeline_common_config, get_training_params
from .data_quality import clip_target_outliers, impute_features
from .feature_transforms import apply_transforms, get_transform_config
from .pipeline_common import compute_time_decay_weights, get_scaler
from .utils import make_base_estimator

logger = logging.getLogger(__name__)


@dataclass
class ModelSpec:
    """Describes a model's columns, artifact naming, and data preparation hooks.

    Attributes:
        name: Model identifier used for config lookups and logging (e.g. "batting").
        feature_cols: Ordered list of feature column names (including seq cols).
        target_cols: Ordered list of target column names.
        seq_cols: Optional sequence columns (filled with 0 when absent in CSV).
        target_cols_share: Alternative target cols when --share-targets is used.
        artifact_prefix: Prefix for saved artifacts (e.g. "batting", "bowling").
        api_section: Section name in the training-data API response (e.g. "batting").
        target_names_for_clip: Target names for outlier clipping (may differ from target_cols).
        use_scaler: Whether to scale features before training.
        use_multi_output: Whether to wrap estimator in MultiOutputRegressor.
        prepare_dataframe: Optional hook to normalize/fill columns in the raw DataFrame.
            Signature: (df: DataFrame, spec: ModelSpec) -> DataFrame
        build_share_targets: Optional hook to compute share targets from raw DataFrame.
            Signature: (df: DataFrame, spec: ModelSpec) -> DataFrame
            Should add share target columns and filter invalid rows.
    """

    name: str
    feature_cols: List[str]
    target_cols: List[str]
    seq_cols: List[str] = field(default_factory=list)
    target_cols_share: Optional[List[str]] = None
    artifact_prefix: str = ""
    api_section: str = ""
    target_names_for_clip: Optional[List[str]] = None
    use_scaler: bool = True
    use_multi_output: bool = True
    prepare_dataframe: Optional[Callable[["pd.DataFrame", "ModelSpec"], "pd.DataFrame"]] = None
    build_share_targets: Optional[Callable[["pd.DataFrame", "ModelSpec"], "pd.DataFrame"]] = None

    def __post_init__(self):
        if not self.artifact_prefix:
            self.artifact_prefix = self.name
        if not self.api_section:
            self.api_section = self.name
        if self.target_names_for_clip is None:
            self.target_names_for_clip = list(self.target_cols)


class TrainingPipeline:
    """Unified training pipeline for player-level cricket models.

    Handles: CSV/API data loading, DataFrame preparation, feature/target extraction,
    imputation, transforms, scaling, model fitting, artifact saving, and metadata writing.
    The main() classmethod provides the full CLI entrypoint with format iteration.
    """

    def __init__(self, spec: ModelSpec):
        self.spec = spec

    # ── Data preparation ─────────────────────────────────────────────────

    def prepare_dataframe(self, df: pd.DataFrame) -> pd.DataFrame:
        """Apply model-specific DataFrame preparation, then fill missing seq cols."""
        if self.spec.prepare_dataframe is not None:
            df = self.spec.prepare_dataframe(df, self.spec)
        # Fill missing seq cols with 0
        for col in self.spec.seq_cols:
            if col not in df.columns:
                df[col] = 0.0
            else:
                df[col] = df[col].fillna(0.0)
        return df

    def dataframe_to_xy(
        self,
        df: pd.DataFrame,
        share_targets: bool = False,
    ) -> Tuple[np.ndarray, np.ndarray, List[str], Dict[str, float], Optional[np.ndarray]]:
        """Build X, Y, feature_names, imputation medians, and optional sample weights.

        When share_targets=True and spec.build_share_targets is defined, computes
        share-based targets instead of raw targets.
        """
        if share_targets:
            if self.spec.build_share_targets is None:
                raise ValueError(f"share_targets not supported for model '{self.spec.name}'")
            if self.spec.target_cols_share is None:
                raise ValueError(f"target_cols_share not defined for model '{self.spec.name}'")
            df = self.spec.build_share_targets(df, self.spec)
            target_cols = self.spec.target_cols_share
        else:
            target_cols = self.spec.target_cols

        # Drop rows missing essential targets
        target_subset = [c for c in target_cols if c in df.columns]
        if target_subset:
            df = df.dropna(subset=target_subset)

        # Impute features
        feature_cols_in_df = [c for c in self.spec.feature_cols if c in df.columns]
        df, medians = impute_features(df, feature_cols_in_df)

        # Ensure all feature columns exist
        for c in self.spec.feature_cols:
            if c not in df.columns:
                df[c] = 0.0

        X_raw = df[self.spec.feature_cols].astype(float).values

        # Apply feature transforms if configured
        transform_config = get_transform_config(self.spec.name)
        if transform_config.get("add_interactions") or transform_config.get("add_log1p"):
            X, feature_names_used = apply_transforms(
                X_raw, list(self.spec.feature_cols), transform_config, self.spec.name
            )
        else:
            X = X_raw
            feature_names_used = list(self.spec.feature_cols)

        # Build Y matrix, pad if needed
        y_cols = [c for c in target_cols if c in df.columns]
        Y = df[y_cols].astype(float).values
        needed = len(target_cols)
        if Y.shape[1] < needed:
            pad = np.zeros((Y.shape[0], needed - Y.shape[1]))
            Y = np.concatenate([Y, pad], axis=1)

        # Time-decay sample weights
        weights = None
        date_col = "match_date" if "match_date" in df.columns else None
        if date_col:
            pipe_cfg = get_pipeline_common_config()
            weights = compute_time_decay_weights(
                df[date_col],
                halflife_years=pipe_cfg.get("time_decay_halflife_years", 2.0),
            )

        return X, Y, feature_names_used, medians, weights

    # ── Dataset loading ──────────────────────────────────────────────────

    def load_dataset(
        self,
        path: str,
        share_targets: bool = False,
    ) -> Tuple[np.ndarray, np.ndarray, List[str], Dict[str, float], Optional[np.ndarray]]:
        """Load dataset from CSV file."""
        if not os.path.exists(path):
            logger.error(
                "training_pipeline.load_dataset.file_not_found model=%s path=%s",
                self.spec.name,
                path,
            )
            raise FileNotFoundError(path)
        df = pd.read_csv(path)
        df = self.prepare_dataframe(df)
        return self.dataframe_to_xy(df, share_targets=share_targets)

    def load_dataset_from_memory(
        self,
        headers: List[str],
        rows: List[List[str]],
        share_targets: bool = False,
    ) -> Tuple[np.ndarray, np.ndarray, List[str], Dict[str, float], Optional[np.ndarray]]:
        """Build X, Y from API-style (headers, rows)."""
        if not headers or not rows:
            tc = self.spec.target_cols_share if share_targets and self.spec.target_cols_share else self.spec.target_cols
            return (
                np.zeros((0, len(self.spec.feature_cols))),
                np.zeros((0, len(tc))),
                list(self.spec.feature_cols),
                {},
                None,
            )
        df = pd.DataFrame(rows, columns=headers)
        df = self.prepare_dataframe(df)
        return self.dataframe_to_xy(df, share_targets=share_targets)

    # ── API fetch ────────────────────────────────────────────────────────

    def fetch_from_api(
        self,
        go_app_url: str,
        format_code: str,
        cutoff_iso: str,
        api_key: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Fetch training data from go-app GET /api/backtest/training-data."""
        from .config import get_training_data_fetch_timeout_sec

        base = go_app_url.rstrip("/")
        section = self.spec.api_section
        url = (
            f"{base}/api/backtest/training-data"
            f"?format={urllib.parse.quote(format_code)}"
            f"&cutoff={urllib.parse.quote(cutoff_iso)}"
            f"&sections={urllib.parse.quote(section)}"
        )
        req = urllib.request.Request(url)
        if api_key:
            req.add_header("X-API-Key", api_key)
        try:
            with urllib.request.urlopen(req, timeout=get_training_data_fetch_timeout_sec()) as resp:
                data = json.loads(resp.read().decode())
        except urllib.error.HTTPError as e:
            body = e.read().decode() if e.fp else ""
            logger.error(
                "training_pipeline.fetch_from_api.http_error model=%s url=%s code=%s",
                self.spec.name,
                url,
                e.code,
            )
            raise ValueError(f"Go-app training-data failed: HTTP {e.code} {body}") from e
        except OSError as e:
            err_msg = str(e).strip()
            logger.error(
                "training_pipeline.fetch_from_api.os_error model=%s url=%s error=%s",
                self.spec.name,
                url,
                e,
            )
            hint = (
                "Go-app may have closed the connection before the response finished. "
                "Increase go-app server.http_write_timeout_sec in go-app/config.json."
            )
            if "closed connection" in err_msg.lower() or "without response" in err_msg.lower():
                raise ValueError(f"Go-app training-data request failed: {err_msg}. {hint}") from e
            raise ValueError(f"Go-app training-data request failed: {err_msg}") from e
        return data.get(section) or {"headers": [], "rows": []}

    # ── Training and saving ──────────────────────────────────────────────

    def train_and_save(
        self,
        X: np.ndarray,
        Y: np.ndarray,
        out_dir: str,
        training_params: dict,
        suffix: Optional[str] = None,
        metadata: Optional[dict] = None,
        transform_config: Optional[dict] = None,
        sample_weight: Optional[np.ndarray] = None,
        share_model: bool = False,
    ) -> None:
        """Train model, save artifacts (scaler + model + metadata)."""
        os.makedirs(out_dir, exist_ok=True)

        # Scale features
        if self.spec.use_scaler:
            pipe_cfg = get_pipeline_common_config()
            use_robust = pipe_cfg.get("use_robust_scaler", True)
            scaler = get_scaler(use_robust=use_robust)
            Xs = scaler.fit_transform(X)
        else:
            scaler = None
            Xs = X

        # Outlier clipping on targets
        clip_percentile = training_params.get("target_clip_percentile", 99.0)
        clip_names = self.spec.target_names_for_clip or self.spec.target_cols
        Y, clip_info = clip_target_outliers(
            Y, percentile=clip_percentile, target_names=clip_names[: Y.shape[1]]
        )

        # Build and fit model
        compress = training_params["joblib_compress"]
        base_est = make_base_estimator(training_params)
        if self.spec.use_multi_output:
            model = MultiOutputRegressor(base_est)
        else:
            model = base_est
        model.fit(Xs, Y, sample_weight=sample_weight)

        # Extract feature importance
        feature_names_for_importance = (metadata.get("feature_names") if metadata else None) or self.spec.feature_cols
        feature_importance = self._extract_feature_importance(model, feature_names_for_importance)

        # Save artifacts
        prefix = f"{self.spec.artifact_prefix}_share" if share_model else self.spec.artifact_prefix
        sfx = f"_{suffix}" if suffix else ""
        if scaler is not None:
            joblib.dump(scaler, os.path.join(out_dir, f"{prefix}_scaler{sfx}.joblib"), compress=compress)
        joblib.dump(model, os.path.join(out_dir, f"{prefix}_model{sfx}.joblib"), compress=compress)

        # Save metadata
        if metadata is not None:
            metadata["share_model"] = share_model
            if feature_importance is not None:
                metadata["feature_importance"] = feature_importance
            if transform_config:
                metadata["feature_transforms"] = transform_config
            if clip_info:
                metadata["target_clip_info"] = clip_info
            meta_path = os.path.join(out_dir, f"{prefix}_metadata_{suffix or 'LEGACY'}.json")
            try:
                with open(meta_path, "w", encoding="utf-8") as f:
                    json.dump(metadata, f, indent=2)
            except OSError as e:
                logger.warning(
                    "training_pipeline.train_and_save.metadata_save_failed model=%s path=%s error=%s",
                    self.spec.name,
                    meta_path,
                    e,
                )

    def train_in_memory(
        self,
        X: np.ndarray,
        Y: np.ndarray,
        format_code: Optional[str] = None,
    ) -> Tuple[Any, Any]:
        """Train model in memory and return (scaler, model). Used by train_on_the_fly."""
        params = get_training_params(self.spec.name, format_code)
        if self.spec.use_scaler:
            scaler = get_scaler(use_robust=False)
            Xs = scaler.fit_transform(X)
        else:
            scaler = None
            Xs = X
        base_est = make_base_estimator(params)
        if self.spec.use_multi_output:
            model = MultiOutputRegressor(base_est)
        else:
            model = base_est
        model.fit(Xs, Y)
        return scaler, model

    # ── CLI entrypoint ───────────────────────────────────────────────────

    @classmethod
    def run_cli(cls, spec: ModelSpec) -> None:
        """Full CLI entrypoint: parse args, resolve formats, train from CSV or API."""
        if not logging.getLogger().handlers:
            logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")

        pipeline = cls(spec)
        args = pipeline._parse_args()

        logger.info(
            "pipeline: train_%s starting out_dir=%s from_api=%s all_formats=%s",
            spec.name,
            args.out,
            args.from_api,
            args.all_formats,
        )

        targets = pipeline._resolve_targets(args)

        if args.from_api:
            pipeline._run_api_mode(args, targets)
            return

        default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())

        # Auto-detect formats when none explicitly provided
        if not targets and not args.csv:
            targets = pipeline._auto_detect_formats(default_csv_dir)

        # Legacy single CSV fallback
        if not targets:
            pipeline._run_legacy_csv(args, default_csv_dir)
            return

        # Per-format training loop
        pipeline._run_csv_format_loop(args, targets, default_csv_dir)

    # ── Private helpers ──────────────────────────────────────────────────

    def _parse_args(self) -> argparse.Namespace:
        default_csv_dir = os.environ.get("GO_APP_OUTPUT_DIR", svc_config.default_go_app_export_dir())
        default_out_dir = os.environ.get("ML_SERVICE_OUTPUT_DIR", svc_config.default_artifacts_dir())

        parser = argparse.ArgumentParser()
        parser.add_argument("--csv", default="", help="Path to CSV (overrides format-based resolution)")
        parser.add_argument("--out", default=default_out_dir, help="Output dir for artifacts")
        parser.add_argument("--format", default="", help="Single format code (e.g., ODI, T20I)")
        parser.add_argument("--formats", default="", help="Comma-separated list of formats")
        parser.add_argument("--all-formats", action="store_true", help="Train for all formats from config")
        parser.add_argument("--from-api", action="store_true", help="Fetch training data from go-app API")
        parser.add_argument("--cutoff", default="", help="RFC3339 cutoff for API fetch")
        parser.add_argument(
            "--go-app-url",
            default=os.environ.get("GO_APP_URL", ""),
            help="Go-app base URL for --from-api",
        )
        parser.add_argument("--share-targets", action="store_true", help="Train on share targets instead of raw")
        parser.add_argument(
            "--api-key",
            default=os.environ.get("GO_APP_API_KEY", ""),
            help="Optional API key for go-app",
        )
        return parser.parse_args()

    def _resolve_targets(self, args: argparse.Namespace) -> List[str]:
        if args.all_formats:
            return _config_formats()
        if args.formats:
            return [s.strip().upper() for s in args.formats.split(",") if s.strip()]
        if args.format:
            return [args.format.strip().upper()]
        return []

    def _auto_detect_formats(self, csv_dir: str) -> List[str]:
        prefix = f"{self.spec.artifact_prefix}_encoded_"
        cfg_fmts = _config_formats()
        existing = [
            f for f in cfg_fmts if os.path.exists(os.path.join(csv_dir, f"{prefix}{f}.csv"))
        ]
        if existing:
            return existing
        targets: List[str] = []
        try:
            for name in os.listdir(csv_dir):
                if name.startswith(prefix) and name.endswith(".csv"):
                    suffix = name[len(prefix) : -len(".csv")]
                    if suffix:
                        targets.append(str(suffix).upper())
        except Exception:
            pass
        return targets

    def _run_api_mode(self, args: argparse.Namespace, targets: List[str]) -> None:
        cutoff = (args.cutoff or "").strip()
        go_app_url = (args.go_app_url or os.environ.get("GO_APP_URL", "")).strip()
        if not cutoff or not go_app_url:
            logger.error(
                "train_%s.from_api_requires cutoff and go_app_url (or GO_APP_URL)",
                self.spec.name,
            )
            raise SystemExit(1)
        if not targets:
            targets = _config_formats()
        api_key = (args.api_key or os.environ.get("GO_APP_API_KEY", "")).strip() or None
        logger.info(
            "pipeline: train_%s fetching data from API formats=%s cutoff=%s",
            self.spec.name,
            targets,
            cutoff,
        )

        def _train_one_api(fmt: str) -> int:
            logger.info("pipeline: train_%s processing format=%s", self.spec.name, fmt)
            data = self.fetch_from_api(go_app_url, fmt, cutoff, api_key)
            headers = data.get("headers") or []
            rows = data.get("rows") or []
            if not headers or not rows:
                logger.warning("train_%s.skip_format_no_data_from_api format=%s", self.spec.name, fmt)
                return 0
            try:
                X, Y, feature_names_used, medians, weights = self.load_dataset_from_memory(
                    headers, rows, share_targets=args.share_targets
                )
            except Exception as e:
                logger.error("train_%s.load_from_api_failed format=%s error=%s", self.spec.name, fmt, e)
                return 0
            if X.size == 0 or Y.size == 0:
                logger.warning("train_%s.skip_format_no_data format=%s", self.spec.name, fmt)
                return 0
            training_params = get_training_params(self.spec.name, fmt)
            transform_config = get_transform_config(self.spec.name)
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
                "imputation_medians": medians,
            }
            self.train_and_save(
                X, Y, args.out, training_params, fmt, meta, transform_config,
                sample_weight=weights, share_model=args.share_targets,
            )
            logger.info(
                "train_%s.saved_format format=%s out_dir=%s rows=%s",
                self.spec.name, fmt, args.out, int(X.shape[0]),
            )
            return 1

        saved_count = _run_concurrent(_train_one_api, targets)
        if targets and saved_count == 0:
            logger.error("train_%s.no_models_saved from_api=True cutoff=%s", self.spec.name, cutoff)
            raise SystemExit(1)

    def _run_legacy_csv(self, args: argparse.Namespace, csv_dir: str) -> None:
        training_params = get_training_params(self.spec.name, None)
        csv_path = args.csv or os.path.join(csv_dir, f"{self.spec.artifact_prefix}_encoded.csv")
        try:
            X, Y, feature_names_used, medians, weights = self.load_dataset(
                csv_path, share_targets=args.share_targets
            )
        except FileNotFoundError as e:
            logger.error("train_%s.legacy_csv_not_found path=%s error=%s", self.spec.name, csv_path, e)
            raise SystemExit(1) from e
        if X.size == 0 or Y.size == 0:
            logger.error("train_%s.no_data path=%s", self.spec.name, csv_path)
            return
        transform_config = get_transform_config(self.spec.name)
        meta = {
            "csv_path": csv_path,
            "rows": int(X.shape[0]),
            "n_features": int(X.shape[1]),
            "n_targets": int(Y.shape[1]),
            "format": None,
            "model": "RandomForestRegressor",
            "hyperparams": training_params,
            "feature_names": feature_names_used,
            "imputation_medians": medians,
        }
        self.train_and_save(
            X, Y, args.out, training_params, None, meta, transform_config,
            sample_weight=weights, share_model=args.share_targets,
        )
        logger.info("train_%s.saved_legacy out_dir=%s", self.spec.name, args.out)

    def _run_csv_format_loop(
        self, args: argparse.Namespace, targets: List[str], csv_dir: str
    ) -> None:
        logger.info(
            "pipeline: train_%s loading from CSV formats=%s csv_dir=%s",
            self.spec.name, targets, csv_dir,
        )

        def _train_one_csv(fmt: str) -> int:
            logger.info("pipeline: train_%s processing format=%s (CSV)", self.spec.name, fmt)
            training_params = get_training_params(self.spec.name, fmt)
            csv_path = args.csv or os.path.join(csv_dir, f"{self.spec.artifact_prefix}_encoded_{fmt}.csv")
            if not os.path.exists(csv_path):
                logger.warning(
                    "train_%s.skip_format_csv_not_found format=%s path=%s",
                    self.spec.name, fmt, csv_path,
                )
                return 0
            try:
                X, Y, feature_names_used, medians, weights = self.load_dataset(
                    csv_path, share_targets=args.share_targets
                )
            except Exception as e:
                logger.error(
                    "train_%s.load_dataset_failed format=%s path=%s error=%s",
                    self.spec.name, fmt, csv_path, e,
                )
                return 0
            if X.size == 0 or Y.size == 0:
                logger.warning(
                    "train_%s.skip_format_no_data format=%s path=%s",
                    self.spec.name, fmt, csv_path,
                )
                return 0
            transform_config = get_transform_config(self.spec.name)
            meta = {
                "csv_path": csv_path,
                "rows": int(X.shape[0]),
                "n_features": int(X.shape[1]),
                "n_targets": int(Y.shape[1]),
                "format": fmt,
                "model": "RandomForestRegressor",
                "hyperparams": training_params,
                "feature_names": feature_names_used,
                "imputation_medians": medians,
            }
            self.train_and_save(
                X, Y, args.out, training_params, fmt, meta, transform_config,
                sample_weight=weights, share_model=args.share_targets,
            )
            logger.info(
                "train_%s.saved_format format=%s out_dir=%s rows=%s",
                self.spec.name, fmt, args.out, int(X.shape[0]),
            )
            return 1

        saved_count = _run_concurrent(_train_one_csv, targets)
        if targets and saved_count == 0:
            logger.error(
                "train_%s.no_models_saved csv_dir=%s out_dir=%s",
                self.spec.name, csv_dir, args.out,
            )
            raise SystemExit(1)

    @staticmethod
    def _extract_feature_importance(
        model: Any, feature_names: List[str]
    ) -> Optional[Dict[str, float]]:
        """Extract average feature importance from a (Multi)OutputRegressor."""
        estimators = getattr(model, "estimators_", None)
        if not estimators:
            # Single estimator (not MultiOutput)
            if hasattr(model, "feature_importances_"):
                n_f = min(len(feature_names), len(model.feature_importances_))
                importance = {feature_names[i]: float(model.feature_importances_[i]) for i in range(n_f)}
                top = sorted(importance.items(), key=lambda x: -x[1])[:5]
                logger.info("training_pipeline.feature_importance_top5 %s", top)
                return importance
            return None
        imps = []
        for est in estimators:
            if hasattr(est, "feature_importances_"):
                imps.append(est.feature_importances_)
        if not imps:
            return None
        n_f = min(len(feature_names), len(imps[0]))
        importance = {
            feature_names[i]: float(np.mean([arr[i] for arr in imps])) for i in range(n_f)
        }
        top = sorted(importance.items(), key=lambda x: -x[1])[:5]
        logger.info("training_pipeline.feature_importance_top5 %s", top)
        return importance


# ── Module-level helpers ─────────────────────────────────────────────────


def _config_formats() -> List[str]:
    """Read ml.formats from ml-service/config.json."""
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f)
            fmts = data.get("ml", {}).get("formats") or []
            return [str(x).upper() for x in fmts if isinstance(x, (str, int))]
    except Exception:
        return []


def _run_concurrent(fn: Callable[[str], int], targets: List[str]) -> int:
    """Run fn concurrently over targets, return total count of successes."""
    if not targets:
        return 0
    max_workers = min(
        len(targets),
        max(1, int(os.environ.get("ML_TRAIN_FORMAT_WORKERS", "4"))),
    )
    saved_count = 0
    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        futures = {executor.submit(fn, fmt): fmt for fmt in targets}
        for fut in as_completed(futures):
            try:
                saved_count += fut.result()
            except Exception:
                raise
    return saved_count
