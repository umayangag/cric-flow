"""Run the rating pass over a source: training frame out, serving state out."""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Callable, List, Optional

import pandas as pd

from ml.xi import contract as C
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


META_COLS: List[str] = ["match_id", "match_date", "format_code", "gender", "team1", "team2", "venue", C.TARGET_COL]


def build(
    source: MatchSource,
    progress: Optional[Callable[[int], None]] = None,
    gender_split_context: bool = False,
) -> BuildResult:
    """Run the pass. Matches are folded into the state at *day close*: every match on a date
    reads features from prior dates only, then the whole day is applied. Within a date the
    source's order is by id, not by start time, so sequential updates would let a match see
    the result of a same-day match it may in fact have preceded. 78% of matches share a date
    with another in the same format; the cost of the strict rule is <= 0.003 AUC."""
    state = RatingState(gender_split_context=gender_split_context)
    rows = []
    player_rows = []
    n_undecided = 0
    n_seen = 0
    team_keys = set()
    namesake_sides = 0
    oversized_squads = 0
    pending: List = []
    current_date = None
    for i, match in enumerate(source.iter_matches()):
        n_seen += 1
        team_keys.update((match.team1, match.team2))
        namesake_sides += _namesake_sides(match)
        oversized_squads += _oversized_squads(match)
        if current_date is not None and match.match_date < current_date:
            raise ValueError(f"source is not in date order: {match.match_id} ({match.match_date}) after {current_date}")
        if current_date is not None and match.match_date != current_date:
            for done in pending:
                state.update(done)
            pending.clear()
        current_date = match.match_date
        if match.outcome is not None:
            win_row, match_player_rows = build_match_rows(state, match)
            rows.append(win_row)
            player_rows.extend(match_player_rows)
        else:
            n_undecided += 1
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
        undecided_matches=n_undecided,
        namesake_sides=namesake_sides,
        oversized_squads=oversized_squads,
        unknown_player_keys=_unknown_player_keys(state),
        player_keys=len(state.players),
        team_keys=len(team_keys),
    )
    logger.info(
        "rating pass: %d training rows, %d player-match rows, %d undecided matches, %d players "
        "(%d namesake sides, %d sides over eleven, %d unresolved player keys)",
        len(frame),
        len(player_frame),
        n_undecided,
        quality.player_keys,
        quality.namesake_sides,
        quality.oversized_squads,
        quality.unknown_player_keys,
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
    """Sides of more than eleven: concussion and injury replacements, listed in full."""
    return sum(1 for side in (match.team1_players, match.team2_players) if len(side) > 11)


def _unknown_player_keys(state: RatingState) -> int:
    """Keys the source could not resolve to a person.

    Both sources spell such a key ``name:<name>``, so counting the prefix counts the
    fallbacks. Zero on the current dataset from either source, which is what makes it worth
    gating: the fallback is the path that quietly goes back to identifying people by name.
    """
    return sum(1 for key in state.players.key_to_slot if str(key).startswith("name:"))
