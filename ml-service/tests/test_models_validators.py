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
