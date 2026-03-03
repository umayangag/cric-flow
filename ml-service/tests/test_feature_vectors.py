import importlib
import os

from app.feature_config import get_feature_names


def test_batting_feature_vector_length(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    m = importlib.import_module("app.main")
    importlib.reload(m)

    f = m.BattingFeatures(
        batting_consistency=0.1,
        batting_form=0.2,
        batting_temp=25,
        batting_wind=3,
        batting_rain=0,
        batting_humidity=50,
        batting_cloud=10,
        batting_pressure=1000,
        batting_viscosity=0,
        batting_inning=1,
        batting_session=1,
        toss=0,
        venue=1.0,
        opposition=2.0,
        season=2024,
        player_name="P",
        format="ODI",
    )
    vec = m.batting_feature_vector(f)
    assert isinstance(vec, list)
    # batting length is defined by shared feature config
    assert len(vec) == len(get_feature_names("batting"))


def test_bowling_feature_vector_length(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    m = importlib.import_module("app.main")
    importlib.reload(m)

    f = m.BowlingFeatures(
        bowling_consistency=0.1,
        bowling_form=0.2,
        bowling_temp=25,
        bowling_wind=3,
        bowling_rain=0,
        bowling_humidity=50,
        bowling_cloud=10,
        bowling_pressure=1000,
        bowling_viscosity=0,
        batting_inning=1,
        bowling_session=1,
        toss=0,
        bowling_venue=1.0,
        bowling_opposition=2.0,
        season=2024,
        player_name="P",
        format="ODI",
    )
    vec = m.bowling_feature_vector(f)
    assert isinstance(vec, list)
    # bowling length is defined by shared feature config
    assert len(vec) == len(get_feature_names("bowling"))
