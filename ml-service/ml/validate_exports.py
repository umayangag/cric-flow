import argparse
import json
import os
from typing import List, Tuple

import pandas as pd

# Simple validator for exported CSVs from go-app/cmd/export-dataset
# Checks that required columns exist, files are present for requested formats,
# and NaN rates are within acceptable thresholds.
#
# Usage:
#   python ml/validate_exports.py --formats ODI,T20I --dir ../../output/go-app
#   python ml/validate_exports.py --all-formats


BATTING_REQUIRED = [
    "runs",
    "balls",
    "fours",
    "sixes",
    "batting_position",
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

BOWLING_REQUIRED = [
    "runs",
    "balls",
    "wickets",
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


def _config_formats() -> List[str]:
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f)
            fmts = data.get("ml", {}).get("formats") or []
            return [str(x).upper() for x in fmts if isinstance(x, (str, int))]
    except Exception:
        return []


def load_csv(path: str) -> pd.DataFrame:
    return pd.read_csv(path)


def validate_df(
    df: pd.DataFrame,
    required_cols: List[str],
    null_threshold: float = 0.2,
) -> Tuple[bool, List[str]]:
    problems: List[str] = []
    # Column presence
    missing = [c for c in required_cols if c not in df.columns]
    if missing:
        problems.append(f"missing columns: {missing}")
    # NaN rate
    if len(df) > 0:
        frac_null = df[required_cols].isna().mean(numeric_only=False)
        bad = {k: float(v) for k, v in frac_null.items() if k in required_cols and float(v) > null_threshold}
        if bad:
            problems.append(f"high NaN rates: {bad}")
    ok = len(problems) == 0
    return ok, problems


def main():
    parser = argparse.ArgumentParser()
    default_dir = os.environ.get("GO_APP_OUTPUT_DIR", os.path.join("..", "..", "output", "go-app"))
    parser.add_argument(
        "--dir",
        default=default_dir,
        help="Directory containing exported CSVs",
    )
    parser.add_argument(
        "--format",
        default="",
        help="Single format code",
    )
    parser.add_argument(
        "--formats",
        default="",
        help="Comma-separated formats list",
    )
    parser.add_argument(
        "--all-formats",
        action="store_true",
        help="Read formats from config.json (ml.formats)",
    )
    parser.add_argument(
        "--null-threshold",
        type=float,
        default=0.2,
        help="Max allowed NaN fraction per column",
    )
    args = parser.parse_args()

    targets: List[str] = []
    if args.all_formats:
        targets = _config_formats()
    elif args.formats:
        targets = [s.strip().upper() for s in args.formats.split(",") if s.strip()]
    elif args.format:
        targets = [args.format.strip().upper()]

    # Legacy validation (no format): optional
    if not targets:
        # Validate legacy files if present
        legacy_bat = os.path.join(args.dir, "batting_encoded.csv")
        legacy_bow = os.path.join(args.dir, "bowling_encoded.csv")
        failed = False
        if os.path.exists(legacy_bat):
            df = load_csv(legacy_bat)
            ok, probs = validate_df(df, BATTING_REQUIRED, args.null_threshold)
            if not ok:
                failed = True
                print(f"[LEGACY] batting_encoded.csv invalid: {probs}")
        if os.path.exists(legacy_bow):
            df = load_csv(legacy_bow)
            ok, probs = validate_df(df, BOWLING_REQUIRED, args.null_threshold)
            if not ok:
                failed = True
                print(f"[LEGACY] bowling_encoded.csv invalid: {probs}")
        if failed:
            raise SystemExit(2)
        print("legacy exports validated (if present)")
        return

    # Per-format validation
    any_failed = False
    for fmt in targets:
        bat = os.path.join(args.dir, f"batting_encoded_{fmt}.csv")
        bow = os.path.join(args.dir, f"bowling_encoded_{fmt}.csv")
        if not os.path.exists(bat):
            print(f"[{fmt}] missing file: {bat}")
            any_failed = True
            continue
        if not os.path.exists(bow):
            print(f"[{fmt}] missing file: {bow}")
            any_failed = True
            continue
        # Load and validate
        bdf = load_csv(bat)
        if len(bdf) == 0:
            print(f"[{fmt}] batting CSV empty: {bat}")
            any_failed = True
        ok, probs = validate_df(bdf, BATTING_REQUIRED, args.null_threshold)
        if not ok:
            any_failed = True
            print(f"[{fmt}] batting CSV invalid: {probs}")

        wdf = load_csv(bow)
        if len(wdf) == 0:
            print(f"[{fmt}] bowling CSV empty: {bow}")
            any_failed = True
        ok, probs = validate_df(wdf, BOWLING_REQUIRED, args.null_threshold)
        if not ok:
            any_failed = True
            print(f"[{fmt}] bowling CSV invalid: {probs}")

    if any_failed:
        raise SystemExit(2)
    print("per-format exports validated OK")


if __name__ == "__main__":
    main()
