#!/usr/bin/env python3
"""Regenerate `.junie/skills/` from `.cursor/skills/`.

The two directories hold the same seven workflows for two different assistants, in
two different file formats:

- `.cursor/skills/<name>/SKILL.md` — YAML frontmatter (`name`, `description`,
  `disable-model-invocation`) followed by the body.
- `.junie/skills/<name>/SKILL.md` — no frontmatter; `.junie/README.md` specifies the
  file must open with a trigger line, `When the user says "/<name>" ...`.

They cannot be one file, and symlinking would break one tool. So `.cursor` is the
source and `.junie` is generated: strip the frontmatter, prepend a trigger line built
from the frontmatter's own `description`.

This exists because the two had drifted **behaviourally**, not just cosmetically — the
Junie copy of `run-check-all-incremental` still described three components and omitted
the `frontend-backend-sync-check` step that CI enforces. Anyone running it there would
have skipped a check.

Usage:
    python scripts/sync-junie-skills.py            # regenerate
    python scripts/sync-junie-skills.py --check    # exit 1 if out of date
"""

from __future__ import annotations

import argparse
import os
import sys

CURSOR_DIR = ".cursor/skills"
JUNIE_DIR = ".junie/skills"

GENERATED_NOTE = (
    "<!-- Generated from ../../.cursor/skills/{name}/SKILL.md by "
    "scripts/sync-junie-skills.py. Edit the Cursor copy, then re-run the script. -->"
)


def split_frontmatter(text: str) -> tuple[dict[str, str], str]:
    """Return (frontmatter mapping, body). Missing frontmatter yields ({}, text)."""
    if not text.startswith("---\n"):
        return {}, text
    end = text.find("\n---\n", 4)
    if end == -1:
        return {}, text
    raw, body = text[4:end], text[end + 5 :]
    meta: dict[str, str] = {}
    key = None
    for line in raw.splitlines():
        if line[:1].strip() and ":" in line:
            key, _, value = line.partition(":")
            key = key.strip()
            meta[key] = value.strip()
        elif key and line.startswith((" ", "\t")):
            meta[key] += " " + line.strip()
    return meta, body.lstrip("\n")


def render_junie(name: str, text: str) -> str:
    meta, body = split_frontmatter(text)
    description = meta.get("description", "").strip()
    trigger = f'When the user says "/{name}"'
    if description:
        trigger += f" — {description}"
    return f"{GENERATED_NOTE.format(name=name)}\n\n{trigger}\n\n{body.rstrip()}\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--check", action="store_true", help="exit 1 if any generated file is out of date")
    args = parser.parse_args()

    if not os.path.isdir(CURSOR_DIR):
        print(f"error: {CURSOR_DIR} not found (run from the repo root)", file=sys.stderr)
        return 2

    stale: list[str] = []
    written = 0
    for name in sorted(os.listdir(CURSOR_DIR)):
        source = os.path.join(CURSOR_DIR, name, "SKILL.md")
        if not os.path.isfile(source):
            continue
        rendered = render_junie(name, open(source, encoding="utf-8").read())
        target = os.path.join(JUNIE_DIR, name, "SKILL.md")
        current = open(target, encoding="utf-8").read() if os.path.isfile(target) else None
        if current == rendered:
            continue
        if args.check:
            stale.append(target)
            continue
        os.makedirs(os.path.dirname(target), exist_ok=True)
        open(target, "w", encoding="utf-8").write(rendered)
        written += 1
        print(f"  wrote {target}")

    # A Junie skill with no Cursor counterpart would silently never be regenerated.
    orphans = [
        n
        for n in sorted(os.listdir(JUNIE_DIR))
        if os.path.isdir(os.path.join(JUNIE_DIR, n)) and not os.path.isdir(os.path.join(CURSOR_DIR, n))
    ]

    if args.check:
        if stale or orphans:
            if stale:
                print(
                    "ERROR: .junie/skills is out of date with .cursor/skills:\n"
                    + "\n".join(f"  {p}" for p in stale)
                    + "\n\nRun: python scripts/sync-junie-skills.py",
                    file=sys.stderr,
                )
            if orphans:
                print(
                    "ERROR: .junie/skills entries with no .cursor/skills source:\n"
                    + "\n".join(f"  {n}" for n in orphans)
                    + "\n\nAdd the Cursor copy, or delete the Junie one.",
                    file=sys.stderr,
                )
            return 1
        print(".junie/skills is up to date with .cursor/skills")
        return 0

    print(f"regenerated {written} file(s)" if written else "already up to date")
    if orphans:
        print("warning: Junie skills with no Cursor source: " + ", ".join(orphans), file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
