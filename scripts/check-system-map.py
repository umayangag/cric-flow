#!/usr/bin/env python3
"""Verify contracts/system-map.json against the code it describes, in both directions.

The System map tab draws a diagram of the whole prediction pipeline and tells its reader
that this is what the system does. A hand-drawn diagram answers a rename by going quietly
wrong, which is the failure ARCHITECTURE_MAP.md had before `gen-architecture-map.py`
(27 inputs against a contract that said 42, a `/predict/fielding` route that did not
exist). The same discipline applies here, with the same shape: the prose is curated, the
structure is data, and the data is checked against the code on every PR.

Two directions, and both matter:

**Forward — everything the map names must exist.** Modules, Go packages, files,
endpoints, tables, make targets, artifact kinds, pipeline steps, gates, feature groups and
performance targets. Delete an endpoint or rename a module and this fails, so the map
cannot go on drawing a box for something that is gone.

**Reverse — everything the code can enumerate must be on the map.** Every route both
services serve, every step in the ops registry, every gate in the H-23 registry, every
feature-group constant in the XI contract, every performance target and every artifact
kind. Add an endpoint and this fails, so the map cannot quietly fall behind either.

Numbers are deliberately *not* checked here: none are written in the map. Nodes carry
binding keys that the tab resolves live from /ops/status, /api/ml/xi-status and
/api/backtest/report, so a stale number is not a thing the map can hold.

Usage:
    python scripts/check-system-map.py          # exit 1 on any violation
    python scripts/check-system-map.py --root . # from elsewhere
"""

from __future__ import annotations

import argparse
import ast
import importlib
import importlib.util
import json
import os
import re
import sys
from typing import Any, Dict, Iterable, List, Sequence, Set, Tuple

MAP_PATH = os.path.join("contracts", "system-map.json")
OPS_CONTRACT_PATH = os.path.join("contracts", "ops-console.contract.json")

#: Anchor kinds a node (or a child) may declare. A kind not listed here is a typo that
#: would otherwise be silently unchecked, which is the one failure mode this file cannot
#: afford, so an unknown kind is an error.
ANCHOR_KINDS = (
    "modules",
    "packages",
    "files",
    "endpoints",
    "tables",
    "make_targets",
    "artifacts",
    "pipeline_steps",
    "gates",
    "feature_groups",
    "targets",
)

#: How a binding's value may be rendered. The frontend asserts it handles exactly these.
VALUE_KINDS = (
    "integer",
    "number1",
    "ratio3",
    "signed3",
    "percent",
    "bytes",
    "text",
    "sha",
    "date",
    "timestamp",
    "yes_no",
    "list",
)

#: Feature-group constants are the module-level lists in the XI contract whose names end
#: this way. Every one of them must be named by some node, so a new column family cannot
#: be added without deciding where on the map it belongs.
FEATURE_GROUP_PATTERN = re.compile(r"^[A-Z][A-Z0-9_]*(_COLS|_KEYS|_STEMS)$")


class Problems:
    """Collected violations. Every check runs; the exit code is decided at the end."""

    def __init__(self) -> None:
        self.items: List[str] = []

    def add(self, where: str, message: str) -> None:
        self.items.append(f"{where}: {message}")

    def extend_missing(self, where: str, missing: Iterable[str], noun: str) -> None:
        for item in sorted(missing):
            self.add(where, f"{noun} {item!r} is not on the map")


# --- Reading the sources of truth ---------------------------------------------------


