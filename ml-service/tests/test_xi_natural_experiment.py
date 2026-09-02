"""Unit tests for E5, lineup-only (ml.xi.natural_experiment)."""

from __future__ import annotations

from typing import Dict, List

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import natural_experiment as ne
from ml.xi.builder import build
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history


def _side(value: float) -> Dict[str, float]:
    return {stem: value for stem in C.SIDE_FEATURE_STEMS}


def _win_row(match_id: str, day: int, team1: str, team2: str, team1_wins: float) -> Dict:
    row = {
        "match_id": match_id,
        "match_date": pd.Timestamp("2024-01-01") + pd.Timedelta(days=day),
        "format_code": "T20",
        "team1": team1,
        "team2": team2,
        C.TARGET_COL: team1_wins,
    }
    for stem in C.SIDE_FEATURE_STEMS:
        row[f"t1_{stem}"] = 1.0
        row[f"t2_{stem}"] = 2.0
    return row


def _player_rows(match_id: str, side: int, keys: List[str]) -> List[Dict]:
    return [{"match_id": match_id, "side": side, "player_key": key} for key in keys]


def _frames(changes: int) -> tuple:
    """Side A plays B twice; the second time with ``changes`` players swapped."""
    a_first = [f"a{i}" for i in range(11)]
    a_second = [f"a{i}" for i in range(11 - changes)] + [f"x{i}" for i in range(changes)]
    b = [f"b{i}" for i in range(11)]
    frame = pd.DataFrame([_win_row("m1", 0, "A", "B", 1.0), _win_row("m2", 7, "B", "A", 1.0)])
    players = pd.DataFrame(
        _player_rows("m1", 1, a_first)
        + _player_rows("m1", 2, b)
        + _player_rows("m2", 1, b)
        + _player_rows("m2", 2, a_second)
    )
    return frame, players


def test_build_pairs_finds_one_side_changing_two_players() -> None:
    frame, players = _frames(changes=2)

    pairs = ne.build_pairs(frame, players)

    assert len(pairs) == 1
    pair = pairs[0]
    assert (pair.team, pair.changes, pair.before_match_id, pair.after_match_id) == ("A", 2, "m1", "m2")
    # A won m1 as team1 and lost m2 as team2: the result moved down.
    assert pair.d_result == -1
    # A batted second in m2, so its own aggregates are the t2 stems and the opponent's the t1.
    assert pair.after_side["pelo_mean"] == 2.0 and pair.opponent_side["pelo_mean"] == 1.0


@pytest.mark.parametrize("changes", [0, 4])
def test_build_pairs_skips_changes_outside_the_range(changes: int) -> None:
    frame, players = _frames(changes=changes)

    assert ne.build_pairs(frame, players) == []


def test_build_pairs_skips_a_side_that_is_not_an_eleven() -> None:
    frame, players = _frames(changes=1)
    players = pd.concat([players, pd.DataFrame(_player_rows("m2", 2, ["twelfth"]))])

    assert ne.build_pairs(frame, players) == []


def test_agreement_scores_only_moved_results_with_a_preference() -> None:
    d_objective = np.array([0.1, -0.1, 0.0, 0.2, -0.3])
    d_result = np.array([1, -1, 1, 0, 1])

    out = ne.agreement(d_objective, d_result)

    assert out["pairs_scored"] == 3 and out["agreed"] == 2
    assert out["agreement"] == pytest.approx(2 / 3)
    assert out["excluded_result_unchanged"] == 1
    assert out["excluded_objective_indifferent"] == 1
    assert out["ci95"][0] < out["agreement"] < out["ci95"][1]


def test_agreement_is_none_when_nothing_can_be_scored() -> None:
    out = ne.agreement(np.array([0.1]), np.array([0]))

    assert out["agreement"] is None and out["standard_error"] is None and out["ci95"] is None


def test_derived_bar_matches_the_closed_form_and_sits_below_it() -> None:
    """An exactly-right objective with a 2 pp lineup effect agrees with the result only a
    little over half the time -- the bar is a function of the claimed effect, and 0.55 was
    never reachable from a median |Δ| of 0.02."""
    rng = np.random.default_rng(1)
    n = 4000
    p_before = rng.uniform(0.3, 0.7, n)
    p_previous = rng.uniform(0.3, 0.7, n)
    d = rng.choice([-0.02, 0.02], n)
    scored = ne.ScoredPairs(p_before, p_previous, p_previous + d, np.zeros(n, dtype=int), np.ones(n, dtype=int))

    out = ne.derived_bar(scored, replicates=300, seed=0)

    assert out["n_pairs"] == n
    assert 0.50 < out["expected_if_exactly_right"] < 0.55
    assert out["simulated_mean"] == pytest.approx(out["expected_if_exactly_right"], abs=0.01)
    assert out["bar"] < out["expected_if_exactly_right"]
    assert out["bar"] > 0.45


