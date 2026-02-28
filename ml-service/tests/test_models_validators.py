import importlib
import os


def _load(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    m = importlib.import_module("app.main")
    importlib.reload(m)
    return m


def test_batting_features_format_normalization(tmp_path):
    m = _load(tmp_path)
    f = m.BattingFeatures(
        batting_consistency=0.0,
        batting_form=0.0,
        batting_temp=0,
        batting_wind=0,
        batting_rain=0,
        batting_humidity=0,
        batting_cloud=0,
        batting_pressure=0,
        batting_viscosity=0,
        batting_inning=1,
        batting_session=1,
        toss=0,
        venue=0.0,
        opposition=0.0,
        season=0,
        player_name="P",
        format="  odi  ",
    )
    assert f.format == "ODI"


def test_backtest_predict_request_teams_validator(tmp_path):
    """BacktestPredictRequest teams validator normalizes and enforces len==2."""
    from datetime import datetime

    import pytest

    from app.models import BacktestPredictRequest

    cutoff = datetime(2024, 1, 1)
    with pytest.raises(ValueError, match="exactly two"):
        BacktestPredictRequest(cutoff_date=cutoff, teams=["A"], player_ids=[1, 2], format="T20")
    with pytest.raises(ValueError, match="exactly two"):
        BacktestPredictRequest(cutoff_date=cutoff, teams=["A", "B", "C"], player_ids=[1, 2], format="T20")
    r = BacktestPredictRequest(cutoff_date=cutoff, teams=["  india  ", "  aus  "], player_ids=[1, 2], format="T20")
    assert r.teams == ["INDIA", "AUS"]


def test_backtest_predict_request_teams_none(tmp_path):
    """BacktestPredictRequest allows teams=None."""
    from datetime import datetime

    from app.models import BacktestPredictRequest

    cutoff = datetime(2024, 1, 1)
    r = BacktestPredictRequest(cutoff_date=cutoff, teams=None, player_ids=None, format="T20")
    assert r.teams is None
    assert r.player_ids is None


def test_backtest_predict_request_player_ids_validator(tmp_path):
    """BacktestPredictRequest player_ids validator rejects non-positive."""
    from datetime import datetime

    import pytest

    from app.models import BacktestPredictRequest

    cutoff = datetime(2024, 1, 1)
    with pytest.raises(ValueError, match="positive"):
        BacktestPredictRequest(cutoff_date=cutoff, teams=["A", "B"], player_ids=[1, 0], format="T20")


def test_extras_features_format_upper(tmp_path):
    """ExtrasFeatures format validator uppercases non-empty format (lines 203-207)."""
    from app.models import ExtrasFeatures

    f = ExtrasFeatures(format="  t20  ")
    assert f.format == "T20"


def test_extras_features_format_none_empty(tmp_path):
    """ExtrasFeatures format validator passes through None and empty string."""
    from app.models import ExtrasFeatures

    f = ExtrasFeatures(format=None)
    assert f.format is None
    f2 = ExtrasFeatures(format="")
    assert f2.format == ""


def test_win_features_format_upper(tmp_path):
    """WinFeatures format validator uppercases non-empty format (lines 239-243)."""
    from app.models import WinFeatures

    f = WinFeatures(format="  odi  ")
    assert f.format == "ODI"


def test_win_features_format_none_empty(tmp_path):
    """WinFeatures format validator passes through None and empty string."""
    from app.models import WinFeatures

    f = WinFeatures(format=None)
    assert f.format is None


def test_historical_match_filter_validators(tmp_path):
    """HistoricalMatchFilter format and team validators (lines 259-269)."""
    from datetime import datetime

    from app.models import HistoricalMatchFilter

    f = HistoricalMatchFilter(format="  t20  ", team1="  ind  ", team2="  pak  ", match_date=datetime(2024, 1, 1))
    assert f.format == "T20"
    assert f.team1 == "IND"
    assert f.team2 == "PAK"


def test_historical_match_backtest_request_match_id_none(tmp_path):
    """HistoricalMatchBacktestRequest with filters uses match_id=None (validator passes through)."""
    from datetime import datetime

    from app.models import HistoricalMatchBacktestRequest, HistoricalMatchFilter

    filt = HistoricalMatchFilter(format="T20", team1="IND", team2="PAK", match_date=datetime(2024, 1, 1))
    r = HistoricalMatchBacktestRequest(cutoff_date=datetime(2024, 1, 1), filters=filt)
    assert r.match_id is None


def test_historical_match_backtest_request_match_id_validator(tmp_path):
    """HistoricalMatchBacktestRequest match_id must be positive (lines 280-286)."""
    from datetime import datetime

    import pytest

    from app.models import HistoricalMatchBacktestRequest

    cutoff = datetime(2024, 1, 1)
    with pytest.raises(ValueError, match="positive"):
        HistoricalMatchBacktestRequest(cutoff_date=cutoff, match_id=0)
    with pytest.raises(ValueError, match="positive"):
        HistoricalMatchBacktestRequest(cutoff_date=cutoff, match_id=-1)
    r = HistoricalMatchBacktestRequest(cutoff_date=cutoff, match_id=1)
    assert r.match_id == 1


def test_historical_match_backtest_request_exactly_one_selector(tmp_path):
    """HistoricalMatchBacktestRequest requires exactly one of match_id or filters (lines 288-295)."""
    from datetime import datetime

    import pytest

    from app.models import HistoricalMatchBacktestRequest, HistoricalMatchFilter

    cutoff = datetime(2024, 1, 1)
    filt = HistoricalMatchFilter(format="T20", team1="IND", team2="PAK", match_date=cutoff)
    with pytest.raises(ValueError, match="exactly one"):
        HistoricalMatchBacktestRequest(cutoff_date=cutoff, match_id=1, filters=filt)
    r = HistoricalMatchBacktestRequest(cutoff_date=cutoff, filters=filt)
    assert r.filters is not None


def test_bowling_features_format_normalization(tmp_path):
    m = _load(tmp_path)
    f = m.BowlingFeatures(
        bowling_consistency=0.0,
        bowling_form=0.0,
        bowling_temp=0,
        bowling_wind=0,
        bowling_rain=0,
        bowling_humidity=0,
        bowling_cloud=0,
        bowling_pressure=0,
        bowling_viscosity=0,
        batting_inning=1,
        bowling_session=1,
        toss=0,
        bowling_venue=0.0,
        bowling_opposition=0.0,
        season=0,
        player_name="P",
        format=" t20i",
    )
    assert f.format == "T20I"
