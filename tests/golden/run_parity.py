#!/usr/bin/env python3
"""
Golden parity runner (scaffold)

- Loads exported per-format training CSVs from go-app/cmd/export-dataset
- Builds inference payloads for ML service (batting, bowling, and win)
- Optionally calls the ML service if ML_BASE_URL is set (defaults to http://localhost:8000)
- Writes sample payloads and (if called) prediction outputs to tests/golden/out/

Usage examples:
  python tests/golden/run_parity.py --exports ./output/exports --formats ODI,T20 --limit 20
  ML_BASE_URL=http://localhost:8000 python tests/golden/run_parity.py --format T20 --limit 50 --call

Notes:
- This is a harness scaffold to aid side-by-side comparisons against the prototype.
- It focuses on payload construction and repeatability; prototype-run integration can be added later.
"""
from __future__ import annotations

import argparse
import csv
import json
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Tuple

try:
    import requests  # type: ignore
except Exception:  # pragma: no cover
    requests = None  # lazy optional dependency


@dataclass
class Options:
    exports: Path
    formats: List[str]
    limit: int
    call: bool
    outdir: Path
    ml_base: str


BAT_MAP = {
    # training->inference field name mapping
    "batting_consistency": "batting_consistency",
    "batting_form": "batting_form",
    "temp": "batting_temp",
    "wind": "batting_wind",
    "rain": "batting_rain",
    "humidity": "batting_humidity",
    "cloud": "batting_cloud",
    "pressure": "batting_pressure",
    "viscosity": "batting_viscosity",
    "inning": "batting_inning",
    "batting_session": "batting_session",
    "toss": "toss",
    "batting_venue": "venue",
    "batting_opposition": "opposition",
    "season_id": "season",
    "player_name": "player_name",
}

BOWL_MAP = {
    "bowling_consistency": "bowling_consistency",
    "bowling_form": "bowling_form",
    "temp": "bowling_temp",
    "wind": "bowling_wind",
    "rain": "bowling_rain",
    "humidity": "bowling_humidity",
    "cloud": "bowling_cloud",
    "pressure": "bowling_pressure",
    "viscosity": "bowling_viscosity",
    "inning": "batting_inning",
    "bowling_session": "bowling_session",
    "toss": "toss",
    "bowling_venue": "bowling_venue",
    "bowling_opposition": "bowling_opposition",
    "season_id": "season",
    "player_name": "player_name",
}


def read_csv_limited(path: Path, limit: int) -> Tuple[List[str], List[List[str]]]:
    with path.open("r", newline="") as f:
        r = csv.reader(f)
        try:
            header = next(r)
        except StopIteration:
            return [], []
        rows: List[List[str]] = []
        for i, row in enumerate(r):
            rows.append(row)
            if limit > 0 and len(rows) >= limit:
                break
    return header, rows


def project_row(header: List[str], row: List[str], fmap: Dict[str, str]) -> Dict[str, object]:
    h2i = {h: i for i, h in enumerate(header)}
    out: Dict[str, object] = {}
    for src, dst in fmap.items():
        if src in h2i:
            val = row[h2i[src]]
            out[dst] = _coerce(val)
    return out


def _coerce(v: str) -> object:
    s = v.strip()
    if s == "":
        return None
    # ints
    try:
        return int(s)
    except ValueError:
        pass
    # floats
    try:
        return float(s)
    except ValueError:
        pass
    return s


def load_inference_payloads(fmt: str, exports_dir: Path, limit: int) -> Tuple[List[Dict[str, object]], List[Dict[str, object]]]:
    bat_file = exports_dir / f"batting_encoded_{fmt}.csv"
    bow_file = exports_dir / f"bowling_encoded_{fmt}.csv"
    bat_header, bat_rows = read_csv_limited(bat_file, limit)
    bow_header, bow_rows = read_csv_limited(bow_file, limit)
    bat_payload = [project_row(bat_header, r, BAT_MAP) for r in bat_rows]
    bow_payload = [project_row(bow_header, r, BOWL_MAP) for r in bow_rows]
    # set format explicitly
    for d in bat_payload:
        d.setdefault("format", fmt)
    for d in bow_payload:
        d.setdefault("format", fmt)
    return bat_payload, bow_payload


def call_ml(ml_base: str, route: str, payload: List[Dict[str, object]]) -> Optional[List[Dict[str, object]]]:
    if not requests:
        print("[WARN] 'requests' not available; skipping ML calls. Install via: pip install requests")
        return None
    url = ml_base.rstrip("/") + route
    resp = requests.post(url, json=payload, timeout=30)
    if resp.status_code != 200:
        print(f"[ERROR] {route} -> {resp.status_code}: {resp.text[:500]}")
        return None
    try:
        return resp.json()
    except Exception as e:  # pragma: no cover
        print(f"[ERROR] JSON decode failed: {e}")
        return None


def ensure_outdir(p: Path) -> None:
    p.mkdir(parents=True, exist_ok=True)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--exports", type=Path, default=Path("./output/exports"))
    ap.add_argument("--format", type=str, default="")
    ap.add_argument("--formats", type=str, default="")
    ap.add_argument("--limit", type=int, default=50)
    ap.add_argument("--call", action="store_true", help="Call the ML service endpoints if ML_BASE_URL is set")
    ap.add_argument("--out", type=Path, default=Path("tests/golden/out"))
    args = ap.parse_args()

    formats: List[str] = []
    if args.format:
        formats = [args.format.strip().upper()]
    elif args.formats:
        formats = [s.strip().upper() for s in args.formats.split(",") if s.strip()]
    else:
        formats = ["ODI", "T20"]  # default small slice to start

    ml_base = os.environ.get("ML_BASE_URL", "http://localhost:8000").strip()

    opts = Options(exports=args.exports, formats=formats, limit=args.limit, call=args.call, outdir=args.out, ml_base=ml_base)

    ensure_outdir(opts.outdir)

    summary: Dict[str, object] = {"formats": formats, "limit": opts.limit}

    for fmt in formats:
        bat_payload, bow_payload = load_inference_payloads(fmt, opts.exports, opts.limit)
        # Save payloads
        with (opts.outdir / f"batting_payload_{fmt}.json").open("w") as f:
            json.dump(bat_payload, f, indent=2)
        with (opts.outdir / f"bowling_payload_{fmt}.json").open("w") as f:
            json.dump(bow_payload, f, indent=2)
        print(f"[OK] Built payloads for {fmt}: batting={len(bat_payload)} bowling={len(bow_payload)}")

        if opts.call:
            batting = call_ml(opts.ml_base, "/predict/batting", bat_payload)
            bowling = call_ml(opts.ml_base, "/predict/bowling", bow_payload)
            if batting is not None:
                with (opts.outdir / f"batting_preds_{fmt}.json").open("w") as f:
                    json.dump(batting, f, indent=2)
            if bowling is not None:
                with (opts.outdir / f"bowling_preds_{fmt}.json").open("w") as f:
                    json.dump(bowling, f, indent=2)

    # future: compute team selection and /predict-win payloads
    with (opts.outdir / "summary.json").open("w") as f:
        json.dump(summary, f, indent=2)

    print(f"[DONE] Outputs written under {opts.outdir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
