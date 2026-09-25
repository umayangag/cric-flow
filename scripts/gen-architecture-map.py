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
  ml-service/ml/xi/contract.py          XI_FEATURE_COLS, DISPLAY_FEATURE_COLS, TARGET_COL
  ml-service/ml/xi/performance.py       TARGETS
  ml-service/app/main.py                @app.get/@app.post routes
  go-app/internal/server/router.go      HandleFunc routes

Usage:
    python scripts/gen-architecture-map.py            # rewrite the generated blocks
    python scripts/gen-architecture-map.py --check    # exit 1 if they are out of date
"""

from __future__ import annotations

import argparse
import ast
import os
import re
import sys
from typing import List, Tuple

MAP_PATH = "ARCHITECTURE_MAP.md"
BEGIN = "<!-- BEGIN GENERATED: {name} -- edit scripts/gen-architecture-map.py, not this block -->"
END = "<!-- END GENERATED: {name} -->"


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
        targets = (
            node.targets if isinstance(node, ast.Assign) else ([node.target] if isinstance(node, ast.AnnAssign) else [])
        )
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
    """Extract HandleFunc paths and their HTTP methods from the router.

    The path and the ``.Methods(...)`` call are matched in two steps rather than one.
    A single expression has to skip over the handler argument, and the handler is
    sometimes itself a call -- ``a.mlServiceProxy("/health", "ml health proxy", nil)``
    -- whose parentheses ended the scan early: three proxy routes were missing from
    the generated table, which is exactly the drift this file exists to prevent.
    So: find each registration, then read the ``Methods(...)`` that follows it and
    precedes the next one.

    Commented-out lines are dropped first. A route someone has commented out is a route
    the service does not serve, and a generated table that still lists it is the exact
    lie this file exists to stop.
    """
    with open(path, encoding="utf-8") as fh:
        src = "".join(line for line in fh if not line.lstrip().startswith("//"))
    starts = [(m.start(), m.group(1), m.end()) for m in re.finditer(r'HandleFunc\(\s*"([^"]+)"', src)]
    routes = []
    for i, (_, path_, end) in enumerate(starts):
        stop = starts[i + 1][0] if i + 1 < len(starts) else len(src)
        methods_match = re.search(r"Methods\(([^)]*)\)", src[end:stop], re.S)
        methods = methods_match.group(1) if methods_match else ""
        verbs = sorted({v.split("Method")[-1].upper() for v in re.findall(r"http\.Method(\w+)", methods)} - {"OPTIONS"})
        routes.append((", ".join(verbs) or "GET", path_))
    return routes


def render_models(root: str) -> str:
    """The model table, read from the contracts the code actually fits on.

    The XI layer is the whole serving surface: two win models over the same XI columns,
    one performance model with several targets, and a simulator that trains nothing. The
    windowed-form win model went in P-6, with the precompute and export steps that fed it.
    """
    objective_in = list_from_import(root, "ml.xi.contract", "XI_FEATURE_COLS")
    display_in = list_from_import(root, "ml.xi.contract", "DISPLAY_FEATURE_COLS")
    win_target = scalar_from_import(root, "ml.xi.contract", "TARGET_COL")
    formats = list_from_import(root, "ml.xi.contract", "FORMAT_CODES")
    perf_targets = performance_targets(root)

    rows = [
        (
            "XI win — objective",
            "Match",
            len(objective_in),
            [win_target],
            "`ml.xi.contract.XI_FEATURE_COLS` — every column is a function of the two elevens",
        ),
        (
            "XI win — display",
            "Match",
            len(display_in),
            [win_target],
            "`ml.xi.contract.DISPLAY_FEATURE_COLS` — the XI columns bar the Elo spread (B-7), "
            "plus team and venue context, home advantage and the toss",
        ),
        ("Performance (L2-B)", "Player", len(objective_in), perf_targets, "as-of player-match rows from `ml.xi.rows`"),
    ]

    out = ["| Model | Level | Inputs | Outputs | Input source |", "|-------|-------|--------|---------|--------------|"]
    for name, level, n_in, outs, source in rows:
        out.append(f"| **{name}** | {level} | {n_in} | {len(outs)} — {', '.join(f'`{o}`' for o in outs)} | {source} |")

    out.append("")
    out.append(
        f"One model of each kind per format: {', '.join(f'`{f}`' for f in formats)}. "
        "The simulator (L2-C) trains nothing — it draws from the performance model."
    )
    out.append("")
    out.append("Input feature names, in order:")
    out.append("")
    for label, names, source in (
        ("XI win — objective", objective_in, "ml.xi.contract.XI_FEATURE_COLS"),
        ("XI win — display", display_in, "ml.xi.contract.DISPLAY_FEATURE_COLS"),
    ):
        shown = ", ".join(f"`{n}`" for n in names[:12])
        more = f" … (+{len(names) - 12} more, see `{source}`)" if len(names) > 12 else ""
        out.append(f"- **{label}** ({len(names)}): {shown}{more}")
    return "\n".join(out)


def performance_targets(root: str) -> List[str]:
    """The performance model's target names, in the order it fits them."""
    import importlib

    ml_root = os.path.join(root, "ml-service")
    if ml_root not in sys.path:
        sys.path.insert(0, ml_root)
    try:
        module = importlib.import_module("ml.xi.performance")
    except Exception as exc:  # pragma: no cover - surfaced to the caller
        raise SystemExit(
            f"error: could not import ml.xi.performance ({exc}).\n"
            "Run via `make gen-architecture-map`, which uses the ml-service venv."
        ) from exc
    return [t.name for t in module.TARGETS]


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
            "ERROR: ARCHITECTURE_MAP.md generated sections are stale.\nRun: python scripts/gen-architecture-map.py",
            file=sys.stderr,
        )
        return 1
    open(path, "w", encoding="utf-8").write(updated)
    print("ARCHITECTURE_MAP.md regenerated")
    return 0


if __name__ == "__main__":
    sys.exit(main())
