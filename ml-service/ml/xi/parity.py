"""Check that the database holds what the Cricsheet archive holds (H-15).

The rating pass has two sources -- the go-app database and the raw Cricsheet files -- and
they are supposed to describe the same cricket. Twice they did not, and both times the
difference sat unnoticed because nobody ran the two side by side:

* match identity was a hash of date and team names, so 618 files shared an id in 309 pairs
  and the database held 22,425 matches for 22,734 files;
* the JSON path keyed 469 unnamed substitute fielders on the empty name, inventing a
  cricketer with a fielding record drawn from 365 matches.

Both are a one-line difference in a count. This runs both sources and prints the counts
beside each other, and exits non-zero when they disagree.

It is a separate command rather than part of a retrain because it needs the archive as well
as the database, and a production retrain has only the database. Run it whenever the
importer or either source changes.

Usage (from ml-service/):
  python -m ml.xi.parity --cricsheet-dir ../data/go-app/cricsheet
"""

from __future__ import annotations

import argparse
import logging
import sys
from typing import Dict, List, Optional, Sequence, Set, Tuple

from ml.xi import contract as C
from ml.xi.builder import BuildResult, build

logger = logging.getLogger(__name__)


#: Counts whose value is a property of the cricket rather than of the store it came from.
#: ``out_of_scope_matches`` is left out: the two sources filter at different points --
#: Postgres in SQL, the archive after parsing -- so it legitimately differs.
_COMPARED_COUNTS = (
    "offered_matches",
    "unusable_matches",
    "matches_read",
    "undecided_matches",
    "namesake_sides",
    "oversized_squads",
    "unknown_player_keys",
    "player_keys",
    "team_keys",
    "players_with_birth_date",
)


def compare(postgres: BuildResult, cricsheet: BuildResult) -> List[str]:
    """Differences between two passes over what should be the same cricket.

    Every comparable count is checked rather than a chosen few, so a count added to
    ``DataQuality`` later is compared without anyone remembering to come back here.
    """
    differences: List[str] = []

    pg, cs = postgres.quality.as_dict(), cricsheet.quality.as_dict()
    for name in _COMPARED_COUNTS:
        if pg[name] != cs[name]:
            differences.append(f"{name}: postgres {pg[name]}, cricsheet {cs[name]}")
    if len(postgres.frame) != len(cricsheet.frame):
        differences.append(f"training rows: postgres {len(postgres.frame)}, cricsheet {len(cricsheet.frame)}")

    only_pg = sorted(_player_keys(postgres) - _player_keys(cricsheet))
    only_cs = sorted(_player_keys(cricsheet) - _player_keys(postgres))
    if only_pg or only_cs:
        differences.append(
            f"player keys differ: {len(only_pg)} only in postgres {_sample(only_pg)}, "
            f"{len(only_cs)} only in cricsheet {_sample(only_cs)}"
        )
    return differences


def _player_keys(result: BuildResult) -> Set[str]:
    return {str(key) for key in result.state.players.key_to_slot}


def _sample(keys: Sequence[str], limit: int = 5) -> str:
    """A few keys to start looking at; the whole list is unreadable and rarely needed.

    Quoted, because the keys worth seeing here are the odd ones -- the difference that
    started this was the key ``name:``, which unquoted reads as a label rather than a value.
    """
    if not keys:
        return "[]"
    shown = ", ".join(repr(k) for k in keys[:limit])
    return f"[{shown}{', ...' if len(keys) > limit else ''}]"


def counts_table(postgres: BuildResult, cricsheet: BuildResult) -> str:
    """Both passes' data-quality counts, side by side, for a human to read."""
    rows: List[Tuple[str, object, object]] = [("count", "postgres", "cricsheet")]
    pg, cs = postgres.quality.as_dict(), cricsheet.quality.as_dict()
    for name in pg:
        if name != "source":
            rows.append((name, pg[name], cs.get(name)))
    width = max(len(str(r[0])) for r in rows)
    return "\n".join(f"  {str(name):<{width}}  {str(a):>10}  {str(b):>10}" for name, a, b in rows)


def build_both(
    cricsheet_dir: str, formats: Sequence[str], birth_dates_path: Optional[str] = None
) -> Tuple[BuildResult, BuildResult]:
    from ml.db import get_db_connection
    from ml.xi.sources import CricsheetJsonSource, PostgresSource
    from ml.xi.train import _international_teams_from_config

    logger.info("rating pass over postgres")
    postgres = build(PostgresSource(get_db_connection(), formats))
    logger.info("rating pass over %s", cricsheet_dir)
    cricsheet = build(
        CricsheetJsonSource(
            cricsheet_dir, _international_teams_from_config(), formats, birth_dates_path=birth_dates_path
        )
    )
    return postgres, cricsheet


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--cricsheet-dir", required=True, help="directory of Cricsheet JSON files")
    p.add_argument("--formats", nargs="+", default=list(C.FORMAT_CODES))
    p.add_argument(
        "--birth-dates",
        default=None,
        help="CSV of player_key,birth_date for the archive path (python -m ml.xi.biography --export)",
    )
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    postgres, cricsheet = build_both(args.cricsheet_dir, args.formats, args.birth_dates)
    logger.info("data-quality counts:\n%s", counts_table(postgres, cricsheet))
    differences = compare(postgres, cricsheet)
    if differences:
        for difference in differences:
            logger.error("source parity: %s", difference)
        logger.error("the database and the archive describe different cricket (%d differences)", len(differences))
        return 1
    logger.info("source parity: the database and the archive agree")
    return 0


def summary(postgres: BuildResult, cricsheet: BuildResult) -> Dict[str, object]:
    """The comparison as data, for a caller that wants to record it."""
    return {
        "postgres": postgres.quality.as_dict(),
        "cricsheet": cricsheet.quality.as_dict(),
        "differences": compare(postgres, cricsheet),
    }


if __name__ == "__main__":
    sys.exit(main())
