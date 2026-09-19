"""``MIN_BOWLING_BALLS`` against the population it was derived from (FEAT-15).

``tests/fixtures/bowling_option_population.json`` holds, per format, a sample of the
decided sides since 2024 as the rating pass reads them -- each side's eleven
``exp_balls_bowled`` in the per-appearance unit FEAT-01 introduced -- beside the whole
population's share of sides under five bowling options and its mean count under the
pre-FEAT-01 unit. The contract's thresholds are the old bar translated into the new unit,
so the sides must read about as they did before the unit changed: real elevens carry five
bowlers, and a definition under which a fifth of them do not is measuring the threshold,
not the eleven. The old numbers read in the new unit (12 / 12 / 30 / 60) fail this in
every format the optimiser serves.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Dict, List

import numpy as np
import pytest

from ml.xi import contract as C

FIXTURE = Path(__file__).resolve().parent / "fixtures" / "bowling_option_population.json"
#: How far the sample's share of sides under five may sit above the population's
#: pre-FEAT-01 share: three sampling standard errors at 150 sides, well inside the
#: 15-21 points the old numbers miss by.
SHARE_UNDER_FIVE_TOLERANCE = 0.05
#: How far the sample's mean count of options may sit from the pre-FEAT-01 mean, either
#: way, so that neither the old numbers (most of a bowler short) nor a bar low enough to
#: admit everyone can pass.
MEAN_OPTIONS_TOLERANCE = 0.5


@pytest.fixture(scope="module")
def population() -> Dict[str, dict]:
    with open(FIXTURE) as fh:
        return json.load(fh)["formats"]


def _options_per_side(sides: List[List[float]], format_code: str) -> np.ndarray:
    return np.array([int(C.is_bowling_option(np.asarray(side), format_code).sum()) for side in sides])


@pytest.mark.parametrize("format_code", C.FORMAT_CODES)
def test_the_fixture_holds_elevens_in_the_per_appearance_unit(population: Dict[str, dict], format_code: str) -> None:
    """Every sampled side is an eleven, sorted descending, with no negative involvement."""
    sides = population[format_code]["sides"]

    assert len(sides) == 150
    assert all(len(side) == 11 and side == sorted(side, reverse=True) and side[-1] >= 0.0 for side in sides)


@pytest.mark.parametrize("format_code", C.FORMAT_CODES)
def test_the_share_of_sides_under_five_options_is_back_at_the_pre_feat01_level(
    population: Dict[str, dict], format_code: str
) -> None:
    """Fails at 12 / 12 / 30 / 60 in T20, T20I and ODI (25 %, 19 % and 38 % of sides under
    five against 4 %, 5 % and 17 % before FEAT-01); TEST's shift was within the tolerance."""
    entry = population[format_code]

    share_under_five = float((_options_per_side(entry["sides"], format_code) < 5).mean())

    assert share_under_five <= entry["share_under_five_before_feat01"] + SHARE_UNDER_FIVE_TOLERANCE


@pytest.mark.parametrize("format_code", C.FORMAT_CODES)
def test_the_mean_count_of_options_per_side_is_back_at_the_pre_feat01_level(
    population: Dict[str, dict], format_code: str
) -> None:
    """The translated bar admits about as many players per side as the old one did -- not
    most of a bowler fewer, as the old numbers in the new unit do, and not everyone."""
    entry = population[format_code]

    mean_options = float(_options_per_side(entry["sides"], format_code).mean())

    assert mean_options == pytest.approx(entry["mean_before_feat01"], abs=MEAN_OPTIONS_TOLERANCE)
