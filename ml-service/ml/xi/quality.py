"""The data-quality gate for the rating pass (H-15).

Two defects lived in this pipeline for years because nothing counted anything. The
database held 22,425 matches for 22,734 source files, and the JSON rating source carried a
fictional cricketer assembled from 469 unnamed substitute fielders. Both were found by
someone comparing two numbers by hand; neither would have survived a run that printed them.

So the pass counts what it drops and what it finds odd, the counts go into the run's
report, and the next run is checked against them. The rules are deliberately dull:

* **Everything is accounted for.** A source offers N matches; N must equal the matches it
  yielded plus the ones it left out of scope plus the ones it could not use. A source that
  drops a match for a reason it does not name fails here.
* **Nothing doubles quietly.** Any quality count more than twice the previous run's fails,
  as does any count that was zero and is not any more. Data does not usually get twice as
  broken between two runs of the same pipeline; when it does, something upstream changed.

The gate reports rather than guesses: it says which count moved and by how much, and the
operator decides whether the new number is right and should become the baseline.
"""

from __future__ import annotations

import json
import logging
import os
from dataclasses import asdict, dataclass, field
from typing import Dict, List, Optional

logger = logging.getLogger(__name__)

REPORT_KEY = "data_quality"

#: The accepted counts, kept beside the artifacts. Separate from the run report on purpose:
#: the report always describes the run that just happened, while this only ever holds counts
#: a run passed the gate with (or an operator accepted). If a failing run updated the
#: baseline, running it twice would clear the gate, which is not a gate.
BASELINE_NAME = "xi_data_quality_baseline.json"

# Counts that describe how broken the data is, as opposed to how much of it there is.
# These are the ones the doubling rule applies to; matches_read is expected to grow.
_GATED_COUNTS = (
    "unusable_matches",
    "undecided_matches",
    "namesake_sides",
    "oversized_squads",
    "unknown_player_keys",
)


@dataclass
class DataQuality:
    """What one rating pass saw. Written to the run report and compared to the last one."""

    source: str = ""
    offered_matches: int = 0
    out_of_scope_matches: int = 0
    unusable_matches: int = 0
    matches_read: int = 0
    undecided_matches: int = 0

    # A person named on both sides of one match. Cricsheet's registry is keyed by name
    # within a file, so two namesakes in one match collapse into one identifier and the
    # source cannot say which side each delivery belongs to. Two files in the current
    # dataset do this. A jump means either a new namesake or a broken registry.
    namesake_sides: int = 0

    # Sides of more than eleven: concussion and injury replacements, which Cricsheet lists
    # in full. 1,357 of 45,468 sides in the current dataset, so this is a fact about the
    # game and not an error -- but a sudden doubling would mean the squad parse had
    # started collecting somebody else.
    oversized_squads: int = 0

    # Player keys the source could not resolve to a person: "name:..." fallbacks. Zero on
    # the current dataset from either source, which is what makes it worth gating.
    unknown_player_keys: int = 0

    player_keys: int = 0

    # Distinct team keys the pass saw. A club that renamed is one key from both sources --
    # ``opposition.canonical_id`` in the database, ``configs/team_lineage.json`` in the
    # archive -- so a lineage applied on one side only shows up here as a difference.
    team_keys: int = 0

    # Players the pass rated who have a date of birth (X-1b): how much of the population an
    # age feature could see. The archive path reads a CSV exported from the database, so
    # the two sources are expected to agree here as they do on every other count.
    players_with_birth_date: int = 0

    # Match stakes (X-3), the three numbers that say how far the derivation reaches: how
    # many matches the archive places in their competition at all, how many are knockouts,
    # and how many have a reconstructible table. They are compared across sources because
    # they are the only thing that would notice the importer dropping an event field: the
    # database and the archive would still describe the same cricket, and one of them
    # would silently know less about it.
    matches_with_stage_label: int = 0
    knockout_matches: int = 0
    matches_with_reconstructible_table: int = 0
    dead_rubber_matches: int = 0

    def as_dict(self) -> Dict[str, object]:
        return asdict(self)

    @property
    def accounted_matches(self) -> int:
        return self.out_of_scope_matches + self.unusable_matches + self.matches_read


@dataclass
class GateResult:
    """Why the gate failed, or that it did not."""

    failures: List[str] = field(default_factory=list)
    baseline: Optional[Dict[str, object]] = None

    @property
    def passed(self) -> bool:
        return not self.failures


def check(current: DataQuality, previous: Optional[Dict[str, object]] = None) -> GateResult:
    """Apply the gate to one pass's counts, against the previous run's if there is one."""
    failures: List[str] = []

    if current.offered_matches != current.accounted_matches:
        failures.append(
            f"{current.offered_matches - current.accounted_matches} of {current.offered_matches} matches "
            f"the source offered are unaccounted for "
            f"(read {current.matches_read}, out of scope {current.out_of_scope_matches}, "
            f"unusable {current.unusable_matches}) -- a match is being dropped for a reason nothing names"
        )
    if current.matches_read == 0:
        failures.append("the source yielded no matches")

    if previous is None:
        logger.info("data-quality gate: no previous run to compare against; recording a baseline")
        return GateResult(failures=failures, baseline=current.as_dict())

    for name in _GATED_COUNTS:
        was = int(previous.get(name, 0) or 0)
        now = int(getattr(current, name))
        if was == 0 and now > 0:
            failures.append(f"{name} was 0 and is now {now}")
        elif was > 0 and now > 2 * was:
            failures.append(f"{name} more than doubled: {was} -> {now}")
    return GateResult(failures=failures, baseline=current.as_dict())


def load_baseline(artifacts_dir: str) -> Optional[Dict[str, object]]:
    """The accepted counts in an artifacts directory, or None on the first run."""
    path = os.path.join(artifacts_dir, BASELINE_NAME)
    if not os.path.exists(path):
        return None
    try:
        with open(path) as fh:
            loaded = json.load(fh)
    except (OSError, ValueError) as err:
        # A baseline we cannot read is not a reason to refuse to train; it is a reason to
        # say this run has nothing to be compared against.
        logger.warning("could not read the data-quality baseline at %s: %s", path, err)
        return None
    return loaded if isinstance(loaded, dict) else None


def save_baseline(artifacts_dir: str, counts: DataQuality) -> str:
    """Record counts as the ones future runs are checked against."""
    path = os.path.join(artifacts_dir, BASELINE_NAME)
    with open(path, "w") as fh:
        json.dump(counts.as_dict(), fh, indent=2)
    return path
