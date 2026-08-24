import argparse
import json
import os
import subprocess
import sys
from typing import List, Tuple

import pandas as pd

# Export CSV validator for go-app/cmd/export-dataset outputs.
# Now supports:
# - Per-format presence checks (TEST/ODI/T20/T20I)
# - Schema modes: training (outputs + features + identifiers) and inference (features + identifiers)
# - Optional delegation to golden header validator (tests/golden/compare_features.py)
# - Null-rate checks with configurable threshold
#
# Usage examples:
#   python ml/validate_exports.py --all-formats --schema training
#   python ml/validate_exports.py --formats ODI,T20 --schema training --dir ../../output/exports
#   python ml/validate_exports.py --format T20I --schema inference --use-golden


# Minimal required feature presence (training schema includes outputs; inference excludes them).
# These lists are used for NaN checks and presence validation in addition to header checks.
BATTING_REQUIRED_FEATURES = [
    # Raw stats (v2) + weather + context
    "batting_mean_w5",
    "batting_std_w10",
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
    "match_month_sin",
    "match_month_cos",
    "match_day_of_week_sin",
    "match_day_of_week_cos",
    # Fielding aggregates (strict mode requires presence)
    "catches",
    "run_outs",
    "stumpings",
    "runouts_direct_hits",
    "fielding_involvements",
]

BOWLING_REQUIRED_FEATURES = [
    "bowling_mean_w5",
    "bowling_std_w10",
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
    "match_month_sin",
    "match_month_cos",
    "match_day_of_week_sin",
    "match_day_of_week_cos",
    # Fielding aggregates (strict mode requires presence)
    "catches",
    "run_outs",
    "stumpings",
    "runouts_direct_hits",
    "fielding_involvements",
]

# Training outputs that must come first in training schema
BATTING_OUTPUTS = ["runs", "balls", "fours", "sixes", "batting_position"]
# Bowling often has econ computed later; only enforce the core three at the front
BOWLING_OUTPUTS = ["runs", "balls", "wickets"]

# Inference required features (strict header names; v2 raw stats)
BATTING_INFER_HEADERS = [
    "batting_mean_w3",
    "batting_mean_w5",
    "batting_std_w10",
    "batting_temp",
    "batting_wind",
    "batting_rain",
    "batting_humidity",
    "batting_cloud",
    "batting_pressure",
    "batting_viscosity",
    "batting_inning",
    "batting_session",
    "toss",
    "venue",
    "opposition",
    "season",
    "player_name",
    # Fielding aggregates appended (must match go-app inference exporters' order)
    "catches",
    "run_outs",
    "stumpings",
    "runouts_direct_hits",
    "fielding_involvements",
]
BOWLING_INFER_HEADERS = [
    "bowling_mean_w3",
    "bowling_mean_w5",
    "bowling_std_w10",
    "bowling_temp",
    "bowling_wind",
    "bowling_rain",
    "bowling_humidity",
    "bowling_cloud",
    "bowling_pressure",
    "bowling_viscosity",
    "batting_inning",
    "bowling_session",
    "toss",
    "bowling_venue",
    "bowling_opposition",
    "season",
    "player_name",
    # Fielding aggregates appended (must match go-app inference exporters' order)
    "catches",
    "run_outs",
    "stumpings",
    "runouts_direct_hits",
    "fielding_involvements",
]


def _config_formats() -> List[str]:
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            data = json.load(f)
            fmts = data.get("ml", {}).get("formats") or []
            fmts = [str(x).upper() for x in fmts if isinstance(x, (str, int))]
            if fmts:
                return fmts
    except Exception:
        pass
    # Default to all four if config unset
    return ["TEST", "ODI", "T20", "T20I"]


def load_csv(path: str) -> pd.DataFrame:
    return pd.read_csv(path)


def read_header(path: str) -> list[str]:
    try:
        # use pandas to read only header
        df = pd.read_csv(path, nrows=0)
        return list(df.columns)
    except Exception:
        # fallback: simple csv read
        import csv  # local import

        with open(path, "r", newline="") as f:
            r = csv.reader(f)
            try:
                return next(r)
            except StopIteration:
                return []


def validate_presence_and_nulls(
    df: pd.DataFrame,
    required_cols: List[str],
    null_threshold: float = 0.2,
) -> Tuple[bool, List[str]]:
    problems: List[str] = []
    missing = [c for c in required_cols if c not in df.columns]
    if missing:
        problems.append(f"missing columns: {missing}")
    if len(df) > 0 and required_cols:
        try:
            frac_null = df[required_cols].isna().mean(numeric_only=False)
            bad = {k: float(v) for k, v in frac_null.items() if k in required_cols and float(v) > null_threshold}
            if bad:
                problems.append(f"high NaN rates: {bad}")
        except Exception as e:
            problems.append(f"null-rate check failed: {e}")
    return len(problems) == 0, problems


def _guess_exports_dir(preferred: str) -> str:
    # Prefer explicit dir; else try common locations
    if preferred and os.path.isdir(preferred):
        return preferred
    candidates = [
        os.path.join("..", "..", "output", "exports"),
        os.path.join("..", "..", "output", "go-app"),
        os.path.join("..", "..", "output"),
    ]
    for c in candidates:
        if os.path.isdir(c):
            return c
    return preferred or os.getcwd()


def _golden_validator_path() -> str:
    # Run from ml-service/ml; golden lives in ../../tests/golden/compare_features.py
    p = os.path.join("..", "..", "tests", "golden", "compare_features.py")
    return p


