"""Per-ball sequence context (E1): the ``seqcalc`` families -- dot streaks, reactions and
spells -- expressed as flags on each delivery of one match, so the rating pass can
accumulate them as-of like every other rate.

A *stream* is one player's deliveries within one innings, in playing order: the batter's
balls faced, or the bowler's balls bowled. "Before" and "after" are always within a stream,
so a batter's dot streak does not survive the partner taking strike and a bowler's does
not survive the over at the other end.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, List, Tuple

import numpy as np

from ml.xi import contract as C
from ml.xi.sources import Deliveries


@dataclass
class SequenceFlags:
    """One value per delivery of the match, aligned with ``Deliveries``."""

    bat_dots_before: np.ndarray  # consecutive dots the batter faced immediately before this ball
    bowl_dots_before: np.ndarray  # consecutive dots the bowler bowled immediately before this ball
    bat_after_boundary: np.ndarray  # bool: the batter's previous ball was a 4 or 6 off the bat
    bowl_after_boundary: np.ndarray  # bool: the bowler's previous ball conceded a 4 or 6 off the bat
    bowl_after_wicket: np.ndarray  # bool: the bowler's previous ball was a bowler-credited wicket
    spell_first_over: np.ndarray  # bool: the ball is in the first over of the bowler's spell
    spells_per_bowler: Dict[str, Tuple[int, int]]  # bowler key -> (spells bowled, overs bowled)


def _stream_codes(innings: np.ndarray, players: np.ndarray) -> np.ndarray:
    """An integer code per ball identifying (innings, player)."""
    labels = np.char.add(np.char.add(innings.astype(str), "|"), players.astype(str))
    _, codes = np.unique(labels, return_inverse=True)
    return codes


def _previous_in_stream(codes: np.ndarray) -> np.ndarray:
    """Index of the previous ball in the same stream, -1 for the first."""
    order = np.argsort(codes, kind="stable")
    sorted_codes = codes[order]
    previous = np.full(len(codes), -1, dtype=int)
    same = sorted_codes[1:] == sorted_codes[:-1]
    previous[order[1:][same]] = order[:-1][same]
    return previous


def _dots_before(codes: np.ndarray, dot: np.ndarray) -> np.ndarray:
    """Consecutive dots immediately before each ball within its stream.

    In stream-sorted order the count is the distance to the last non-dot ball, capped at the
    stream start; both are running maxima, so no per-ball loop is needed.
    """
    n = len(codes)
    if n == 0:
        return np.zeros(0, dtype=int)
    order = np.argsort(codes, kind="stable")
    sorted_codes = codes[order]
    position = np.arange(n)
    starts = np.concatenate([[0], np.flatnonzero(sorted_codes[1:] != sorted_codes[:-1]) + 1])
    stream_start = starts[np.searchsorted(starts, position, side="right") - 1]
    breaks = np.where(dot[order], -1, position)
    last_break_before = np.concatenate([[-1], np.maximum.accumulate(breaks)[:-1]])
    dots = position - np.maximum(last_break_before, stream_start - 1) - 1
    out = np.zeros(n, dtype=int)
    out[order] = dots
    return out


def _spell_flags(d: Deliveries, bowler_codes: np.ndarray) -> Tuple[np.ndarray, Dict[str, Tuple[int, int]]]:
    """First-over-of-spell flag per ball, and (spells, overs) per bowler across the match.

    Successive overs by one bowler at most ``SEQUENCE_SPELL_MAX_GAP`` apart are one spell,
    which is how alternate-end bowling reads in over numbers.
    """
    first_over = np.zeros(len(d), dtype=bool)
    per_bowler: Dict[str, List[int]] = {}
    for code in np.unique(bowler_codes):
        balls = np.flatnonzero(bowler_codes == code)
        overs = np.unique(d.over[balls])
        spell_starts = {int(overs[0])}
        for previous, current in zip(overs[:-1], overs[1:]):
            if current - previous > C.SEQUENCE_SPELL_MAX_GAP:
                spell_starts.add(int(current))
        first_over[balls] = np.isin(d.over[balls], list(spell_starts))
        counts = per_bowler.setdefault(str(d.bowler[balls[0]]), [0, 0])
        counts[0] += len(spell_starts)
        counts[1] += len(overs)
    return first_over, {key: (spells, overs) for key, (spells, overs) in per_bowler.items()}


def sequence_flags(d: Deliveries) -> SequenceFlags:
    n = len(d)
    if n == 0:
        empty_int, empty_bool = np.zeros(0, dtype=int), np.zeros(0, dtype=bool)
        return SequenceFlags(empty_int, empty_int, empty_bool, empty_bool, empty_bool, empty_bool, {})
    dot = d.runs_total == 0
    boundary = (d.runs_batter == 4) | (d.runs_batter == 6)
    bat_codes = _stream_codes(d.innings, d.batter)
    bowl_codes = _stream_codes(d.innings, d.bowler)
    bat_previous = _previous_in_stream(bat_codes)
    bowl_previous = _previous_in_stream(bowl_codes)

    def after(previous: np.ndarray, event: np.ndarray) -> np.ndarray:
        return (previous >= 0) & event[np.maximum(previous, 0)]

    first_over, spells = _spell_flags(d, bowl_codes)
    return SequenceFlags(
        bat_dots_before=_dots_before(bat_codes, dot),
        bowl_dots_before=_dots_before(bowl_codes, dot),
        bat_after_boundary=after(bat_previous, boundary),
        bowl_after_boundary=after(bowl_previous, boundary),
        bowl_after_wicket=after(bowl_previous, d.bowler_wicket > 0),
        spell_first_over=first_over,
        spells_per_bowler=spells,
    )
