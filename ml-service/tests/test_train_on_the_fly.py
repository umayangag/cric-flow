"""Unit tests for app.train_on_the_fly: row parsing, in-memory training, fetch and full pipeline."""

import json
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from app.train_on_the_fly import (
    BATTING_FEATURE_COLS,
    BATTING_TARGET_COLS,
    BOWLING_FEATURE_COLS,
    BOWLING_TARGET_COLS,
    _batting_rows_to_xy,
    _bowling_rows_to_xy,
    _train_batting_in_memory,
    _train_bowling_in_memory,
    fetch_training_data,
    train_on_the_fly,
)


# --- Minimal valid batting: headers + one row (targets + feature cols) ---
def _batting_headers():
    return list(BATTING_TARGET_COLS) + list(BATTING_FEATURE_COLS) + ["player_name"]


def _one_batting_row():
    """One row: 5 targets + 26 features + player_name. Order matches _batting_headers()."""
    targets = ["10", "12", "1", "0", "3"]  # runs, balls, fours, sixes, batting_position
    feats = [
        "0.5",
        "20.0",
        "20.0",
        "20.0",
        "0",  # batting_consistency, batting_form, form_short, form_long, momentum
        "25",
        "5",
        "0",
        "50",
        "2",
        "0",
        "0",  # temp, wind, rain, humidity, cloud, pressure, viscosity (7)
        "1",
        "1",
        "1",  # inning, batting_session, toss
        "0.5",
        "0.5",
        "2024",  # batting_venue, batting_opposition, season_id
        "0",
        "0",
        "0",
        "0",
        "0",
        "0",
        "0",
        "0",  # seq cols (8)
    ]
    assert len(feats) == len(BATTING_FEATURE_COLS)
    return targets + feats + ["Player One"]


# --- Minimal valid bowling: headers + one row ---
def _bowling_headers():
    return list(BOWLING_TARGET_COLS) + list(BOWLING_FEATURE_COLS) + ["player_name"]


def _one_bowling_row():
    """One row: 3 targets + 25 features + player_name. Order matches _bowling_headers()."""
    targets = ["24", "24", "2"]  # runs, balls, wickets
    feats = [
        "0.4",
        "1.5",
        "1.5",
        "1.5",
        "0",  # bowling_consistency, bowling_form, form_short, form_long, momentum
        "25",
        "5",
        "0",
        "50",
        "2",
        "0",
        "0",  # temp..viscosity (7)
        "1",
        "0",
        "1",  # inning, bowling_session, toss
        "0.5",
        "0.5",
        "2024",  # bowling_venue, bowling_opposition, season_id
        "0",
        "0",
        "0",
        "0",
        "0",
        "0",
        "0",
        "0",  # seq cols (8)
    ]
    assert len(feats) == len(BOWLING_FEATURE_COLS)
    return targets + feats + ["Bowler One"]


# --- _batting_rows_to_xy ---
def test_batting_rows_to_xy_empty():
    X, Y = _batting_rows_to_xy([], [])
    assert X.shape == (0, len(BATTING_FEATURE_COLS))
    assert Y.shape == (0, 5)  # runs, balls, fours, sixes, batting_position (no strike_rate)


def test_batting_rows_to_xy_empty_headers_no_rows():
    X, Y = _batting_rows_to_xy(_batting_headers(), [])
    assert X.shape == (0, len(BATTING_FEATURE_COLS))
    assert Y.shape == (0, 5)


def test_batting_rows_to_xy_one_valid_row():
    headers = _batting_headers()
    rows = [_one_batting_row()]
    X, Y = _batting_rows_to_xy(headers, rows)
    assert X.shape == (1, len(BATTING_FEATURE_COLS))
    assert Y.shape == (1, 5)  # strike_rate is NOT a target (derived post-prediction)
    assert Y[0, 0] == 10.0  # runs


def test_batting_rows_to_xy_toss_string_normalized():
    """Toss column as string 'bat' / 'field' is normalized to 1/0."""
    headers = _batting_headers()
    row = _one_batting_row()
    idx_toss = headers.index("toss")
    row[idx_toss] = "bat"
    rows = [row]
    X, Y = _batting_rows_to_xy(headers, rows)
    assert X.shape[0] == 1
    col_toss = BATTING_FEATURE_COLS.index("toss")
    assert X[0, col_toss] == 1.0  # toss should be 1 for "bat"
    row2 = _one_batting_row()
    row2[idx_toss] = "field"
    X2, _ = _batting_rows_to_xy(headers, [row2])
    assert X2[0, col_toss] == 0.0