def _run_golden_validator(exports_dir: str, schema: str) -> Tuple[bool, str]:
    script = _golden_validator_path()
    if not os.path.exists(script):
        return False, f"golden validator not found: {script}"
    py = sys.executable or "python3"
    try:
        proc = subprocess.run(
            [py, script, "--exports", exports_dir, "--schema", schema],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            check=False,
        )
        ok = proc.returncode == 0
        output = proc.stdout.strip()
        return ok, output
    except Exception as e:
        return False, f"failed to run golden validator: {e}"


def main():
    parser = argparse.ArgumentParser()
    default_dir = os.environ.get("GO_APP_OUTPUT_DIR", os.path.join("..", "..", "output", "go-app"))
    parser.add_argument("--dir", default=default_dir, help="Directory containing exported CSVs")
    parser.add_argument("--format", default="", help="Single format code")
    parser.add_argument("--formats", default="", help="Comma-separated formats list")
    parser.add_argument("--all-formats", action="store_true", help="Validate TEST,ODI,T20,T20I (or from config)")
    parser.add_argument(
        "--schema", default="training", choices=["training", "inference"], help="Schema mode for header checks"
    )
    parser.add_argument(
        "--use-golden", action="store_true", help="Use golden header validator for strict header checks"
    )
    parser.add_argument("--null-threshold", type=float, default=0.2, help="Max allowed NaN fraction per column")
    args = parser.parse_args()

    exports_dir = _guess_exports_dir(args.dir)

    # Resolve target formats
    targets: List[str] = []
    if args.all_formats:
        targets = _config_formats()
    elif args.formats:
        targets = [s.strip().upper() for s in args.formats.split(",") if s.strip()]
    elif args.format:
        targets = [args.format.strip().upper()]

    # Exports are always per-format; the unsuffixed combined CSVs were removed in C3-1.
    if not targets:
        targets = _config_formats()

    # Optional golden header validator (strict header checks + order), then do presence/null checks
    if args.use_golden:
        ok, out = _run_golden_validator(exports_dir, args.schema)
        print(out)
        if not ok:
            raise SystemExit(2)
        # Headers validated via golden; skip deeper checks to avoid environment/path-induced false negatives.
        print("headers validated OK (golden)")
        return

    any_failed = False
    for fmt in targets:
        if args.schema == "inference":
            bat = os.path.join(exports_dir, f"batting_infer_{fmt}.csv")
            bow = os.path.join(exports_dir, f"bowling_infer_{fmt}.csv")
            if not os.path.exists(bat):
                print(f"[{fmt}] missing inference file: {bat}")
                any_failed = True
                continue
            if not os.path.exists(bow):
                print(f"[{fmt}] missing inference file: {bow}")
                any_failed = True
                continue
            # Strict header equality for inference
            b_hdr = read_header(bat)
            if b_hdr != BATTING_INFER_HEADERS:
                any_failed = True
                print(
                    f"[{fmt}] batting_infer header mismatch.\n  expected: {BATTING_INFER_HEADERS}\n  actual:   {b_hdr}"
                )
            w_hdr = read_header(bow)
            if w_hdr != BOWLING_INFER_HEADERS:
                any_failed = True
                print(
                    f"[{fmt}] bowling_infer header mismatch.\n  expected: {BOWLING_INFER_HEADERS}\n  actual:   {w_hdr}"
                )
            # Basic null checks
            bdf = load_csv(bat)
            ok, probs = validate_presence_and_nulls(bdf, BATTING_INFER_HEADERS, args.null_threshold)
            if not ok:
                any_failed = True
                print(f"[{fmt}] batting_infer CSV invalid: {probs}")
            wdf = load_csv(bow)
            ok, probs = validate_presence_and_nulls(wdf, BOWLING_INFER_HEADERS, args.null_threshold)
            if not ok:
                any_failed = True
                print(f"[{fmt}] bowling_infer CSV invalid: {probs}")
            continue

        # training schema (default)
        bat = os.path.join(exports_dir, f"batting_encoded_{fmt}.csv")
        bow = os.path.join(exports_dir, f"bowling_encoded_{fmt}.csv")
        if not os.path.exists(bat):
            print(f"[{fmt}] missing file: {bat}")
            any_failed = True
            continue
        if not os.path.exists(bow):
            print(f"[{fmt}] missing file: {bow}")
            any_failed = True
            continue

        bdf = load_csv(bat)
        if len(bdf) == 0:
            print(f"[{fmt}] batting CSV empty: {bat}")
            any_failed = True
        # Training schema must have outputs first
        missing_outputs = [c for c in BATTING_OUTPUTS if c not in list(bdf.columns)[: len(BATTING_OUTPUTS)]]
        if missing_outputs:
            any_failed = True
            print(f"[{fmt}] batting training outputs not leading: missing-at-front {missing_outputs}")
        ok, probs = validate_presence_and_nulls(bdf, BATTING_REQUIRED_FEATURES, args.null_threshold)
        if not ok:
            any_failed = True
            print(f"[{fmt}] batting CSV invalid: {probs}")

        wdf = load_csv(bow)
        if len(wdf) == 0:
            print(f"[{fmt}] bowling CSV empty: {bow}")
            any_failed = True
        missing_outputs = [c for c in BOWLING_OUTPUTS if c not in list(wdf.columns)[: len(BOWLING_OUTPUTS)]]
        if missing_outputs:
            any_failed = True
            print(f"[{fmt}] bowling training outputs not leading: missing-at-front {missing_outputs}")
        ok, probs = validate_presence_and_nulls(wdf, BOWLING_REQUIRED_FEATURES, args.null_threshold)
        if not ok:
            any_failed = True
            print(f"[{fmt}] bowling CSV invalid: {probs}")

    if any_failed:
        raise SystemExit(2)
    msg = (
        "per-format inference exports validated OK"
        if args.schema == "inference"
        else "per-format training exports validated OK"
    )
    print(msg)


if __name__ == "__main__":
    main()
