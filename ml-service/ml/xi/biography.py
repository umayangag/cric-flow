"""Player biographies as the rating pass reads them (X-1b).

The archive says what happened, never who it happened to; X-1a acquired dates of birth from
Wikidata into ``player_biography`` for 85 % of appearances, and this module is the one
place the pass reads them. Only the date of birth crosses into ``ml``: bowling style,
batting hand and career end exist in Wikidata for 265, 18 and 3 of 13,662 players (plan
§ X-1a), so nothing here reads them and no feature is built on them.

Two readers, one shape -- a map from the player key the pass rates people by (the Cricsheet
registry identifier, ``player.external_id``) to a ``date``:

* Postgres: ``player_biography`` joined to ``player``, the production source;
* a CSV of ``player_key,birth_date`` for the archive path, written from the database by
  ``python -m ml.xi.biography --export <path>`` so an offline reproduction and the H-8
  parity run read the same facts the database holds.

A date of birth is static and knowable at any match date (H-21): the same value is right
for a 2010 row and a 2026 request, which is what lets one map serve the whole pass.
"""

from __future__ import annotations

import argparse
import csv
import logging
import os
import sys
from datetime import date
from typing import Dict, Iterable, Mapping, Optional, Sequence

import numpy as np

from ml.xi import contract as C

logger = logging.getLogger(__name__)

BirthDates = Dict[str, date]

#: Days in a year, for an age in years; the calendar's mean, not a tuned value.
DAYS_PER_YEAR = 365.25
CSV_COLUMNS = ("player_key", "birth_date")

# The player key expression is ``sources._player_key``'s -- the registry identifier, with
# the same name fallback -- so a biography lands on the slot its matches are rated under.
_BIRTH_DATES_SQL = """
SELECT COALESCE(p.external_id, 'name:' || p.player_name), b.birth_date
FROM player_biography b
JOIN player p ON p.id = b.player_id
WHERE b.birth_date IS NOT NULL
"""


def age_years(birth_date: date, on: date) -> float:
    """Age in years at ``on``: elapsed days over the mean year. Wikidata renders a
    year-precision birth date as the first of January (plan § X-1a), so an age here can be
    up to a year high for such a player; it is recorded, not smoothed."""
    return (on - birth_date).days / DAYS_PER_YEAR


def age_vectors(birth_dates: Mapping[str, date], player_keys: Sequence[str], on: date) -> Dict[str, np.ndarray]:
    """``contract.AGE_COLS`` for a list of players at one date: the age in years, and
    whether a date of birth exists at all. A player without one reads 0.0 in both columns
    -- a category of his own, never an imputed age."""
    ages = np.zeros(len(player_keys))
    known = np.zeros(len(player_keys))
    for i, key in enumerate(player_keys):
        birth = birth_dates.get(key)
        if birth is not None:
            ages[i] = age_years(birth, on)
            known[i] = 1.0
    return {"age": ages, "age_known": known}


def load_birth_dates_postgres(connection) -> BirthDates:
    """Every player with a stated date of birth, keyed as the rating pass keys him."""
    with connection.cursor() as cur:
        cur.execute(_BIRTH_DATES_SQL)
        rows = cur.fetchall()
    out: BirthDates = {str(key): _as_date(value) for key, value in rows}
    logger.info("birth dates: %d players from player_biography", len(out))
    return out


def load_birth_dates_csv(path: Optional[str]) -> BirthDates:
    """The archive path's biographies: the CSV ``--export`` writes. ``None`` is a run
    without biographies -- every age unknown -- and is logged as such rather than treated
    as an error, because the archive alone never carried one."""
    if not path:
        logger.warning("no birth-dates file given: every player's age reads as unknown")
        return {}
    out: BirthDates = {}
    with open(path, newline="") as fh:
        reader = csv.DictReader(fh)
        missing = [column for column in CSV_COLUMNS if column not in (reader.fieldnames or [])]
        if missing:
            raise ValueError(f"{path}: birth-dates CSV lacks column(s) {', '.join(missing)}")
        for row in reader:
            out[row["player_key"]] = date.fromisoformat(row["birth_date"])
    logger.info("birth dates: %d players from %s", len(out), path)
    return out


def write_birth_dates_csv(path: str, birth_dates: Mapping[str, date]) -> int:
    """Write the map as the CSV ``load_birth_dates_csv`` reads, sorted by key so two
    exports of the same table are byte-identical."""
    directory = os.path.dirname(os.path.abspath(path))
    os.makedirs(directory, exist_ok=True)
    with open(path, "w", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(CSV_COLUMNS)
        for key in sorted(birth_dates):
            writer.writerow([key, birth_dates[key].isoformat()])
    return len(birth_dates)


def count_known(birth_dates: Mapping[str, date], player_keys: Iterable[str]) -> int:
    """How many of ``player_keys`` have a date of birth -- the data-quality count the pass
    reports, so a run says how much of its population an age feature could see."""
    return sum(1 for key in player_keys if key in birth_dates)


def _as_date(value) -> date:
    return value if isinstance(value, date) else date.fromisoformat(str(value))


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument(
        "--export",
        required=True,
        metavar="PATH",
        help="write player_biography's birth dates (POSTGRES_* env vars) as a CSV the archive path reads",
    )
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    from ml.db import get_db_connection

    birth_dates = load_birth_dates_postgres(get_db_connection())
    written = write_birth_dates_csv(args.export, birth_dates)
    logger.info("wrote %d birth dates to %s (age bands: %s)", written, args.export, list(C.AGE_BANDS))
    return 0


if __name__ == "__main__":
    sys.exit(main())
