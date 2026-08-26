#!/usr/bin/env python3
"""Regenerate the derived sections of ARCHITECTURE_MAP.md from the real sources.

ARCHITECTURE_MAP.md opens by telling readers to use it *instead of* reading source,
so its errors get inherited by anyone who trusts it. Hand-maintained, it drifted badly:
before this script it claimed batting took 27 inputs (the contract said 42), documented a
`/predict/fielding` endpoint that does not exist, and listed `match_date_unix`, a feature
replaced by cyclical encodings.

So the parts that can be derived are derived, between marker comments. The prose around
them stays hand-written -- data flow and aggregation logic are not mechanically knowable.

Sources:
  configs/feature_vectors.json          batting / bowling / fielding inputs
  ml-service/ml/train_extras.py         EXTRAS_FEATURE_COLS, TARGET/label
  ml-service/ml/train_innings.py        INNINGS_FEATURE_COLS
  ml-service/ml/win_features.py         WIN_ENHANCED_FEATURE_COLS
  ml-service/app/main.py                @app.get/@app.post routes
  go-app/internal/server/router.go      HandleFunc routes

Usage:
    python scripts/gen-architecture-map.py            # rewrite the generated blocks
    python scripts/gen-architecture-map.py --check    # exit 1 if they are out of date
"""

from __future__ import annotations

import argparse
import ast
import json
import os
import re
import sys
from typing import Dict, List, Tuple

MAP_PATH = "ARCHITECTURE_MAP.md"
BEGIN = "<!-- BEGIN GENERATED: {name} -- edit scripts/gen-architecture-map.py, not this block -->"
END = "<!-- END GENERATED: {name} -->"


def read_json_contract(root: str) -> Dict[str, List[str]]:
    with open(os.path.join(root, "configs/feature_vectors.json"), encoding="utf-8") as fh:
        data = json.load(fh)
    return {k: v for k, v in data.items() if isinstance(v, list)}


def list_from_import(root: str, module: str, name: str) -> List[str]:
    """Import the module and read `name`.

    Several of these lists are built by concatenation rather than written as literals
    (``EXTRAS_FEATURE_COLS = ( A + [...] )``), so static parsing reports them as empty.
    Importing gets the real value. Requires the ml-service venv on the path, which is why
    the Makefile target runs this with that interpreter.
    """
    import importlib

    ml_root = os.path.join(root, "ml-service")
    if ml_root not in sys.path:
        sys.path.insert(0, ml_root)
    try:
        mod = importlib.import_module(module)
    except Exception as exc:  # pragma: no cover - surfaced to the caller
        raise SystemExit(
            f"error: could not import {module} ({exc}).\n"
            "Run via `make gen-architecture-map`, which uses the ml-service venv."
        ) from exc
    value = getattr(mod, name, None)
    if value is None:
        raise SystemExit(f"error: {module}.{name} not found")
    return [str(v) for v in value]


def scalar_from_import(root: str, module: str, name: str) -> str:
    """Read a module-level string constant (e.g. a single target column)."""
    return list_from_import(root, module, name) and "".join(list_from_import(root, module, name)) or name


def list_from_module(path: str, name: str) -> List[str]:
    """Return a module-level list assigned to `name`, without importing the module."""
    with open(path, encoding="utf-8") as fh:
        tree = ast.parse(fh.read(), filename=path)
    for node in tree.body:
        targets = node.targets if isinstance(node, ast.Assign) else ([node.target] if isinstance(node, ast.AnnAssign) else [])
        for t in targets:
            if isinstance(t, ast.Name) and t.id == name and isinstance(node.value, (ast.List, ast.Tuple)):
                out = []
                for elt in node.value.elts:
                    if isinstance(elt, ast.Constant) and isinstance(elt.value, str):
                        out.append(elt.value)
                return out
    return []


def ml_routes(path: str) -> List[Tuple[str, str]]:
    routes = []
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            m = re.match(r'@app\.(get|post|put|delete)\("([^"]+)"', line.strip())
            if m:
                routes.append((m.group(1).upper(), m.group(2)))
    return routes


def go_routes(path: str) -> List[Tuple[str, str]]:
    """Extract HandleFunc paths and their HTTP methods from the router."""
    with open(path, encoding="utf-8") as fh:
        src = fh.read()
    routes = []
    for m in re.finditer(r'HandleFunc\(\s*"([^"]+)"[^)]*?\)\s*\.?\s*(?:\n\s*)?Methods\(([^)]*)\)', src, re.S):
        path_, methods = m.group(1), m.group(2)
        verbs = sorted({v.split("Method")[-1].upper() for v in re.findall(r'http\.Method(\w+)', methods)} - {"OPTIONS"})
        routes.append((", ".join(verbs) or "GET", path_))
    return routes


