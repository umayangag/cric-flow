"""What the XI request models refuse (SERVE-02).

Every route behind these models answers a side it cannot express: an eleven's aggregates
are computed over whatever list arrives, so a nine-player side comes back with a plausible
probability for a question nobody asked, a duplicated pool id can be selected twice, and a
player on both sides is read into both aggregates. None of it shows on the response, which
is why the refusal is at the boundary rather than in the search.
"""

from __future__ import annotations

from typing import List

import pytest
from pydantic import ValidationError

from app.models.xi import (
    TEAM_SIZE,
    PerformancePredictRequest,
    SimulateRequest,
    XiConstraints,
    XiOptimizeRequest,
    XiWinRequest,
)
from tests.xi_fixtures import xi

# The three request models that name two elevens: a win prediction and the two that
# inherit from it. One rule, checked on each, so none of them can drift off it.
TWO_ELEVEN_REQUESTS = [XiWinRequest, PerformancePredictRequest, SimulateRequest]


def _sides(team1: List[str], team2: List[str]) -> dict:
    return {"format": "T20I", "team1_player_ids": team1, "team2_player_ids": team2}


@pytest.mark.parametrize("model", TWO_ELEVEN_REQUESTS)
def test_two_full_elevens_are_accepted(model) -> None:
    """The ordinary request: eleven distinct a side, nobody shared."""
    request = model(**_sides(xi("a"), xi("b")))

    assert len(request.team1_player_ids) == TEAM_SIZE
    assert len(request.team2_player_ids) == TEAM_SIZE


@pytest.mark.parametrize("model", TWO_ELEVEN_REQUESTS)
def test_a_nine_player_side_is_refused_rather_than_answered(model) -> None:
    """Nine players is a different question, and the answer would not say so."""
    with pytest.raises(ValidationError, match="holds 9 players and a side is scored as 11"):
        model(**_sides(xi("a")[:9], xi("b")))


@pytest.mark.parametrize("model", TWO_ELEVEN_REQUESTS)
def test_a_twelfth_player_is_refused(model) -> None:
    """And so is a side with one too many."""
    with pytest.raises(ValidationError, match="holds 12 players"):
        model(**_sides(xi("a") + ["a11"], xi("b")))


@pytest.mark.parametrize("model", TWO_ELEVEN_REQUESTS)
def test_a_side_naming_one_player_twice_is_refused(model) -> None:
    """One player fills one place: a repeat is a ten-man side, scored as eleven."""
    doubled = xi("a")[:10] + ["a0"]

    with pytest.raises(ValidationError, match="team1_player_ids names the same player more than once: a0"):
        model(**_sides(doubled, xi("b")))


@pytest.mark.parametrize("model", TWO_ELEVEN_REQUESTS)
def test_a_player_on_both_sides_is_refused(model) -> None:
    """Nobody plays both elevens (GO-04's boundary half)."""
    with pytest.raises(ValidationError, match="team1_player_ids and team2_player_ids name the same player: a3"):
        model(**_sides(xi("a"), xi("b")[:10] + ["a3"]))


def test_a_duplicated_pool_id_cannot_be_offered_to_the_search() -> None:
    """The optimiser's pool index keeps the last position an id occupies, so a duplicated
    candidate can be chosen twice for one eleven."""
    with pytest.raises(ValidationError, match="pool_player_ids names the same player more than once: a2"):
        XiOptimizeRequest(format="T20I", pool_player_ids=xi("a") + ["a2"], opponent_player_ids=xi("b"))


def test_a_pool_smaller_than_the_eleven_is_refused() -> None:
    with pytest.raises(ValidationError):
        XiOptimizeRequest(format="T20I", pool_player_ids=xi("a")[:10], opponent_player_ids=xi("b"))


def test_a_pool_that_holds_the_opponent_is_refused() -> None:
    """Optimising against an eleven the pool can pick from is optimising against itself."""
    with pytest.raises(ValidationError, match="pool_player_ids and opponent_player_ids name the same player: b4"):
        XiOptimizeRequest(format="T20I", pool_player_ids=xi("a") + ["b4"], opponent_player_ids=xi("b"))


def test_an_opponent_that_is_not_an_eleven_is_refused() -> None:
    with pytest.raises(ValidationError, match="opponent_player_ids holds 9 players"):
        XiOptimizeRequest(format="T20I", pool_player_ids=xi("a"), opponent_player_ids=xi("b")[:9])


def test_no_opponent_is_still_how_the_rating_ordered_path_asks() -> None:
    """objective='ratings' reads no opponent, and an empty list is not a short side."""
    request = XiOptimizeRequest(format="T20I", pool_player_ids=xi("a"), objective="ratings")

    assert request.opponent_player_ids == []


@pytest.mark.parametrize("size", [1, 9, 12, 15])
def test_no_side_size_but_eleven_is_supported(size: int) -> None:
    """The constraint used to accept 1-15 although every model behind it was fitted on
    elevens, and the optimiser would then return a side no other route could score."""
    with pytest.raises(ValidationError, match="team_size is 11"):
        XiConstraints(team_size=size)


def test_the_default_constraint_is_an_eleven() -> None:
    assert XiConstraints().team_size == TEAM_SIZE