def load_generator(root: str):
    """The route extractors, imported from the generator rather than copied.

    Two regexes that must agree about what a route is are two regexes that will stop
    agreeing. `gen-architecture-map.py` already owns them; its filename has a dash, so it
    is loaded by path rather than imported by name.
    """
    path = os.path.join(root, "scripts", "gen-architecture-map.py")
    spec = importlib.util.spec_from_file_location("gen_architecture_map", path)
    if spec is None or spec.loader is None:  # pragma: no cover - a broken checkout
        raise SystemExit(f"error: could not load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def import_ml(root: str, module_name: str):
    """Import an ml-service module, the way the generator does."""
    ml_root = os.path.join(root, "ml-service")
    if ml_root not in sys.path:
        sys.path.insert(0, ml_root)
    try:
        return importlib.import_module(module_name)
    except Exception as exc:  # pragma: no cover - surfaced to the caller
        raise SystemExit(
            f"error: could not import {module_name} ({exc}).\n"
            "Run via `make check-system-map`, which uses the ml-service venv."
        ) from exc


def served_endpoints(root: str, generator) -> Set[str]:
    """Every route both services serve, as `"METHODS /path"`."""
    ml = generator.ml_routes(os.path.join(root, "ml-service", "app", "main.py"))
    go = generator.go_routes(os.path.join(root, "go-app", "internal", "server", "router.go"))
    return {f"{methods} {path}" for methods, path in list(ml) + list(go)}


def live_tables(root: str) -> Set[str]:
    """Tables a migration creates and no later migration drops.

    Read from the SQL rather than from a list, so dropping a table in a migration is
    enough to make the map that still draws it fail.
    """
    migrations_dir = os.path.join(root, "go-app", "migrations")
    created: Set[str] = set()
    dropped: Set[str] = set()
    for name in sorted(os.listdir(migrations_dir)):
        if not name.endswith(".sql"):
            continue
        with open(os.path.join(migrations_dir, name), encoding="utf-8") as fh:
            sql = fh.read()
        for match in re.finditer(r"CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([\w\.\"]+)", sql, re.I):
            created.add(match.group(1).replace('"', "").split(".")[-1].lower())
        for match in re.finditer(r"DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?([\w\.\",\s]+?);", sql, re.I):
            for table in match.group(1).split(","):
                dropped.add(table.strip().replace('"', "").split(".")[-1].lower())
    return created - dropped


def make_targets(root: str) -> Set[str]:
    """Targets defined in the root Makefile."""
    with open(os.path.join(root, "Makefile"), encoding="utf-8") as fh:
        return {m.group(1) for m in re.finditer(r"^([a-zA-Z][\w.-]*)\s*:(?!=)", fh.read(), re.M)}


def artifact_kinds(root: str) -> Set[str]:
    """Every kind of file a run directory holds, named by the code that writes it."""
    store = import_ml(root, "ml.xi.store")
    runs = import_ml(root, "ml.xi.runs")
    return {
        store.RATINGS_ARTIFACT,
        store.model_artifact_name("<FORMAT>"),
        store.performance_artifact_name("<FORMAT>"),
        runs.MANIFEST_NAME,
        runs.CURRENT_POINTER_NAME,
    }


def feature_groups(root: str) -> Set[str]:
    """The XI contract's column-family constants, read without importing it."""
    path = os.path.join(root, "ml-service", "ml", "xi", "contract.py")
    with open(path, encoding="utf-8") as fh:
        tree = ast.parse(fh.read(), filename=path)
    names: Set[str] = set()
    for node in tree.body:
        targets = (
            node.targets if isinstance(node, ast.Assign) else ([node.target] if isinstance(node, ast.AnnAssign) else [])
        )
        for target in targets:
            if isinstance(target, ast.Name) and FEATURE_GROUP_PATTERN.match(target.id):
                names.add(target.id)
    return names


def pipeline_step_ids(root: str) -> Set[str]:
    """Every step the ops registry defines, compute lane and data lane alike."""
    with open(os.path.join(root, OPS_CONTRACT_PATH), encoding="utf-8") as fh:
        contract = json.load(fh)
    return {step["id"] for step in contract["pipeline_steps"] + contract["data_steps"]}


def module_exists(root: str, dotted: str) -> bool:
    """Whether an ml-service module path resolves to a file or a package."""
    base = os.path.join(root, "ml-service", *dotted.split("."))
    return os.path.isfile(base + ".py") or os.path.isfile(os.path.join(base, "__init__.py"))


# --- The checks ----------------------------------------------------------------------


def collect_anchors(nodes: Sequence[Dict[str, Any]]) -> Dict[str, Set[str]]:
    """Every anchor value on the map, grouped by kind — nodes and their children."""
    collected: Dict[str, Set[str]] = {kind: set() for kind in ANCHOR_KINDS}
    for node in nodes:
        for holder in [node] + list(node.get("children", [])):
            for kind, values in holder.get("anchors", {}).items():
                if kind in collected:
                    collected[kind].update(values)
    return collected


def check_structure(system_map: Dict[str, Any], problems: Problems) -> None:
    """Ids, lanes, layout slots, edges and bindings hold together."""
    lanes = {lane["id"] for lane in system_map["lanes"]}
    sources = set(system_map["sources"])
    nodes = system_map["nodes"]

    seen_ids: Set[str] = set()
    slots: Dict[Tuple[str, int], str] = {}
    for node in nodes:
        node_id = node["id"]
        where = f"node {node_id}"
        if node_id in seen_ids:
            problems.add(where, "duplicate node id")
        seen_ids.add(node_id)

        if node["lane"] not in lanes:
            problems.add(where, f"lane {node['lane']!r} is not declared")
        slot = (node["lane"], node["column"])
        if slot in slots:
            problems.add(where, f"shares lane/column {slot} with {slots[slot]}")
        slots[slot] = node_id

        if not node.get("summary", "").strip():
            problems.add(where, "has no summary; every node must read for a non-expert")
        if not node.get("documented_in"):
            problems.add(where, "names no document")

        for holder in [node] + list(node.get("children", [])):
            for kind in holder.get("anchors", {}):
                if kind not in ANCHOR_KINDS:
                    problems.add(where, f"unknown anchor kind {kind!r}")

        child_ids: Set[str] = set()
        for child in node.get("children", []):
            if child["id"] in seen_ids or child["id"] in child_ids:
                problems.add(where, f"duplicate child id {child['id']!r}")
            child_ids.add(child["id"])
            if not child.get("summary", "").strip():
                problems.add(where, f"child {child['id']!r} has no summary")

        binding_keys: Set[str] = set()
        for binding in node.get("bindings", []):
            key = binding["key"]
            if key in binding_keys:
                problems.add(where, f"duplicate binding key {key!r}")
            binding_keys.add(key)
            if binding["source"] not in sources:
                problems.add(where, f"binding {key!r} reads undeclared source {binding['source']!r}")
            if binding["value_kind"] not in VALUE_KINDS:
                problems.add(where, f"binding {key!r} has unknown value_kind {binding['value_kind']!r}")
            if binding.get("per_format") and "{format}" not in binding["path"]:
                problems.add(where, f"binding {key!r} is per_format but its path names no {{format}}")
            if "{format}" in binding["path"] and not binding.get("per_format"):
                problems.add(where, f"binding {key!r} uses {{format}} without per_format")

    for edge in system_map["edges"]:
        for end in ("from", "to"):
            if edge[end] not in seen_ids:
                problems.add(f"edge {edge['from']}->{edge['to']}", f"{end} names no node")


def check_documents(root: str, system_map: Dict[str, Any], problems: Problems) -> None:
    """Every "documented in" pointer resolves to a document and a section still in it."""
    cache: Dict[str, str] = {}
    for node in system_map["nodes"]:
        for reference in node.get("documented_in", []):
            doc, section = reference["doc"], reference["section"]
            where = f"node {node['id']}"
            if doc not in cache:
                path = os.path.join(root, doc)
                if not os.path.isfile(path):
                    problems.add(where, f"documented_in names a missing document {doc}")
                    cache[doc] = ""
                    continue
                with open(path, encoding="utf-8") as fh:
                    cache[doc] = fh.read()
            if cache[doc] and section not in cache[doc]:
                problems.add(where, f"{doc} no longer has the section {section!r}")


def check_forward(root: str, system_map: Dict[str, Any], known: Dict[str, Set[str]], problems: Problems) -> None:
    """Everything the map names exists in the code."""
    for node in system_map["nodes"]:
        for holder in [node] + list(node.get("children", [])):
            where = f"node {node['id']}" if holder is node else f"node {node['id']} / {holder['id']}"
            anchors = holder.get("anchors", {})

            for dotted in anchors.get("modules", []):
                if not module_exists(root, dotted):
                    problems.add(where, f"module {dotted!r} does not exist")
            for package in anchors.get("packages", []):
                if not os.path.isdir(os.path.join(root, package)):
                    problems.add(where, f"package directory {package!r} does not exist")
            for file_path in anchors.get("files", []):
                if not os.path.isfile(os.path.join(root, file_path)):
                    problems.add(where, f"file {file_path!r} does not exist")

            for kind, noun in (
                ("endpoints", "endpoint"),
                ("tables", "table"),
                ("make_targets", "make target"),
                ("artifacts", "artifact"),
                ("pipeline_steps", "pipeline step"),
                ("gates", "gate"),
                ("feature_groups", "feature group"),
                ("targets", "performance target"),
            ):
                for value in anchors.get(kind, []):
                    if value not in known[kind]:
                        problems.add(where, f"{noun} {value!r} does not exist")

    for name, source in system_map["sources"].items():
        if source["endpoint"] not in known["endpoints"]:
            problems.add(f"source {name}", f"reads {source['endpoint']!r}, which is not served")


def check_reverse(system_map: Dict[str, Any], known: Dict[str, Set[str]], problems: Problems) -> None:
    """Everything the code can enumerate appears somewhere on the map."""
    on_map = collect_anchors(system_map["nodes"])

    # The harness draws its gates from the report's own registry, so the ids it lists are
    # the map's record of them; children_from names that, and the ids live on the node.
    for kind, noun in (
        ("endpoints", "endpoint"),
        ("pipeline_steps", "pipeline step"),
        ("gates", "gate"),
        ("feature_groups", "feature group"),
        ("targets", "performance target"),
        ("artifacts", "artifact kind"),
    ):
        problems.extend_missing("reverse", known[kind] - on_map[kind], noun)


def check(root: str) -> List[str]:
    problems = Problems()
    with open(os.path.join(root, MAP_PATH), encoding="utf-8") as fh:
        system_map = json.load(fh)

    generator = load_generator(root)
    gates = import_ml(root, "ml.xi.gates")
    performance = import_ml(root, "ml.xi.performance")

    known: Dict[str, Set[str]] = {
        "endpoints": served_endpoints(root, generator),
        "tables": live_tables(root),
        "make_targets": make_targets(root),
        "artifacts": artifact_kinds(root),
        "pipeline_steps": pipeline_step_ids(root),
        "gates": set(gates.REGISTRY),
        "feature_groups": feature_groups(root),
        "targets": {target.name for target in performance.TARGETS},
    }

    check_structure(system_map, problems)
    check_documents(root, system_map, problems)
    check_forward(root, system_map, known, problems)
    check_reverse(system_map, known, problems)
    return problems.items


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--root", default=".", help="repo root (default: .)")
    args = parser.parse_args()

    problems = check(os.path.abspath(args.root))
    if problems:
        print(f"ERROR: contracts/system-map.json disagrees with the code ({len(problems)} problems):", file=sys.stderr)
        for problem in problems:
            print(f"  - {problem}", file=sys.stderr)
        print(
            "\nThe map is not a picture; it is a contract. Fix the map, or restore what it names.",
            file=sys.stderr,
        )
        return 1
    print("contracts/system-map.json agrees with the code, in both directions")
    return 0


if __name__ == "__main__":
    sys.exit(main())
