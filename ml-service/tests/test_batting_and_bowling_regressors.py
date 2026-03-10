"""Tests for ml.batting_regressor and ml.bowling_regressor.

These modules load heavy joblib artifacts at import time; in tests we
mock joblib.load so that we can exercise the calculation helpers and
predict_* functions without touching real model files.
"""

import importlib
import os

import joblib
import pandas as pd

from ml.dataset_definitions import output_batting_columns, output_bowling_columns


def test_calculate_strike_rate_handles_zero_balls(monkeypatch):
    """calculate_strike_rate should return 0 when balls_faced is 0."""

    def fake_load(path):
        class Dummy:
            def transform(self, data):
                return data

            def predict(self, data):
                return [[0.0] * len(output_batting_columns) for _ in range(len(data))]

            def inverse_transform(self, data):
                return data

        return Dummy()

    monkeypatch.setattr(joblib, "load", fake_load)

    # Import after patching so module-level loads use the fake loader.
    import ml.batting_regressor as batting_regressor

    importlib.reload(batting_regressor)

    assert batting_regressor.calculate_strike_rate({"runs_scored": 0, "balls_faced": 0}) == 0


def test_predict_batting_populates_outputs_and_strike_rate(monkeypatch):
    """predict_batting should fill output columns and compute strike_rate."""

    def fake_load(path):
        class DummyScaler:
            def transform(self, data):
                return data

            def inverse_transform(self, data):
                return data

        class DummyPredictor:
            def predict(self, data):
                # One row of constant outputs per input row.
                return [[1.0] * len(output_batting_columns) for _ in range(len(data))]

        if "model" in str(path):
            return DummyPredictor()
        return DummyScaler()

    # Force the optional output_scaler branch to be exercised.
    monkeypatch.setattr(os.path, "isfile", lambda _path: True)
    monkeypatch.setattr(joblib, "load", fake_load)

    import ml.batting_regressor as batting_regressor

    importlib.reload(batting_regressor)

    df = pd.DataFrame(
        [
            {
                "runs_scored": 30,
                "balls_faced": 15,
            }
        ]
    )

    result = batting_regressor.predict_batting(df.copy())

    for column in output_batting_columns:
        assert column in result.columns

    row = result.loc[0]
    assert row["strike_rate"] == row["runs_scored"] * 100 / row["balls_faced"]


def test_calculate_econ_handles_zero_deliveries(monkeypatch):
    """calculate_econ should return 0 when deliveries is 0."""

    def fake_load(path):
        class Dummy:
            def transform(self, data):
                return data

            def predict(self, data):
                return [[0.0] * len(output_bowling_columns) for _ in range(len(data))]

            def inverse_transform(self, data):
                return data

        return Dummy()

    monkeypatch.setattr(joblib, "load", fake_load)

    import ml.bowling_regressor as bowling_regressor

    importlib.reload(bowling_regressor)

    assert bowling_regressor.calculate_econ({"runs_conceded": 0, "deliveries": 0}) == 0


def test_predict_bowling_populates_outputs_and_econ(monkeypatch):
    """predict_bowling should fill output columns and compute econ."""

    def fake_load(path):
        class DummyScaler:
            def transform(self, data):
                return data

            def inverse_transform(self, data):
                return data

        class DummyPredictor:
            def predict(self, data):
                return [[2.0] * len(output_bowling_columns) for _ in range(len(data))]

        if "model" in str(path):
            return DummyPredictor()
        return DummyScaler()

    # Force the optional output_scaler branch to be exercised.
    monkeypatch.setattr(os.path, "isfile", lambda _path: True)
    monkeypatch.setattr(joblib, "load", fake_load)

    import ml.bowling_regressor as bowling_regressor

    importlib.reload(bowling_regressor)

    df = pd.DataFrame(
        [
            {
                "runs_conceded": 30,
                "deliveries": 30,
            }
        ]
    )

    result = bowling_regressor.predict_bowling(df.copy())

    for column in output_bowling_columns:
        assert column in result.columns

    assert result.loc[0, "econ"] == 6.0