def test_batting_rows_to_xy_multiple_rows():
    headers = _batting_headers()
    rows = [_one_batting_row(), _one_batting_row()]
    rows[1][0] = "25"  # different runs
    rows[1][1] = "20"  # different balls
    X, Y = _batting_rows_to_xy(headers, rows)
    assert X.shape == (2, len(BATTING_FEATURE_COLS))
    assert Y.shape == (2, 5)
    assert Y[1, 0] == 25.0


# --- _bowling_rows_to_xy ---
def test_bowling_rows_to_xy_empty():
    X, Y = _bowling_rows_to_xy([], [])
    assert X.shape == (0, len(BOWLING_FEATURE_COLS))
    assert Y.shape == (0, 3)  # runs, balls, wickets (economy derived post-prediction)


def test_bowling_rows_to_xy_one_valid_row():
    headers = _bowling_headers()
    rows = [_one_bowling_row()]
    X, Y = _bowling_rows_to_xy(headers, rows)
    assert X.shape == (1, len(BOWLING_FEATURE_COLS))
    assert Y.shape == (1, 3)  # economy is NOT a target (derived post-prediction)
    assert Y[0, 0] == 24.0
    assert Y[0, 2] == 2.0


def test_bowling_rows_to_xy_toss_and_session_normalized():
    headers = _bowling_headers()
    row = _one_bowling_row()
    idx_toss = headers.index("toss")
    row[idx_toss] = "bat"
    idx_sess = headers.index("bowling_session")
    row[idx_sess] = ""  # null/empty
    X, Y = _bowling_rows_to_xy(headers, [row])
    assert X.shape[0] == 1
    col_toss = BOWLING_FEATURE_COLS.index("toss")
    col_sess = BOWLING_FEATURE_COLS.index("bowling_session")
    assert X[0, col_toss] == 1.0  # toss
    assert X[0, col_sess] == 0.0  # bowling_session filled with 0


# --- _train_batting_in_memory / _train_bowling_in_memory ---
def test_train_batting_in_memory_returns_scaler_and_model():
    X = np.random.RandomState(42).rand(20, len(BATTING_FEATURE_COLS))
    Y = np.random.RandomState(43).rand(20, 5)  # runs, balls, fours, sixes, batting_position
    scaler, model = _train_batting_in_memory(X, Y)
    assert scaler is not None
    assert model is not None
    Xs = scaler.transform(X[:2])
    pred = model.predict(Xs)
    assert pred.shape == (2, 5)


def test_train_bowling_in_memory_returns_scaler_and_model():
    X = np.random.RandomState(44).rand(20, len(BOWLING_FEATURE_COLS))
    Y = np.random.RandomState(45).rand(20, 3)  # runs, balls, wickets
    scaler, model = _train_bowling_in_memory(X, Y)
    assert scaler is not None
    assert model is not None
    Xs = scaler.transform(X[:2])
    pred = model.predict(Xs)
    assert pred.shape == (2, 3)


# --- fetch_training_data (mocked) ---
def test_fetch_training_data_success():
    payload = {
        "batting": {"headers": ["runs"], "rows": [["10"]]},
        "bowling": {"headers": ["runs"], "rows": [["24"]]},
    }
    mock_resp = MagicMock()
    mock_resp.read.return_value = json.dumps(payload).encode()
    mock_resp.__enter__ = MagicMock(return_value=mock_resp)
    mock_resp.__exit__ = MagicMock(return_value=False)

    with patch("app.train_on_the_fly.urllib.request.urlopen", return_value=mock_resp):
        out = fetch_training_data("http://localhost:8080", "T20", "2024-10-30T00:00:00Z")
    assert out == payload


def test_fetch_training_data_sends_api_key():
    payload = {"batting": {"headers": [], "rows": []}, "bowling": {"headers": [], "rows": []}}
    mock_resp = MagicMock()
    mock_resp.read.return_value = json.dumps(payload).encode()
    mock_resp.__enter__ = MagicMock(return_value=mock_resp)
    mock_resp.__exit__ = MagicMock(return_value=False)

    with patch("app.train_on_the_fly.urllib.request.urlopen", return_value=mock_resp) as m_urlopen:
        with patch("app.train_on_the_fly.urllib.request.Request") as m_req:
            req_instance = MagicMock()
            m_req.return_value = req_instance
            fetch_training_data("http://goapp", "ODI", "2024-01-01T00:00:00Z", api_key="secret")
            m_urlopen.assert_called_once()
            req_instance.add_header.assert_called_once_with("X-API-Key", "secret")


