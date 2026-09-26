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
    "decided_matches_without_deliveries",
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

    # The matches read split by format code, zero-filled over every canonical code so that
    # two sources always carry the same keys. It is the only count that can see the format
    # taxonomy, and it is here because for one batch nothing could: the archive path's
    # ``detect_format`` read a list of international sides from ``go-app/config.json`` that
    # IMPORT-09 had deleted, so 5,700 international T20s were club T20 on that path alone --
    # and every total stayed identical, because a match reclassified is still a match read.
    # ``make xi-parity`` saw only the downstream symptoms (two stakes counts, a handful of
    # player keys) and could not name the cause. A fact about the cricket, compared across
    # sources and not gated: it moves whenever the archive grows.
    matches_read_by_format: Dict[str, int] = field(default_factory=dict)

    undecided_matches: int = 0

    # Of the undecided matches, the draws and the ties nobody broke: the ones form reads
    # as half a win for each side (``MatchRecord.drawn_or_tied``). It is the only count
    # that can see ``match.result``, and it exists because the Postgres source hard-coded
    # the field to None for years while the archive path read it, so ``team_form_diff``
    # had two definitions and every other count still agreed (FEAT-04). Not gated by the
    # doubling rule: it is a fact about the cricket, and it goes from zero to a quarter of
    # all Tests the first time a pre-0017 database is re-imported.
    drawn_or_tied_matches: int = 0

    # Decided matches the source handed over with no deliveries at all: a forfeit, a result
    # awarded without play, or -- the case worth gating -- an importer that kept the match's
    # result and squads and lost its ball events (FEAT-03). The pass keeps the win row,
    # whose label is real, and builds no player rows and no innings outcomes, because
    # "what he did" is unobserved and a nought would train the performance models on it.
    # Zero on the current dataset from either source; gated by the doubling rule for the
    # same reason as ``unknown_player_keys``, and compared across sources because the
    # archive path drops a file with no innings while the database offers a match with an
    # innings row and no balls, so the two can disagree here and nowhere else.
    decided_matches_without_deliveries: int = 0

    # Byes, leg-byes and penalty runs over every delivery the pass read: the part of the
    # runs off the bat's end that the bowler is not charged (``Deliveries.runs_bowler``,
    # FEAT-08). It is the one count that can see the extras breakdown, and it is here for
    # the same reason as ``drawn_or_tied_matches``: a source that charges the bowler the
    # whole total agrees with the other on every other count, so without it the parity
    # check could not tell the two definitions of ``bowl_rate`` apart. A fact about the
    # cricket and not gated: it reads zero until a pre-0018 database is re-imported.
    runs_not_charged_to_bowler: int = 0

    # Runs off every delivery the pass read (``Deliveries.runs_total``), the extras
    # included. The count the three around it are differences from, and the only one that
    # moves when a source rewrites what a ball scored without changing how it was charged.
    # It is here because the dataset digest folds these counts in (EVAL-12): an undecided
    # match produces no row for the digest to read, so this aggregate is the only thing
    # that sees a re-import rewriting the scorecard of a draw. A fact about the cricket
    # and not gated -- it grows with every import.
    runs_scored: int = 0

    # Wides over every delivery the pass read: the deliveries no batter faced
    # (``Deliveries.faced``, IMPORT-05). Here for the same reason as the runs count: a
    # source that counts every delivery as faced agrees with the other on every other
    # count, so without it the parity check could not tell the two definitions of
    # ``balls_faced`` apart. A fact about the cricket and not gated: it reads zero until a
    # pre-0018 database is re-imported.
    deliveries_not_faced: int = 0

    # Wickets lost over every delivery the pass read (``Deliveries.wicket``, IMPORT-06):
    # every wicket record the source holds that the vocabulary calls a dismissal, on every
    # ball, the second wicket of a delivery included. Here because the database used to
    # keep one wicket per delivery and the archive path mirrored it, so the two sources
    # agreed on every count while both were short seventeen wickets; a source that drops
    # a wicket record now differs from the other here. A fact about the cricket and not
    # gated.
    dismissals: int = 0

    # A person named on both sides of one match. Cricsheet's registry is keyed by name
    # within a file, so two namesakes in one match collapse into one identifier and the
    # source cannot say which side each delivery belongs to. Two files in the current
    # dataset do this. A jump means either a new namesake or a broken registry.
    namesake_sides: int = 0

    # Sides still of more than eleven once the replacements are taken out (FEAT-02): a
    # source that lists twelve and records no replacement for the twelfth. Cricsheet lists
    # everyone who took the field, and 1,365 of 45,810 sides in the current archive are
    # over eleven; 1,342 of those carry a `replacements.match` entry naming the man who came
    # in, so the pass leaves him out and the side is an eleven. The 23 that do not (21 Syed
    # Mushtaq Ali Trophy 2022 sides of twelve, 2 Women's T20 Challenge 2018 sides of
    # thirteen) and the one whose entry names a player the other side lists (1537342)
    # stay oversized and are what this counts. A sudden doubling would mean the squad parse
    # had started collecting somebody else, or a source had stopped seeing its replacements.
    oversized_squads: int = 0

    # Players who joined a side after the match started and were left out of its eleven:
    # the `in` of a `replacements.match` entry on the archive path, `match_player.
    # is_replacement` (migration 0020) on the database. Here because the two sources
    # derive it separately -- the importer in Go, the archive path in Python -- and a source
    # that flags nobody, or flags the man who went out as well as the man who came in,
    # agrees with the other on every count that counts elevens; `make xi-parity` compares
    # it. A fact about the cricket and not gated: it reads zero until a pre-0020 database
    # is re-imported.
    replacement_players: int = 0

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

    # The home flag's reach (FEAT-05): matches where exactly one side read as at home, as
    # the pass read them; and the toss's: matches the source recorded no toss for, which
    # read ``TOSS_UNKNOWN``. Facts about the cricket and the curated venue table, compared
    # across sources and not gated.
    matches_with_one_home_side: int = 0
    matches_without_toss: int = 0

    # Matches whose last day is after their first (FEAT-09): the ones the pass folds in at
    # the close of that last day rather than the first. The only count that can see
    # ``match.match_end_date`` (migration 0024), so it is what tells a database migrated
    # but not re-imported -- every end date NULL, every match one day long -- from the
    # archive, where 3,171 of 22,905 run longer. A fact about the cricket and not gated.
    multi_day_matches: int = 0

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
