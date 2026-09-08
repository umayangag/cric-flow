"""The two role predicates the objective reads, in one definition (P3-1).

A player's *role*, in this system, is not cricket's vocabulary. It is exactly two
predicates evaluated on the served as-of vectors: he keeps wicket (the state has credited
him with a stumping) and he is a bowling option (his expected balls bowled clear the
format's threshold, ``contract.MIN_BOWLING_BALLS``). "All-rounder", "finisher", "death
bowler" and "powerplay specialist" are not vocabularies this system has measured -- A-3's
phase-matchup family was a recorded null (plan §8.11) -- so nothing here invents them, and
a player who answers neither predicate is a batter *by elimination*, which is a different
statement and is labelled as one wherever it is shown.

Why this module exists. Until P3-1 the two predicates were written out three times: inside
``optimizer._Pool`` (where the search enforces them), inside
``app.xi_service._constraint_check`` (where an eleven is measured against them without
being changed), and implicitly in ``optimizer.selection_reasons``, which is what the "why
this player" card renders. P3-1 adds a fourth reader -- the auction module asks for the
roles of an arbitrary list of players, none of them in a selected eleven -- and four
copies of a predicate is four chances for the surface that values a player to disagree
with the surface that would pick him. They are one definition here, and the tests pin the
new reader's answer equal to ``selection_reasons``' on the same ids.

Nothing in this module selects, ranks or scores. It reads vectors and answers yes or no.
"""

from __future__ import annotations

from typing import List, Mapping, Sequence, Tuple

import numpy as np

from ml.xi import contract as C

# The constraint state a surface may name for a player (P1-3, P3-1): the requirements the
# objective evaluates, each read off the same as-of vectors the objective reads.
#
# Wire vocabulary, declared once in contracts/ops-console.contract.json and asserted from
# every side (H-24).
ROLE_KEEPER = "keeper"
ROLE_BOWLING_OPTION = "bowling_option"
SELECTION_ROLES: Tuple[str, ...] = (ROLE_KEEPER, ROLE_BOWLING_OPTION)


def is_keeper(keeper_vector_value) -> bool:
    """Whether the served state has credited this player with a stumping.

    That is all the ``keeper`` vector holds (``contract.PLAYER_VECTOR_KEYS``), and so it
    is all the predicate can mean: a keeper is a player who has kept, not a player some
    name set was marked against. The database's ``player.is_wicket_keeper`` is that other
    thing and is not this.
    """
    return bool(keeper_vector_value > 0)


def is_bowling_option(expected_balls_bowled, format_code: str) -> bool:
    """Whether this player's expected balls bowled make him a bowling option.

    One player, one answer. ``contract.is_bowling_option`` is the definition -- it also
    works elementwise on arrays, which is what the feature columns need -- and this is the
    scalar reading of it that every constraint and every surface shares.
    """
    return bool(C.is_bowling_option(expected_balls_bowled, format_code))


def roles_of(vectors: Mapping[str, np.ndarray], index: int, format_code: str) -> List[str]:
    """The roles one player's as-of vectors support, in ``SELECTION_ROLES`` order.

    An empty list means the vectors support neither predicate -- a batter by elimination.
    It never means "unknown": a player the served state has not seen has no vectors to
    read, and his caller reports him unknown rather than passing zeros through here.
    """
    roles: List[str] = []
    if is_keeper(vectors["keeper"][index]):
        roles.append(ROLE_KEEPER)
    if is_bowling_option(vectors["exp_balls_bowled"][index], format_code):
        roles.append(ROLE_BOWLING_OPTION)
    return roles


def count_bowling_options(expected_balls_bowled: Sequence[float], format_code: str) -> int:
    """How many of these players are bowling options -- the count a constraint is met by."""
    return int(sum(1 for balls in expected_balls_bowled if is_bowling_option(balls, format_code)))


def any_keeper(keeper_vector: Sequence[float]) -> bool:
    """Whether any of these players keeps wicket."""
    return any(is_keeper(value) for value in keeper_vector)
