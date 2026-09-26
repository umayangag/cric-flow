"""Run the rating pass over a source: training frame out, serving state out."""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Tuple

import pandas as pd

from ml.xi import contract as C
from ml.xi import runs
from ml.xi.biography import count_known
from ml.xi.display_regression import UNRECORDED_LEVEL
from ml.xi.quality import DataQuality
from ml.xi.ratings import RatingState
from ml.xi.rows import build_match_rows
from ml.xi.sources import MatchSource, SourceCounts

logger = logging.getLogger(__name__)


@dataclass
class BuildResult:
    frame: pd.DataFrame  # one row per match with a decided winner (plus metadata columns)
    player_frame: pd.DataFrame  # one row per (match, player) for the same matches, all XI players (H-20)
    state: RatingState  # as-of the end of the source: what the serving path predicts from
    n_undecided: int  # matches folded into the state but not usable as a training row
    quality: DataQuality  # what the pass dropped and what it found odd (H-15)

    def dataset_digest(self) -> Tuple[str, Dict[str, Any]]:
        """The digest of the cricket this pass consumed, and what the digest covered.

        It used to read ``match_id|match_date`` off each decided match and nothing else.
        That answers "the same list of fixtures?", which is not the question a run's
        provenance has to answer: a re-import that rewrote every squad and every delivery
        -- the thing this repository does routinely -- left the fixture list untouched and
        so produced a byte-identical sha (EVAL-12). Two runs trained on different cricket
        could not be told apart, which is the whole use the field has.

        So three kinds of line go in, and each closes one of the holes:

        * one per decided match: its identity, the sides, the venue and the label the win
          models train on, so a changed result or a re-pointed team is visible;
        * one per player-match row: who was in the XI and what the deliveries did to him
          (``DIGEST_PLAYER_COLS``), so a changed squad changes the set of lines and a
          changed delivery changes a line;
        * one per pass-level count (``DIGEST_PASS_COUNTS``), which are summed over *every*
          match the pass read -- the undecided ones included. Those produce no rows at all
          yet still fold into the ratings, so without this last kind a re-import that
          touched only draws would again be invisible.

        All of it is already in memory when the pass ends; nothing is re-read and no
        delivery is scanned a second time (0.96 s on the full archive, against a 216 s
        pass). The numeric columns are cast to one dtype before they are rendered, so the
        digest cannot move because pandas inferred ``int64`` one day and ``float64`` the
        next.

        What it still cannot see: an undecided match is covered only by those pass-level
        totals, not match by match, because the pass keeps no row for one. A re-import that
        moved runs between two players inside a single draw and changed nothing else would
        pass. Closing that would mean digesting every delivery the pass reads, which is the
        one scan this deliberately avoids; the aggregate is what a re-import -- which
        changes a definition and therefore the totals -- actually trips.
        """
        match_lines = _digest_lines(
            "match",
            self.frame.match_id,
            self.frame.match_date.dt.strftime("%Y-%m-%d"),
            self.frame.format_code,
            self.frame.gender,
            self.frame.team1,
            self.frame.team2,
            self.frame.venue,
            self.frame[C.TARGET_COL].astype("float64"),
        )
        player_lines = _digest_lines(
            "xi",
            self.player_frame.match_id,
            self.player_frame.player_key,
            *(self.player_frame[column].astype("float64") for column in DIGEST_PLAYER_COLS),
        )
        counts = self.quality.as_dict()
        pass_lines = [f"pass|{name}|{counts[name]}" for name in DIGEST_PASS_COUNTS]
        coverage: Dict[str, Any] = {
            "scheme": runs.DATASET_DIGEST_SCHEME,
            "matches": int(len(self.frame)),
            "player_rows": int(len(self.player_frame)),
            "pass_counts": len(pass_lines),
            "undecided_matches": int(self.n_undecided),
        }
        return runs.dataset_sha([*match_lines, *player_lines, *pass_lines]), coverage


#: What the dataset digest reads off each player-match row, beside the match and the player:
#: everything the deliveries did to him. A re-import that changes a squad changes the set of
#: rows; one that changes a delivery changes these numbers. The as-of feature columns are
#: deliberately left out -- they are a function of the ratings, so digesting them would make
#: the digest of the data partly a digest of the model (EVAL-12).
DIGEST_PLAYER_COLS: Tuple[str, ...] = ("side", *C.PLAYER_MATCH_TARGET_COLS)

#: The pass-level counts the digest folds in (``ml.xi.quality``). Every one of them is
#: summed over every match the pass read, undecided matches included, which is the only
#: cover the digest has for matches that fold into the ratings without producing a row.
#: Listed rather than derived from ``DataQuality``'s fields so that a count added there
#: does not silently change what a digest means; what is deliberately left out is the
#: source's own bookkeeping (how many files it offered, skipped and could not use), which
#: differs between the archive and the database for identical cricket.
DIGEST_PASS_COUNTS: Tuple[str, ...] = (
    "matches_read",
    "undecided_matches",
    "drawn_or_tied_matches",
    "decided_matches_without_deliveries",
    "runs_scored",
    "runs_not_charged_to_bowler",
    "deliveries_not_faced",
    "dismissals",
    "namesake_sides",
    "oversized_squads",
    "replacement_players",
    "unknown_player_keys",
    "player_keys",
    "team_keys",
    "players_with_birth_date",
    "matches_with_stage_label",
    "knockout_matches",
    "matches_with_reconstructible_table",
    "dead_rubber_matches",
)


