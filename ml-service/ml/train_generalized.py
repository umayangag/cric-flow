"""
Train generalized ML pipeline (Player Performance / Match Outcome).

Loads ball-by-ball data from DB or CSV, applies era normalization and feature
engineering, compares LogReg/XGBoost/LightGBM with walk-forward validation.

Usage:
  python -m ml.train_generalized --task player_performance --from-db --cutoff 2024-12-01
  python -m ml.train_generalized --task match_outcome --from-db --cutoff 2024-12-01
  python -m ml.train_generalized --csv path/to/ball_by_ball.csv --task player_performance
"""

from __future__ import annotations

import argparse
import json
import logging
import os
from datetime import date

from ml.ball_by_ball_loader import load_ball_by_ball_from_csv, load_ball_by_ball_from_db
from ml.config import default_artifacts_dir
from ml.generalized_pipeline import (
    PipelineConfig,
    run_generalized_pipeline,
)

logger = logging.getLogger(__name__)


def main() -> None:
    if not logging.getLogger().handlers:
        logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")

    ap = argparse.ArgumentParser(description="Train generalized cricket ML pipeline")
    ap.add_argument("--task", default="player_performance", choices=["player_performance", "match_outcome"])
    ap.add_argument("--csv", default="", help="Path to ball-by-ball CSV")
    ap.add_argument("--from-db", action="store_true", help="Load from PostgreSQL")
    ap.add_argument("--cutoff", default="", help="Cutoff date (YYYY-MM-DD) for DB load")
    ap.add_argument("--format", default="", help="Format filter (ODI,T20I) for DB")
    ap.add_argument("--out", default="", help="Output dir for artifacts")
    ap.add_argument("--delta-threshold", type=float, default=0.08, help="Max train-val delta")
    ap.add_argument("--n-splits", type=int, default=5, help="Time-series CV splits")
    args = ap.parse_args()

    out_dir = args.out or os.environ.get("ML_SERVICE_OUTPUT_DIR") or default_artifacts_dir()

    if args.csv:
        df = load_ball_by_ball_from_csv(args.csv)
    elif args.from_db:
        cutoff = None
        if args.cutoff:
            cutoff = date.fromisoformat(args.cutoff)
        formats = [s.strip() for s in args.format.split(",")] if args.format else None
        df = load_ball_by_ball_from_db(cutoff_date=cutoff, format_codes=formats)
    else:
        logger.error("Provide --csv or --from-db")
        raise SystemExit(1)

    if df.empty or len(df) < 100:
        logger.error("Insufficient data: %d rows", len(df))
        raise SystemExit(1)

    config = PipelineConfig(
        task=args.task,
        n_splits=args.n_splits,
        delta_threshold=args.delta_threshold,
    )

    result = run_generalized_pipeline(df, task=args.task, config=config)

    if "error" in result:
        logger.error("Pipeline failed: %s", result["error"])
        raise SystemExit(1)

    # Save artifacts
    os.makedirs(out_dir, exist_ok=True)
    import joblib

    pipe = result["pipeline"]
    suffix = args.task.replace("_", "-")
    joblib.dump(pipe, os.path.join(out_dir, f"generalized_pipeline_{suffix}.joblib"), compress=3)

    meta = {
        "task": args.task,
        "best_model": result["best_model"],
        "best_metrics": result["best_metrics"],
        "summary": result["summary"],
        "rows": len(df),
        "feature_names": pipe.feature_names_,
    }
    meta_path = os.path.join(out_dir, f"generalized_pipeline_{suffix}_metadata.json")
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(meta, f, indent=2)

    logger.info(
        "train_generalized.saved task=%s best=%s val_metric=%.4f delta=%.4f",
        args.task,
        result["best_model"],
        result["best_metrics"]["val_metric"],
        result["best_metrics"]["delta"],
    )


if __name__ == "__main__":
    main()
