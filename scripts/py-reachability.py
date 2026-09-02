#!/usr/bin/env python3
"""Report ml-service modules unreachable from any live entrypoint.

Why this exists: `ruff` and the test suite both stay quiet about a module that
nothing imports, because its own tests keep it "used". That is how ml-service
accumulated ~2,500 lines of unreachable code (see docs/CLEANUP_PR_CHECKLIST.md,
C1-4 and C1-6). This walks the import graph from the entrypoints that actually
run in production and reports whatever it cannot reach.

Method: parse every module under the source roots with `ast`, resolve absolute
and relative imports to module names, then breadth-first search from ENTRYPOINTS
(modules run via `python -m`, plus modules run as scripts). Tests are deliberately
*not* roots -- a module reachable only from its own test is exactly what we are
looking for.

Usage:
    python scripts/py-reachability.py            # list unreachable modules
    python scripts/py-reachability.py --check    # exit 1 if any are unlisted
    python scripts/py-reachability.py --json     # machine-readable output
"""

from __future__ import annotations

import argparse
import ast
import json
import os
import sys
from collections import defaultdict
from typing import Dict, List, Set

# Source roots scanned for modules. Tests are parsed for imports but never used as roots.
SOURCE_ROOTS = ("app", "ml")

# Entrypoints imported by nothing but executed directly. Both kinds are graph roots:
# a root's own imports are reachable, which an allowlist entry would not achieve.
#
#   app.main            the ASGI app uvicorn serves
#   ml.train_win        invoked as `python -m ml.train_win` by app/training_orchestrator.py
#                       (run_training_subprocess) and by the Makefiles
#   ml.auto_tune        invoked as `python -m ml.auto_tune`
#   ml.win_discrimination  invoked by `make -C ml-service win-discrimination`
#   ml.xi.train         invoked by `make train-xi`
#
# Keep this in step with the `-m ml.` call sites; the check fails loudly if an
# entrypoint listed here no longer exists on disk.
MODULE_ENTRYPOINTS = (
    "app.main",
    "ml.train_win",
    "ml.auto_tune",
    "ml.win_discrimination",
    "ml.xi.train",
)

# Run as scripts rather than imported, so no module imports them -- but they and
# everything they pull in are live. Value is the command that runs them.
SCRIPT_ENTRYPOINTS: Dict[str, str] = {
    "ml.validate_exports": "make -C ml-service validate-exports",
    "ml.xi.parity": "make xi-parity",
    "ml.xi.evaluate": "make xi-evaluate",
}

ENTRYPOINTS = MODULE_ENTRYPOINTS + tuple(SCRIPT_ENTRYPOINTS)

# Modules that are unreachable on purpose and are neither kind of entrypoint.
# Prefer SCRIPT_ENTRYPOINTS when something is actually executed -- this is a last
# resort, and empty is the healthy state.
ALLOWED_UNREACHABLE: Dict[str, str] = {}


def discover_modules(base: str) -> Dict[str, str]:
    """Map dotted module name -> file path for every .py under the source roots."""
    modules: Dict[str, str] = {}
    for root in SOURCE_ROOTS:
        root_dir = os.path.join(base, root)
        if not os.path.isdir(root_dir):
            continue
        for dirpath, dirnames, filenames in os.walk(root_dir):
            dirnames[:] = [d for d in dirnames if d not in ("__pycache__", ".venv")]
            for filename in filenames:
                if not filename.endswith(".py"):
                    continue
                path = os.path.join(dirpath, filename)
                dotted = os.path.relpath(path, base)[: -len(".py")].replace(os.sep, ".")
                if dotted.endswith(".__init__"):
                    dotted = dotted[: -len(".__init__")]
                modules[dotted] = path
    return modules


def is_package(dotted: str, path: str) -> bool:
    return os.path.basename(path) == "__init__.py"