def _digest_lines(kind: str, *columns: pd.Series) -> List[str]:
    """One ``|``-separated line per row of ``columns``, built vectorised.

    A Python loop over the half-million player-match rows is the only part of the digest
    that could cost anything measurable; pandas' string concatenation keeps the whole
    digest inside a second on the full archive.
    """
    joined = pd.Series(kind, index=columns[0].index, dtype="object")
    for column in columns:
        joined = joined + "|" + column.astype(str)
    return joined.tolist()


META_COLS: List[str] = [
    "match_id",
    "match_date",
    "format_code",
    "gender",
    "team1",
    "team2",
    "venue",
    "competition_level",
    C.TARGET_COL,
]


def build(
    source: MatchSource,
    progress: Optional[Callable[[int], None]] = None,
    gender_split_context: bool = False,
    age_aware_cold_start: bool = C.AGE_AWARE_COLD_START,
) -> BuildResult:
    """Run the pass. Matches are folded into the state at *day close*: every match on a date
    reads features from prior dates only, then the whole day is applied. Within a date the
    source's order is by id, not by start time, so sequential updates would let a match see
    the result of a same-day match it may in fact have preceded. 78% of matches share a date
    with another in the same format; the cost of the strict rule is <= 0.003 AUC.

    The day that closes is the match's *last* day (FEAT-09): a Test is folded once its
    fifth day is over, not its first, so a fixture played while it is on reads a state
    without it. 3,171 of the archive's 22,905 matches run over more than one day and 10,656
    start inside another's span, 1,386 of them in the same format."""
    state = RatingState(
        gender_split_context=gender_split_context,
        birth_dates=source.birth_dates(),
        age_aware_cold_start=age_aware_cold_start,
        venue_countries=source.venue_countries(),
    )
    rows = []
    player_rows = []
    n_undecided = 0
    n_drawn_or_tied = 0
    n_decided_without_deliveries = 0
    n_seen = 0
    matches_read_by_format = {code: 0 for code in C.FORMAT_CODES}
    matches_read_by_level = {level: 0 for level in (*C.COMPETITION_LEVELS, UNRECORDED_LEVEL)}
    team_keys = set()
    namesake_sides = 0
    oversized_squads = 0
    replacement_players = 0
    runs_not_charged_to_bowler = 0
    runs_scored = 0
    deliveries_not_faced = 0
    dismissals = 0
    stakes_counts = {"stage": 0, "knockout": 0, "table": 0, "dead": 0}
    matches_with_one_home_side = 0
    matches_without_toss = 0
    multi_day_matches = 0
    pending: List = []
    current_date = None
    for i, match in enumerate(source.iter_matches()):
        n_seen += 1
        matches_read_by_format[match.format_code] = matches_read_by_format.get(match.format_code, 0) + 1
        level = match.competition_level if match.competition_level in C.COMPETITION_LEVELS else UNRECORDED_LEVEL
        matches_read_by_level[level] += 1
        team_keys.update((match.team1, match.team2))
        namesake_sides += _namesake_sides(match)
        oversized_squads += _oversized_squads(match)
        replacement_players += len(match.replacements)
        runs_not_charged_to_bowler += _runs_not_charged_to_bowler(match)
        runs_scored += _runs_scored(match)
        deliveries_not_faced += _deliveries_not_faced(match)
        dismissals += _dismissals(match)
        stakes_counts["stage"] += int(match.stakes.stage_known)
        stakes_counts["knockout"] += int(match.stakes.is_knockout)
        stakes_counts["table"] += int(match.stakes.dead_rubber_known)
        stakes_counts["dead"] += int(match.stakes.dead_rubber)
        if current_date is not None and match.match_date < current_date:
            raise ValueError(f"source is not in date order: {match.match_id} ({match.match_date}) after {current_date}")
        if current_date is not None and match.match_date != current_date:
            pending = state.fold_finished(pending, match.match_date)
        current_date = match.match_date
        multi_day_matches += int(match.last_day > match.match_date)
        # Read before the day is folded, as the row is: the home flag is as-of (FEAT-05).
        matches_with_one_home_side += int(sum(state.home_sides(match)) == 1.0)
        matches_without_toss += int(match.toss_won_by_team1 is None)
        if match.outcome is not None:
            win_row, match_player_rows = build_match_rows(state, match)
            rows.append(win_row)
            player_rows.extend(match_player_rows)
            n_decided_without_deliveries += int(not len(match.deliveries))
        else:
            n_undecided += 1
            n_drawn_or_tied += int(match.drawn_or_tied)
        pending.append(match)
        if progress and i % 1000 == 0:
            progress(i)
    for done in pending:
        state.update(done)
    frame = pd.DataFrame(rows)
    player_frame = pd.DataFrame(player_rows, columns=C.PLAYER_MATCH_COLS)
    counts = _source_counts(source, n_seen=n_seen)
    quality = DataQuality(
        source=type(source).__name__,
        offered_matches=counts.offered,
        out_of_scope_matches=counts.out_of_scope,
        unusable_matches=counts.unusable,
        matches_read=counts.yielded,
        matches_read_by_format=matches_read_by_format,
        matches_read_by_level=matches_read_by_level,
        undecided_matches=n_undecided,
        drawn_or_tied_matches=n_drawn_or_tied,
        decided_matches_without_deliveries=n_decided_without_deliveries,
        runs_not_charged_to_bowler=runs_not_charged_to_bowler,
        runs_scored=runs_scored,
        deliveries_not_faced=deliveries_not_faced,
        dismissals=dismissals,
        namesake_sides=namesake_sides,
        oversized_squads=oversized_squads,
        replacement_players=replacement_players,
        unknown_player_keys=_unknown_player_keys(state),
        player_keys=len(state.players),
        team_keys=len(team_keys),
        players_with_birth_date=count_known(state.birth_dates, state.players.keys),
        matches_with_stage_label=stakes_counts["stage"],
        knockout_matches=stakes_counts["knockout"],
        matches_with_reconstructible_table=stakes_counts["table"],
        dead_rubber_matches=stakes_counts["dead"],
        matches_with_one_home_side=matches_with_one_home_side,
        matches_without_toss=matches_without_toss,
        multi_day_matches=multi_day_matches,
    )
    logger.info(
        "rating pass: %d training rows, %d player-match rows, %d undecided matches, "
        "%d decided matches without deliveries, %d players "
        "(%d namesake sides, %d sides over eleven, %d unresolved player keys, %d with a date of birth), "
        "%d matches with exactly one side at home, %d without a toss",
        len(frame),
        len(player_frame),
        n_undecided,
        quality.decided_matches_without_deliveries,
        quality.player_keys,
        quality.namesake_sides,
        quality.oversized_squads,
        quality.unknown_player_keys,
        quality.players_with_birth_date,
        quality.matches_with_one_home_side,
        quality.matches_without_toss,
    )
    return BuildResult(frame=frame, player_frame=player_frame, state=state, n_undecided=n_undecided, quality=quality)