def test_derived_bar_is_empty_without_a_preference() -> None:
    scored = ne.ScoredPairs(np.array([0.5]), np.array([0.5]), np.array([0.5]), np.array([1]), np.array([1]))

    assert ne.derived_bar(scored)["bar"] is None


def test_score_pairs_reads_the_lineup_delta_from_the_previous_eleven() -> None:
    frame, players = _frames(changes=1)
    pair = ne.build_pairs(frame, players)[0]
    pair.before_side = _side(1.5)

    scored = ne.score_pairs([pair], lambda x: np.full(len(x), 0.6), C.XI_FEATURE_COLS)

    assert len(scored) == 1
    # A constant model claims the same probability everywhere: no lineup preference.
    assert scored.d_lineup[0] == pytest.approx(0.0)


def test_score_pairs_skips_pairs_without_a_previous_eleven() -> None:
    frame, players = _frames(changes=1)
    pair = ne.build_pairs(frame, players)[0]

    assert len(ne.score_pairs([pair], lambda x: np.full(len(x), 0.6), C.XI_FEATURE_COLS)) == 0


def test_decision_reason_names_the_verdict_and_the_policy_together() -> None:
    decision = {
        "agreement": 0.51,
        "pairs_scored": 100,
        "bar": 0.505,
        "expected_if_exactly_right": 0.52,
        "passes_derived_bar": True,
    }

    reason = ne.decision_reason("T20", decision, served=False)

    assert reason.startswith("optimised selection not served in T20, because E5 lineup-only agreement 0.510")
    assert "against the derived bar 0.505" in reason and reason.endswith("passes")


def test_decision_reason_says_when_nothing_could_be_scored() -> None:
    reason = ne.decision_reason("ODI", {"agreement": None, "bar": None}, served=True)

    assert reason == "optimised selection served in ODI, because E5 could not be scored (no pairs whose result moved)"


@pytest.fixture(scope="module")
def synthetic_pairs() -> List[ne.LineupPair]:
    matches, _, _ = _synthetic_history(80)
    result = build(_ListSource(matches))
    pairs = ne.build_pairs(result.frame, result.player_frame)
    ne.score_previous_elevens(pairs, _ListSource(matches))
    return pairs


def test_score_previous_elevens_agrees_with_the_training_pass_on_the_fielded_eleven(synthetic_pairs) -> None:
    matches, _, _ = _synthetic_history(80)
    pairs = ne.build_pairs(*(lambda r: (r.frame, r.player_frame))(build(_ListSource(matches))))

    out = ne.score_previous_elevens(pairs, _ListSource(matches))

    assert out["pairs_scored"] == len(pairs) > 0
    assert out["fielded_eleven_max_abs_difference"] == pytest.approx(0.0, abs=1e-9)
    assert all(p.before_side is not None for p in pairs)


def test_evaluate_format_reports_folds_pooled_decision_and_locked_window(synthetic_pairs) -> None:
    proba = lambda x: 1.0 / (1.0 + np.exp(-x[:, 0]))  # noqa: E731 - a monotone stand-in objective
    dates = sorted(p.after_date for p in synthetic_pairs)
    cutoff, mid, locked_start = dates[0], dates[len(dates) // 3], dates[2 * len(dates) // 3]

    out = ne.evaluate_format(
        "T20",
        synthetic_pairs,
        [(cutoff, mid, proba), (mid, locked_start, proba)],
        (locked_start, proba),
        C.XI_FEATURE_COLS,
        served=True,
    )

    assert out["definition"] == ne.DEFINITION and out["why_not_as_played"] == ne.WHY_NOT_AS_PLAYED
    assert out["pairs"]["total"] == len(synthetic_pairs)
    assert out["pairs"]["development"] + out["pairs"]["locked"] == len(synthetic_pairs)
    assert len(out["walk_forward"]["folds"]) == 2 and out["walk_forward"]["summary"]["agreement"]["n_folds"] == 2
    assert out["development"]["pairs_scored"] > 0
    assert out["development"]["derived_bar"]["bar"] is not None
    assert out["decision"]["passes_derived_bar"] in (True, False)
    assert out["decision"]["optimised_selection_served"] is True
    assert "locked window" in out["locked"]["note"]
    assert out["locked"]["pairs_scored"] > 0


def test_evaluate_format_labels_a_fold_without_an_objective(synthetic_pairs) -> None:
    dates = sorted(p.after_date for p in synthetic_pairs)

    out = ne.evaluate_format(
        "T20", synthetic_pairs, [(dates[0], dates[-1], None)], (dates[-1], None), C.XI_FEATURE_COLS, False
    )

    assert out["walk_forward"]["folds"][0]["skipped_reason"] == "no objective for this fold"
    assert out["decision"]["agreement"] is None
    assert out["locked"]["skipped_reason"] == "no objective or no pairs"