def build_import_graph(base: str, modules: Dict[str, str]) -> Dict[str, Set[str]]:
    """dotted module -> set of in-repo modules it imports."""
    packages = {d for d, p in modules.items() if is_package(d, p)}
    graph: Dict[str, Set[str]] = defaultdict(set)

    for dotted, path in modules.items():
        with open(path, encoding="utf-8") as handle:
            tree = ast.parse(handle.read(), filename=path)

        # For a package __init__, relative imports resolve against the package itself;
        # for a plain module they resolve against its parent package.
        context = dotted if dotted in packages else dotted.rpartition(".")[0]

        targets: Set[str] = set()
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    targets.add(alias.name)
            elif isinstance(node, ast.ImportFrom):
                if node.level:
                    parts = context.split(".") if context else []
                    keep = len(parts) - (node.level - 1)
                    base_pkg = ".".join(parts[:keep]) if keep >= 0 else ""
                    base_name = f"{base_pkg}.{node.module}".strip(".") if node.module else base_pkg
                else:
                    base_name = node.module or ""
                if base_name:
                    targets.add(base_name)
                # `from pkg import mod` imports a submodule, not just a name
                for alias in node.names:
                    targets.add(f"{base_name}.{alias.name}".strip("."))

        graph[dotted] = {t for t in targets if t in modules}

    # Importing a submodule executes its package __init__, so treat that as an edge.
    for dotted in list(graph):
        parent = dotted.rpartition(".")[0]
        while parent:
            if parent in modules:
                graph[parent]  # ensure the key exists
            parent = parent.rpartition(".")[0]
    return graph


def reachable_from(graph: Dict[str, Set[str]], modules: Dict[str, str], roots) -> Set[str]:
    seen: Set[str] = set()
    stack: List[str] = [r for r in roots if r in modules]
    while stack:
        current = stack.pop()
        if current in seen:
            continue
        seen.add(current)
        # importing a.b.c runs a and a.b as well
        parent = current.rpartition(".")[0]
        while parent:
            if parent in modules and parent not in seen:
                stack.append(parent)
            parent = parent.rpartition(".")[0]
        stack.extend(graph.get(current, set()) - seen)
    return seen


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base", default="ml-service", help="ml-service directory (default: ml-service)")
    parser.add_argument("--check", action="store_true", help="exit 1 when an unlisted module is unreachable")
    parser.add_argument("--json", action="store_true", help="emit JSON")
    args = parser.parse_args()

    base = os.path.abspath(args.base)
    if not os.path.isdir(base):
        print(f"error: {base} is not a directory", file=sys.stderr)
        return 2

    modules = discover_modules(base)
    missing_entrypoints = [e for e in ENTRYPOINTS if e not in modules]
    graph = build_import_graph(base, modules)
    reached = reachable_from(graph, modules, ENTRYPOINTS)

    unreachable = sorted(set(modules) - reached)
    unexpected = [m for m in unreachable if m not in ALLOWED_UNREACHABLE]

    # A listed module that is now reachable, or gone from disk, means the entry has
    # served its purpose. Fail on it so the allowlist cannot quietly rot.
    stale_allowlist = sorted(m for m in ALLOWED_UNREACHABLE if m not in modules or m in reached)

    if args.json:
        print(
            json.dumps(
                {
                    "total_modules": len(modules),
                    "reachable": len(reached),
                    "unreachable": unreachable,
                    "unexpected": unexpected,
                    "stale_allowlist": stale_allowlist,
                    "missing_entrypoints": missing_entrypoints,
                },
                indent=2,
            )
        )
    else:
        print(f"modules={len(modules)} reachable={len(reached)} unreachable={len(unreachable)}")
        for module in unreachable:
            reason = ALLOWED_UNREACHABLE.get(module)
            marker = "allowed" if reason else "UNREACHABLE"
            suffix = f" ({reason})" if reason else ""
            print(f"  [{marker}] {module}{suffix}")
        if stale_allowlist:
            print("\nallowlist entries no longer needed:")
            for entry in stale_allowlist:
                print(f"  {entry}")
        if missing_entrypoints:
            print("\nentrypoints declared but not found on disk:")
            for entry in missing_entrypoints:
                print(f"  {entry}")

    if missing_entrypoints:
        print(
            "\nERROR: an entrypoint in ENTRYPOINTS no longer exists. Update scripts/py-reachability.py"
            " -- a stale entrypoint makes this check report live modules as dead.",
            file=sys.stderr,
        )
        return 1

    if args.check and stale_allowlist:
        print(
            f"\nERROR: {len(stale_allowlist)} ALLOWED_UNREACHABLE entr(y/ies) are stale -- the module is now"
            " reachable or no longer exists:\n"
            + "\n".join(f"  {m}" for m in stale_allowlist)
            + "\n\nRemove them from scripts/py-reachability.py.",
            file=sys.stderr,
        )
        return 1

    if args.check and unexpected:
        print(
            f"\nERROR: {len(unexpected)} module(s) unreachable from any entrypoint:\n"
            + "\n".join(f"  {m}" for m in unexpected)
            + "\n\nEither delete them, wire them in, or -- if they are script entrypoints --"
            "\nadd them to ALLOWED_UNREACHABLE in scripts/py-reachability.py with a reason.",
            file=sys.stderr,
        )
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
