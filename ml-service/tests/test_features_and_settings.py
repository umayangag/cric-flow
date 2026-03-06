import importlib
import os
from types import SimpleNamespace

from app.feature_config import get_feature_names


def test_batting_and_bowling_feature_vectors_content(tmp_path):
    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(tmp_path)
    features_mod = importlib.import_module("app.features")
    importlib.reload(features_mod)

    # v2 contract: 18 raw stats first, then env/context, then seq (0 when absent)
    bat = SimpleNamespace(
        batting_mean_w3=0.5,
        batting_mean_w5=1.0,
        batting_mean_w10=1.2,
        batting_mean_w20=1.1,
        batting_std_w5=0.3,
        batting_std_w10=0.4,
        batting_max_w10=2.0,
        batting_min_w10=0.0,
        batting_median_w10=1.0,
        batting_last_1=1.5,
        batting_last_2=1.2,
        batting_last_3=1.0,
        batting_career_mean=1.1,
        batting_career_count=50.0,
        batting_pct_zero_w10=0.1,
        batting_trend_w5=0.05,
        batting_days_since_last=7.0,
        batting_innings_in_last_90d=10.0,
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
        match_date_unix=0.0,
    )
    bat_vec = features_mod.batting_feature_vector(bat)
    assert bat_vec[:18] == [0.5, 1.0, 1.2, 1.1, 0.3, 0.4, 2.0, 0.0, 1.0, 1.5, 1.2, 1.0, 1.1, 50.0, 0.1, 0.05, 7.0, 10.0]
    assert bat_vec[18:31] == [30, 5, 0, 60, 10, 1000, 1, 2, 3, 1, 7.5, 8.5, 2024]
    assert bat_vec[31:] == [0.0] * (len(bat_vec) - 31)  # match_date_unix + seq

    bowl = SimpleNamespace(
        bowling_mean_w3=0.8,
        bowling_mean_w5=1.0,
        bowling_mean_w10=1.1,
        bowling_mean_w20=1.05,
        bowling_std_w5=0.2,
        bowling_std_w10=0.3,
        bowling_max_w10=1.5,
        bowling_min_w10=0.5,
        bowling_median_w10=1.0,
        bowling_last_1=1.2,
        bowling_last_2=1.0,
        bowling_last_3=0.9,
        bowling_career_mean=1.05,
        bowling_career_count=40.0,
        bowling_pct_zero_w10=0.0,
        bowling_trend_w5=0.02,
        bowling_days_since_last=5.0,
        bowling_innings_in_last_90d=8.0,
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
        match_date_unix=0.0,
    )
    bowl_vec = features_mod.bowling_feature_vector(bowl)
    assert bowl_vec[:18] == [0.8, 1.0, 1.1, 1.05, 0.2, 0.3, 1.5, 0.5, 1.0, 1.2, 1.0, 0.9, 1.05, 40.0, 0.0, 0.02, 5.0, 8.0]
    assert bowl_vec[18:31] == [30, 5, 0, 60, 10, 1000, 1, 2, 3, 1, 7.5, 8.5, 2024]
    assert bowl_vec[31:] == [0.0] * (len(bowl_vec) - 31)


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
        batting_mean_w5=1.0,
        batting_std_w10=0.3,
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
    names = get_feature_names("batting")
    idx_prev_sr = names.index("bat_prev_sr")
    idx_prev_out = names.index("bat_prev_out_rate")
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
    names = get_feature_names("fielding")
    assert len(vec) == len(names)
    assert vec[0] == 0.6
    assert vec[1] == 0.3
    season_idx = names.index("season_id")
    assert vec[season_idx] == 2024


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