def _source_counts(source: MatchSource, n_seen: int) -> SourceCounts:
    """What the source says it did, or what the pass saw if it does not say.

    A source that reports nothing is described by what arrived rather than by zeros: zeros
    would make the gate's accounting identity hold vacuously, which is the opposite of what
    it is for. Only a source that tracks its own drops can report them.
    """
    counts = getattr(source, "counts", None)
    if isinstance(counts, SourceCounts) and counts.offered:
        return counts
    return SourceCounts(offered=n_seen, yielded=n_seen)


def _namesake_sides(match) -> int:
    """Sides of this match holding a player key the other side also holds.

    One person cannot play for both teams, so a shared key means two people the source
    cannot tell apart -- Cricsheet's registry is keyed by name within a file. The go-app
    importer drops such a name from both squads; a source that does not will hand the same
    ratings to both sides.
    """
    shared = set(match.team1_players) & set(match.team2_players)
    return 2 if shared else 0


def _oversized_squads(match) -> int:
    """Sides still of more than eleven once the replacements are taken out: a source that
    lists twelve and records no replacement for the twelfth (FEAT-02)."""
    return sum(1 for side in (match.team1_players, match.team2_players) if len(side) > 11)


def _runs_not_charged_to_bowler(match) -> int:
    """The match's byes, leg-byes and penalty runs: what its deliveries scored that no
    bowler is charged (``Deliveries.runs_bowler``). Whole runs on every source, so the
    count is exact."""
    deliveries = match.deliveries
    return int(round(float((deliveries.runs_total - deliveries.runs_bowler).sum())))


def _runs_scored(match) -> int:
    """The match's runs off the bat's end, extras included (``Deliveries.runs_total``).
    Whole runs on every source, so the count is exact."""
    return int(round(float(match.deliveries.runs_total.sum())))


def _deliveries_not_faced(match) -> int:
    """The match's wides: the deliveries no batter faced (``Deliveries.faced``). Whole
    deliveries on every source, so the count is exact."""
    deliveries = match.deliveries
    return int(len(deliveries) - int(round(float(deliveries.faced.sum()))))


def _dismissals(match) -> int:
    """The match's wickets lost (``Deliveries.wicket``): every wicket the source holds
    that the vocabulary calls a dismissal, the second one on a delivery included. Whole
    wickets on every source, so the count is exact."""
    return int(round(float(match.deliveries.wicket.sum())))


def _unknown_player_keys(state: RatingState) -> int:
    """Keys the source could not resolve to a person.

    Both sources spell such a key ``name:<name>``, so counting the prefix counts the
    fallbacks. Zero on the current dataset from either source, which is what makes it worth
    gating: the fallback is the path that quietly goes back to identifying people by name.
    """
    return sum(1 for key in state.players.key_to_slot if str(key).startswith("name:"))
