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
    """predict_proba as a logistic of a fixed linear score over XI_FEATURE_COLS."""

    def __init__(self, weights_by_column):
        self.weights = np.asarray([weights_by_column.get(col, 0.0) for col in C.XI_FEATURE_COLS])

    def predict_proba(self, x):
        z = 1.0 / (1.0 + np.exp(-np.asarray(x) @ self.weights))
        return np.column_stack([1.0 - z, z])


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
