"""The reviewed mapping of franchise renames, for the rating pass (I-4).

A club that renames appears in Cricsheet under both names, so Elo, form, head-to-head and
venue familiarity -- all keyed by team -- restart at the boundary. ``configs/team_lineage.json``
says which names belong to one club; go-app writes it into ``opposition.canonical_id`` at
import, and this reads the same file so the Cricsheet-JSON source agrees with the database.

Both sources reading one file is the point. They disagreed three times already (§10.4 of
the rearchitecture plan), and a mapping applied on one side only would be a fourth.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Dict, Optional, Sequence, Tuple

logger = logging.getLogger(__name__)

FILE_NAME = "team_lineage.json"
ENV_VAR = "GO_APP_TEAM_LINEAGE"


class TeamLineage:
    """Maps a superseded (team name, gender) to the club's current name."""

    def __init__(self, renames: Sequence[Dict[str, str]] = ()):
        self._successor: Dict[Tuple[str, str], str] = {
            (r["from"], r["gender"]): r["to"] for r in renames if r.get("from") and r.get("to") and r.get("gender")
        }

    def __len__(self) -> int:
        return len(self._successor)

    def club(self, name: str, gender: str) -> str:
        """The name the club plays under now, which is ``name`` unless it renamed."""
        return self._successor.get((name, gender), name)


def _search_paths() -> Sequence[str]:
    here = os.path.dirname(os.path.abspath(__file__))
    return (
        os.path.join(here, "..", "..", "..", "configs", FILE_NAME),
        os.path.join("configs", FILE_NAME),
        os.path.join("..", "configs", FILE_NAME),
    )


def load(path: Optional[str] = None) -> TeamLineage:
    """Read the mapping, or return an empty one when there is no file.

    An absent mapping is not an error -- a deployment may have no renames recorded -- but it
    is logged, because a silently empty lineage looks exactly like a dataset with no
    rebrands in it. A file that exists and does not parse *is* an error: it is committed
    data, and running the pass against a broken version only buries the problem.
    """
    candidates = [path] if path else [os.environ.get(ENV_VAR) or "", *_search_paths()]
    for candidate in candidates:
        if candidate and os.path.exists(candidate):
            with open(candidate) as fh:
                mapping = TeamLineage(json.load(fh).get("renames") or [])
            logger.info("team lineage: %d renames from %s", len(mapping), candidate)
            return mapping
    logger.warning("team lineage: no %s found; renamed clubs will stay separate", FILE_NAME)
    return TeamLineage()
