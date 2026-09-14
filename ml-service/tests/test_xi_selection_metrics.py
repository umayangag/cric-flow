"""Unit tests for the selection-consumer metrics (ml.xi.selection_metrics)."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import selection_metrics as sm
from ml.xi.builder import build
from tests.test_xi_optimizer_and_store import _ListSource, _synthetic_history


class _LinearModel:
    """predict_proba as a logistic of a fixed linear score over ``columns``."""

    def __init__(self, weights_by_column, columns=None):
        self.weights = np.asarray([weights_by_column.get(col, 0.0) for col in (columns or C.XI_FEATURE_COLS)])

    def predict_proba(self, x):
        z = 1.0 / (1.0 + np.exp(-np.asarray(x) @ self.weights))
        return np.column_stack([1.0 - z, z])


def _display_model(weights_by_column) -> _LinearModel:
    return _LinearModel(weights_by_column, C.DISPLAY_FEATURE_COLS)


@pytest.fixture(scope="module")
def synthetic_build():
    matches, _, _ = _synthetic_history(60)
    return build(_ListSource(matches))


def test_swap_monotonicity_is_clean_for_a_monotone_objective(synthetic_build) -> None:
    model = _LinearModel({"d_pelo_mean": 0.01, "d_imp_bat_sum": 0.5})

    report = sm.swap_monotonicity(model, C.XI_FEATURE_COLS, synthetic_build.player_frame, "T20", max_matches=10)

    assert report["upgrades"] > 0
    assert report["violations"] == 0
    assert report["violation_share"] == 0.0


def test_swap_monotonicity_catches_an_anti_monotone_objective(synthetic_build) -> None:
    model = _LinearModel({"d_pelo_mean": -0.01, "d_imp_bat_sum": -0.5})

    report = sm.swap_monotonicity(model, C.XI_FEATURE_COLS, synthetic_build.player_frame, "T20", max_matches=10)

    assert report["violation_share"] == 1.0


def test_swap_monotonicity_is_none_without_rows() -> None:
    empty = pd.DataFrame(columns=C.PLAYER_MATCH_COLS)

    assert sm.swap_monotonicity(_LinearModel({}), C.XI_FEATURE_COLS, empty, "T20") is None


def test_swap_monotonicity_reports_the_probe_per_axis(synthetic_build) -> None:
    """FEAT-14: a wrong sign on the Elo axis must show on its own, not only when the rate
    terms fail to cover it. A surface that likes batting impact and dislikes Elo violates
    on every Elo-only upgrade and on no rates-only one."""
    model = _LinearModel({"d_pelo_mean": -0.01, "d_imp_bat_sum": 0.5})

    report = sm.swap_monotonicity(model, C.XI_FEATURE_COLS, synthetic_build.player_frame, "T20", max_matches=10)

    assert set(report["by_axis"]) == set(sm.UPGRADE_AXES)
    assert report["by_axis"]["pelo_only"]["upgrades"] == report["upgrades"]
    assert report["by_axis"]["pelo_only"]["violation_share"] == 1.0
    assert report["by_axis"]["rates_only"]["violation_share"] == 0.0


def test_display_swap_monotonicity_is_clean_for_a_monotone_display_surface(synthetic_build) -> None:
    """B-7: a display surface that likes a better batting eleven never contradicts itself."""
    model = _display_model({"d_imp_bat_sum": 0.5, "team_elo_diff": 0.01})

    report = sm.display_swap_monotonicity(
        model, C.DISPLAY_FEATURE_COLS, synthetic_build.frame, synthetic_build.player_frame, "T20", max_matches=10
    )

    assert report["upgrades"] > 0
    assert report["violations"] == 0
    assert report["violation_share"] == 0.0


def test_display_swap_monotonicity_catches_a_surface_that_falls_on_an_upgrade(synthetic_build) -> None:
    """The number a person watches moving the wrong way is exactly what this counts.

    The player Elo weight is what makes every upgrade move the surface: a player with no
    expected balls faced changes no batting impact at all, and an unmoved probability is
    not a violation.
    """
    model = _display_model({"d_imp_bat_sum": -0.5, "d_pelo_mean": -0.01})

    report = sm.display_swap_monotonicity(
        model, C.DISPLAY_FEATURE_COLS, synthetic_build.frame, synthetic_build.player_frame, "T20", max_matches=10
    )

    assert report["violation_share"] == 1.0


def test_display_swap_monotonicity_holds_the_team_context_fixed(synthetic_build) -> None:
    """A selector cannot change team Elo, so the probe must not either.

    A surface reading nothing but team context therefore cannot move under an upgrade,
    whatever sign its weight has -- which is why constraining those columns is not what
    moves this number (B-7).
    """
    model = _display_model({"team_elo_diff": -5.0, "team_form_diff": -5.0, "venue_fam_diff": -5.0})

    report = sm.display_swap_monotonicity(
        model, C.DISPLAY_FEATURE_COLS, synthetic_build.frame, synthetic_build.player_frame, "T20", max_matches=10
    )

    assert report["upgrades"] > 0
    assert report["violations"] == 0


def test_display_swap_monotonicity_is_none_without_matches(synthetic_build) -> None:
    empty = synthetic_build.frame.head(0)

    report = sm.display_swap_monotonicity(
        _display_model({}), C.DISPLAY_FEATURE_COLS, empty, synthetic_build.player_frame, "T20"
    )

    assert report is None


def test_display_swap_monotonicity_skips_a_match_with_no_player_rows(synthetic_build) -> None:
    """A window's win rows and its player rows are separate frames, and a match present in
    one but not the other is skipped rather than scored against a half-built eleven."""
    frame = synthetic_build.frame
    kept = set(frame.match_id.tolist()[1:])
    players = synthetic_build.player_frame
    model = _display_model({"d_imp_bat_sum": 0.5})

    report = sm.display_swap_monotonicity(
        model, C.DISPLAY_FEATURE_COLS, frame, players[players.match_id.isin(kept)], "T20", max_matches=10
    )

    assert report["upgrades"] > 0


def test_specific_vs_typical_reports_the_delta(synthetic_build) -> None:
    model = _LinearModel({"d_pelo_mean": 0.01, "d_imp_bat_sum": 0.5})
    frame = synthetic_build.frame

    report = sm.specific_vs_typical(
        model, C.XI_FEATURE_COLS, frame, pd.Timestamp("2023-02-01"), pd.Timestamp("2023-04-01")
    )

    assert report["n"] >= 20
    assert report["delta"] == pytest.approx(report["auc_specific_xi"] - report["auc_typical_xi"])


def test_specific_vs_typical_is_none_without_enough_history(synthetic_build) -> None:
    frame = synthetic_build.frame.head(10)

    report = sm.specific_vs_typical(
        _LinearModel({}), C.XI_FEATURE_COLS, frame, pd.Timestamp("2023-01-01"), pd.Timestamp("2023-01-05")
    )

    assert report is None


def _canary_frame() -> pd.DataFrame:
    rng = np.random.RandomState(0)
    rows = []
    for fmt, leaky in (("T20", True), ("TEST", False)):
        for i in range(80):
            y = float(i % 2)
            row = {col: 0.0 for col in C.DISPLAY_FEATURE_COLS}
            row.update(
                {
                    "match_date": pd.Timestamp("2024-06-01"),
                    "format_code": fmt,
                    C.TARGET_COL: y,
                    # The leak: reads the answer in T20, pure noise in TEST -- the S-3c shape.
                    "d_pelo_mean": (y * 2 - 1) if leaky else float(rng.randn()),
                }
            )
            rows.append(row)
    return pd.DataFrame(rows)


def test_leak_canary_flags_a_column_that_collapses_in_test_format() -> None:
    frame = _canary_frame()

    report = sm.leak_canary(frame, pd.Timestamp("2024-01-01"), pd.Timestamp("2025-01-01"))

    assert report["best_single_column"]["T20"]["column"] == "d_pelo_mean"
    suspects = report["test_control_suspects"]
    assert any(s["column"] == "d_pelo_mean" and s["format"] == "T20" for s in suspects)
