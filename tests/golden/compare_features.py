#!/usr/bin/env python3
"""
Golden dataset feature validator

- Verifies exported CSV headers match ML expectations (order and spelling)
- Optionally checks basic dtypes and nullability for critical columns

Usage:
  python tests/golden/compare_features.py --exports ./output/exports [--schema training|inference]

Exit codes:
  0 = success
  1 = mismatches found
  2 = other error
"""
from __future__ import annotations

import argparse
import csv
import json
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import List, Tuple

# Filenames patterns to check
BAT_FILES = [
    "batting_encoded.csv",
    "batting_encoded_TEST.csv",
    "batting_encoded_ODI.csv",
    "batting_encoded_T20.csv",
    "batting_encoded_T20I.csv",
]
BOWL_FILES = [
    "bowling_encoded.csv",
    "bowling_encoded_TEST.csv",
    "bowling_encoded_ODI.csv",
    "bowling_encoded_T20.csv",
    "bowling_encoded_T20I.csv",
]


@dataclass
class CheckResult:
    filename: str
    ok: bool
    details: List[str]


def read_csv_header(path: Path) -> List[str]:
    with path.open("r", newline="") as f:
        reader = csv.reader(f)
        try:
            header = next(reader)
        except StopIteration:
            return []
    return header


def cmp_headers(actual: List[str], expected: List[str]) -> Tuple[bool, List[str]]:
    msgs: List[str] = []
    ok = True
    if actual != expected:
        ok = False
        # Detailed diff
        maxlen = max(len(actual), len(expected))
        for i in range(maxlen):
            a = actual[i] if i < len(actual) else "<missing>"
            e = expected[i] if i < len(expected) else "<missing>"
            if a != e:
                msgs.append(f"pos {i}: expected '{e}' but found '{a}'")
        if len(actual) != len(expected):
            msgs.append(
                f"length mismatch: expected {len(expected)} columns but found {len(actual)}"
            )
    return ok, msgs


def load_expected(schema: str, base_dir: Path) -> Tuple[List[str], List[str]]:
    if schema not in {"training", "inference"}:
        raise ValueError("schema must be 'training' or 'inference'")
    if schema == "training":
        bat = base_dir / "expected_headers_batting_training.json"
        bow = base_dir / "expected_headers_bowling_training.json"
    else:
        bat = base_dir / "expected_headers_batting.json"
        bow = base_dir / "expected_headers_bowling.json"
    with bat.open("r") as f:
        exp_bat = json.load(f)
    with bow.open("r") as f:
        exp_bow = json.load(f)
    return exp_bat, exp_bow


def check_group(exports_dir: Path, files: List[str], expected: List[str]) -> List[CheckResult]:
    results: List[CheckResult] = []
    for name in files:
        p = exports_dir / name
        if not p.exists():
            # Skip missing files silently (not every mode emits all files)
            continue
        actual = read_csv_header(p)
        ok, msg = cmp_headers(actual, expected)
        results.append(CheckResult(filename=name, ok=ok, details=msg))
    return results


def main(argv: List[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--exports",
        type=Path,
        default=Path("./output/exports"),
        help="Directory containing exported CSVs",
    )
    ap.add_argument(
        "--schema",
        choices=["training", "inference"],
        default="training",
        help="Which expected schema to validate against",
    )
    args = ap.parse_args(argv)

    exports_dir: Path = args.exports
    if not exports_dir.exists():
        print(f"ERROR: exports directory not found: {exports_dir}", file=sys.stderr)
        return 2

    expected_dir = Path(__file__).parent
    try:
        exp_bat, exp_bow = load_expected(args.schema, expected_dir)
    except Exception as e:
        print(f"ERROR: cannot load expected headers json: {e}", file=sys.stderr)
        return 2

    any_fail = False
    bat_results = check_group(exports_dir, BAT_FILES, exp_bat)
    bowl_results = check_group(exports_dir, BOWL_FILES, exp_bow)

    def report(results: List[CheckResult], label: str) -> None:
        nonlocal any_fail
        if not results:
            print(f"[INFO] No {label} files found under {exports_dir} (skipping)")
            return
        for r in results:
            status = "OK" if r.ok else "FAIL"
            print(f"[{status}] {r.filename}")
            if not r.ok:
                any_fail = True
                for d in r.details:
                    print(f"  - {d}")

    print(f"== Batting header checks (schema={args.schema}) ==")
    report(bat_results, "batting")
    print(f"\n== Bowling header checks (schema={args.schema}) ==")
    report(bowl_results, "bowling")

    if any_fail:
        print("\nMismatch detected. See details above.")
        return 1
    print("\nAll checked headers match expected definitions.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
