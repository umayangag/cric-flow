"""Find franchise renames: one club that became two opposition rows (I-4).

Throwaway, by design. The checklist's argument is that a heuristic which runs on every
import would silently merge two real clubs the first time a coincidence cleared its
threshold, so the mapping this produces is reviewed by hand and committed as data --
`configs/team_lineage.json` -- rather than recomputed.

The rule: **no temporal overlap, plus roster carry-over across the boundary.** A club that
renames stops playing under the old name the day it starts playing under the new one, and
takes its squad with it.

What does not work, and why the rule is shaped this way:

* String similarity pairs `Barbados Tridents` with `Barbados Royals` (a real rename) and
  `Birmingham Bears` with `Birmingham Phoenix` (two teams in two competitions, coexisting).
* Whole-history rosters are too diluted: Royal Challengers Bangalore fielded 180 people
  over sixteen years against Bengaluru's 69, an overlap of 41% -- below anything usable.
  Comparing only the innings either side of the boundary gives a clean signal.

Usage (from the repository root, with POSTGRES_* in the environment):
  ml-service/.venv/bin/python scripts/experiments/xi/team_lineage_candidates.py
"""

from __future__ import annotations

import argparse
import os
import sys
from collections import defaultdict
from typing import Dict, List, Set, Tuple

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "..", "ml-service"))

from ml.db import get_db_connection  # noqa: E402

BOUNDARY_MATCHES = 15
MIN_OVERLAP = 0.40
MIN_MATCHES = 5

SQUADS_SQL = """
SELECT mp.opposition_id, m.match_date, mp.match_id, mp.player_id
FROM match_player mp
JOIN match m ON m.match_id = mp.match_id
ORDER BY mp.opposition_id, m.match_date, mp.match_id
"""

TEAMS_SQL = "SELECT id, opposition_name, gender FROM opposition"


def load(connection):
    with connection.cursor() as cur:
        cur.execute(TEAMS_SQL)
        teams = {row[0]: (row[1], row[2]) for row in cur.fetchall()}
        cur.execute(SQUADS_SQL)
        rows = cur.fetchall()
    squads: Dict[int, Dict[Tuple, Set[int]]] = defaultdict(lambda: defaultdict(set))
    for opposition_id, match_date, match_id, player_id in rows:
        squads[opposition_id][(match_date, match_id)].add(player_id)
    return teams, squads


def boundary_rosters(squads) -> Dict[int, dict]:
    """Each team's first and last N squads, and the dates it played between."""
    out = {}
    for opposition_id, by_match in squads.items():
        keys = sorted(by_match)
        first = set().union(*[by_match[k] for k in keys[:BOUNDARY_MATCHES]])
        last = set().union(*[by_match[k] for k in keys[-BOUNDARY_MATCHES:]])
        out[opposition_id] = {
            "first": first,
            "last": last,
            "first_date": keys[0][0],
            "last_date": keys[-1][0],
            "matches": len(keys),
        }
    return out


def overlap(a: Set[int], b: Set[int]) -> float:
    """Share of the smaller roster the two have in common."""
    if not a or not b:
        return 0.0
    return len(a & b) / min(len(a), len(b))


def candidates(teams, rosters) -> List[tuple]:
    found = []
    for old_id, old in rosters.items():
        for new_id, new in rosters.items():
            if old_id == new_id:
                continue
            if teams[old_id][1] != teams[new_id][1]:
                continue  # a men's side does not become a women's side
            if old["matches"] < MIN_MATCHES or new["matches"] < MIN_MATCHES:
                continue
            if old["last_date"] >= new["first_date"]:
                continue  # they coexisted: two clubs, not one renamed
            score = overlap(old["last"], new["first"])
            if score >= MIN_OVERLAP:
                found.append((score, old_id, new_id))
    return sorted(found, reverse=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--min-overlap", type=float, default=MIN_OVERLAP)
    args = parser.parse_args()

    teams, squads = load(get_db_connection())
    rosters = boundary_rosters(squads)
    print(f"{len(teams)} teams, {len(rosters)} with squads\n")
    print(f"{'overlap':>8}  {'gender':<7} {'predecessor':<32} {'successor':<32} {'gap':>6}")
    for score, old_id, new_id in candidates(teams, rosters):
        if score < args.min_overlap:
            continue
        old_name, gender = teams[old_id]
        new_name, _ = teams[new_id]
        gap = (rosters[new_id]["first_date"] - rosters[old_id]["last_date"]).days
        print(
            f"{score:8.0%}  {gender:<7} "
            f"{old_name + ' (' + str(rosters[old_id]['matches']) + ')':<32} "
            f"{new_name + ' (' + str(rosters[new_id]['matches']) + ')':<32} {gap:>5}d"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
