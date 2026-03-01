import importlib
import os
from types import SimpleNamespace


def test_batting_and_bowling_feature_vectors_content(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    features_mod = importlib.import_module("app.features")
    importlib.reload(features_mod)

    bat = SimpleNamespace(
        batting_consistency=1.1,
        batting_form=2.2,
        batting_form_short=2.0,
        batting_form_long=1.8,
        batting_momentum=0.5,
        batting_temp=30,
        batting_wind=5,
        batting_rain=0,
        batting_humidity=60,
        batting_cloud=10,
        batting_pressure=1000,
        batting_viscosity=1,
        batting_inning=2,
        batting_session=3,
        toss=1,
        venue=7.5,
        opposition=8.5,
        season=2024,
    )
    bat_vec = features_mod.batting_feature_vector(bat)
    # 18 base + 8 seq (0 when absent)
    assert bat_vec[:18] == [
        1.1,
        2.2,
        2.0,
        1.8,
        0.5,
        30,
        5,
        0,
        60,
        10,
        1000,
        1,
        2,
        3,
        1,
        7.5,
        8.5,
        2024,
    ]
    assert bat_vec[18:] == [0.0] * 8  # seq cols default to 0

    bowl = SimpleNamespace(
        bowling_consistency=1.1,
        bowling_form=2.2,
        bowling_momentum=0.5,
        bowling_career_avg=1.5,
        bowling_temp=30,
        bowling_wind=5,
        bowling_rain=0,
        bowling_humidity=60,
        bowling_cloud=10,
        bowling_pressure=1000,
        bowling_viscosity=1,
        batting_inning=2,
        bowling_session=3,
        toss=1,
        bowling_venue=7.5,
        bowling_opposition=8.5,
        season=2024,
    )
    bowl_vec = features_mod.bowling_feature_vector(bowl)
    # 17 base + 8 seq (0 when absent): consistency, form, momentum, career_avg, temp..viscosity, inning, session, toss, venue, opp, season
    assert bowl_vec[:17] == [
        1.1,
        2.2,
        0.5,
        1.5,  # bowling_career_avg
        30,
        5,
        0,
        60,
        10,
        1000,
        1,
        2,
        3,
        1,
        7.5,
        8.5,
        2024,
    ]
    assert bowl_vec[17:] == [0.0] * 8  # seq cols default to 0


def test_feature_value_handles_none_and_non_numeric(tmp_path):
    """_feature_value returns 0.0 for None and non-convertible values."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    features_mod = importlib.import_module("app.features")
    importlib.reload(features_mod)

    # None -> 0.0
    obj_none = SimpleNamespace(x=None)
    assert features_mod._feature_value(obj_none, "x") == 0.0

    # Non-numeric (TypeError/ValueError) -> 0.0
    obj_bad = SimpleNamespace(x="not_a_number")
    assert features_mod._feature_value(obj_bad, "x") == 0.0

    # Missing attribute -> 0.0 (getattr default)
    obj_missing = SimpleNamespace()
    assert features_mod._feature_value(obj_missing, "nonexistent") == 0.0


def test_feature_value_via_batting_vector_with_none_and_bad_types(tmp_path):
    """Batting vector uses _feature_value; object with None/bad seq value yields 0.0."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    features_mod = importlib.import_module("app.features")
    importlib.reload(features_mod)

    bat = SimpleNamespace(
        batting_consistency=1.0,
        batting_form=2.0,
        batting_form_short=2.0,
        batting_form_long=1.8,
        batting_momentum=0.0,
        batting_temp=25,
        batting_wind=0,
        batting_rain=0,
        batting_humidity=50,
        batting_cloud=0,
        batting_pressure=1013,
        batting_viscosity=0,
        batting_inning=1,
        batting_session=1,
        toss=0,
        venue=0.5,
        opposition=0.5,
        season=2024,
        bat_prev_sr=None,  # triggers _feature_value None path
        bat_prev_out_rate="nope",  # triggers except
        bat_window_sr_12_pp=120.0,
        bat_window_boundary_rate_12_pp=0.15,
        bat_entry_sr_1_6=100.0,
        bat_set_sr_13_30=130.0,
        bat_react_after_dot_sr=110.0,
        bat_after_k_dots_boundary_p_k2=0.2,
    )
    vec = features_mod.batting_feature_vector(bat)
    # Seq cols: bat_prev_sr (None->0), bat_prev_out_rate (bad->0), rest numeric
    idx_prev_sr = 18
    idx_prev_out = 19
    assert vec[idx_prev_sr] == 0.0
    assert vec[idx_prev_out] == 0.0


def test_fielding_feature_vector(tmp_path):
    """fielding_feature_vector builds vector from config order."""
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    features_mod = importlib.import_module("app.features")
    importlib.reload(features_mod)

    fld = SimpleNamespace(
        fielding_consistency=0.6,
        fielding_form=0.3,
        fielding_temp=28,
        fielding_wind=2,
        fielding_rain=0,
        fielding_humidity=55,
        fielding_cloud=5,
        fielding_pressure=1010,
        fielding_viscosity=0,
        fielding_inning=1,
        fielding_toss=1,
        fielding_venue=0.4,
        fielding_opposition=0.6,
        fielding_season=2024,
    )
    vec = features_mod.fielding_feature_vector(fld)
    assert len(vec) == 14
    assert vec[0] == 0.6
    assert vec[1] == 0.3
    assert vec[13] == 2024


def test_settings_get_models_dir_precedence(tmp_path, monkeypatch):
    settings_mod = importlib.import_module("app.settings")
    importlib.reload(settings_mod)

    # Case 1: ML_SERVICE_OUTPUT_DIR has highest priority
    monkeypatch.setenv("ML_SERVICE_OUTPUT_DIR", str(tmp_path / "a"))
    monkeypatch.delenv("MODELS_DIR", raising=False)
    result = settings_mod.get_models_dir(None)
    assert result == str(tmp_path / "a")

    # Case 2: fall back to MODELS_DIR when ML_SERVICE_OUTPUT_DIR is unset
    monkeypatch.delenv("ML_SERVICE_OUTPUT_DIR", raising=False)
    monkeypatch.setenv("MODELS_DIR", str(tmp_path / "b"))
    result = settings_mod.get_models_dir(None)
    assert result == str(tmp_path / "b")

    # Case 3: use svc_config.default_artifacts_dir when both envs unset
    class DummyCfg:
        def default_artifacts_dir(self):
            return str(tmp_path / "c")

    monkeypatch.delenv("ML_SERVICE_OUTPUT_DIR", raising=False)
    monkeypatch.delenv("MODELS_DIR", raising=False)
    result = settings_mod.get_models_dir(DummyCfg())
    assert result == str(tmp_path / "c")