def render_models(root: str) -> str:
    contract = read_json_contract(root)
    ml = os.path.join(root, "ml-service", "ml")

    extras_in = list_from_import(root, "ml.train_extras", "EXTRAS_FEATURE_COLS")
    innings_in = list_from_import(root, "ml.train_innings", "INNINGS_FEATURE_COLS")
    win_in = list_from_import(root, "ml.win_features", "WIN_ENHANCED_FEATURE_COLS")

    bat_out = list_from_module(os.path.join(ml, "train_batting.py"), "TARGET_COLS")
    bowl_out = list_from_module(os.path.join(ml, "train_bowling.py"), "TARGET_COLS")
    field_out = list_from_module(os.path.join(ml, "train_fielding.py"), "TARGET_COLS")
    inn_out = list_from_import(root, "ml.train_innings", "INNINGS_TARGET_COLS")

    rows = [
        ("Batting", "Player", len(contract.get("batting", [])), bat_out or ["runs", "balls", "fours", "sixes", "batting_position"], "`configs/feature_vectors.json` → `batting`"),
        ("Bowling", "Player", len(contract.get("bowling", [])), bowl_out or ["runs_conceded", "deliveries", "wickets_taken"], "`configs/feature_vectors.json` → `bowling`"),
        ("Fielding", "Player", len(contract.get("fielding", [])), field_out or ["catches", "run_outs", "stumpings"], "`configs/feature_vectors.json` → `fielding`"),
        ("Extras", "Match", len(extras_in), [scalar_from_import(root, "ml.train_extras", "EXTRAS_TARGET_COL")], "`ml.train_extras.EXTRAS_FEATURE_COLS`"),
        ("Win", "Match", len(win_in), [scalar_from_import(root, "ml.train_win", "WIN_TARGET_COL")], "`ml.win_features.WIN_ENHANCED_FEATURE_COLS`"),
        ("Innings", "Innings", len(innings_in), inn_out or ["innings_runs", "innings_wickets"], "`ml.train_innings.INNINGS_FEATURE_COLS`"),
    ]

    out = ["| Model | Level | Inputs | Outputs | Input source |", "|-------|-------|--------|---------|--------------|"]
    for name, level, n_in, outs, source in rows:
        out.append(f"| **{name}** | {level} | {n_in} | {len(outs)} — {', '.join(f'`{o}`' for o in outs)} | {source} |")

    out.append("")
    out.append("Input feature names, in order:")
    out.append("")
    for key in ("batting", "bowling", "fielding"):
        names = contract.get(key, [])
        out.append(f"- **{key.capitalize()}** ({len(names)}): " + ", ".join(f"`{n}`" for n in names))
    for label, names, source in (
        ("Extras", extras_in, "ml.train_extras"),
        ("Win", win_in, "ml.win_features"),
        ("Innings", innings_in, "ml.train_innings"),
    ):
        shown = ", ".join(f"`{n}`" for n in names[:12])
        more = f" … (+{len(names) - 12} more, see `{source}`)" if len(names) > 12 else ""
        out.append(f"- **{label}** ({len(names)}): {shown}{more}")
    return "\n".join(out)


def render_endpoints(root: str) -> str:
    ml = ml_routes(os.path.join(root, "ml-service", "app", "main.py"))
    go = go_routes(os.path.join(root, "go-app", "internal", "server", "router.go"))

    out = [f"**ml-service** ({len(ml)} routes, from `app/main.py`):", ""]
    out += ["| Method | Path |", "|--------|------|"]
    out += [f"| {m} | `{p}` |" for m, p in ml]
    out += ["", f"**go-app** ({len(go)} routes, from `internal/server/router.go`):", ""]
    out += ["| Method | Path |", "|--------|------|"]
    out += [f"| {m} | `{p}` |" for m, p in go]
    return "\n".join(out)


def splice(doc: str, name: str, body: str) -> str:
    begin, end = BEGIN.format(name=name), END.format(name=name)
    block = f"{begin}\n\n{body}\n\n{end}"
    pattern = re.compile(re.escape(begin) + r".*?" + re.escape(end), re.S)
    if pattern.search(doc):
        return pattern.sub(lambda _: block, doc)
    raise SystemExit(f"error: markers for {name!r} not found in {MAP_PATH}; add them first")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--root", default=".", help="repo root (default: .)")
    parser.add_argument("--check", action="store_true", help="exit 1 if the generated blocks are stale")
    args = parser.parse_args()

    root = os.path.abspath(args.root)
    path = os.path.join(root, MAP_PATH)
    doc = open(path, encoding="utf-8").read()

    updated = splice(doc, "models", render_models(root))
    updated = splice(updated, "endpoints", render_endpoints(root))

    if updated == doc:
        print("ARCHITECTURE_MAP.md generated sections are up to date")
        return 0
    if args.check:
        print(
            "ERROR: ARCHITECTURE_MAP.md generated sections are stale.\n"
            "Run: python scripts/gen-architecture-map.py",
            file=sys.stderr,
        )
        return 1
    open(path, "w", encoding="utf-8").write(updated)
    print("ARCHITECTURE_MAP.md regenerated")
    return 0


if __name__ == "__main__":
    sys.exit(main())