def test_fetch_training_data_http_error():
    import urllib.error

    class HTTPErrorWithRead(urllib.error.HTTPError):
        def read(self):
            return b"overloaded"

    err = HTTPErrorWithRead("http://x", 503, "Service Unavailable", None, None)
    err.fp = True  # truthy so code attempts body = e.read().decode()

    with patch("app.train_on_the_fly.urllib.request.urlopen", side_effect=err):
        with pytest.raises(ValueError) as excinfo:
            fetch_training_data("http://goapp", "T20", "2024-01-01T00:00:00Z")
    assert "503" in str(excinfo.value)
    assert "Go-app" in str(excinfo.value)


def test_fetch_training_data_os_error():
    with patch("app.train_on_the_fly.urllib.request.urlopen", side_effect=OSError("Connection refused")):
        with pytest.raises(ValueError) as excinfo:
            fetch_training_data("http://goapp", "T20", "2024-01-01T00:00:00Z")
    assert "Connection refused" in str(excinfo.value)


# --- train_on_the_fly (mocked fetch) ---
def test_train_on_the_fly_success():
    """Full pipeline with mocked fetch: valid batting + bowling data -> returns two (scaler, model) pairs."""
    bat_headers = _batting_headers()
    bowl_headers = _bowling_headers()
    data = {
        "batting": {"headers": bat_headers, "rows": [_one_batting_row(), _one_batting_row()]},
        "bowling": {"headers": bowl_headers, "rows": [_one_bowling_row(), _one_bowling_row()]},
    }

    with patch("app.train_on_the_fly.fetch_training_data", return_value=data):
        (scaler_bat, model_bat), (scaler_bowl, model_bowl) = train_on_the_fly(
            "http://goapp", "T20", "2024-10-30T00:00:00Z"
        )
    assert scaler_bat is not None
    assert model_bat is not None
    assert scaler_bowl is not None
    assert model_bowl is not None
    # Predict with one sample
    X_bat = np.random.RandomState(1).rand(1, len(BATTING_FEATURE_COLS))
    X_bowl = np.random.RandomState(2).rand(1, len(BOWLING_FEATURE_COLS))
    out_bat = model_bat.predict(scaler_bat.transform(X_bat))
    out_bowl = model_bowl.predict(scaler_bowl.transform(X_bowl))
    assert out_bat.shape == (1, len(BATTING_TARGET_COLS))
    assert out_bowl.shape == (1, len(BOWLING_TARGET_COLS))


def test_train_on_the_fly_insufficient_batting_raises():
    data = {
        "batting": {"headers": _batting_headers(), "rows": []},  # no rows
        "bowling": {"headers": _bowling_headers(), "rows": [_one_bowling_row()]},
    }
    with patch("app.train_on_the_fly.fetch_training_data", return_value=data):
        with pytest.raises(ValueError) as excinfo:
            train_on_the_fly("http://goapp", "T20", "2024-10-30T00:00:00Z")
    assert "Insufficient batting" in str(excinfo.value)
    assert "T20" in str(excinfo.value)


def test_train_on_the_fly_insufficient_bowling_raises():
    data = {
        "batting": {"headers": _batting_headers(), "rows": [_one_batting_row()]},
        "bowling": {"headers": _bowling_headers(), "rows": []},
    }
    with patch("app.train_on_the_fly.fetch_training_data", return_value=data):
        with pytest.raises(ValueError) as excinfo:
            train_on_the_fly("http://goapp", "ODI", "2024-06-01T00:00:00Z")
    assert "Insufficient bowling" in str(excinfo.value)
    assert "ODI" in str(excinfo.value)


def test_train_on_the_fly_all_nan_batting_raises():
    """Insufficient batting: zero batting rows after filtering raises ValueError."""
    # Use no batting rows so that X_bat is empty and we get "Insufficient batting"
    data = {
        "batting": {"headers": _batting_headers(), "rows": []},
        "bowling": {"headers": _bowling_headers(), "rows": [_one_bowling_row()]},
    }
    with patch("app.train_on_the_fly.fetch_training_data", return_value=data):
        with pytest.raises(ValueError) as excinfo:
            train_on_the_fly("http://goapp", "T20", "2024-10-30T00:00:00Z")
    assert "Insufficient batting" in str(excinfo.value)
