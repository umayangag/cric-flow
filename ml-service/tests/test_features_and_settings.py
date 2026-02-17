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
        bowling_form_short=2.0,
        bowling_form_long=1.8,
        bowling_momentum=0.5,
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
    # 18 base + 7 seq (0 when absent)
    assert bowl_vec[:18] == [
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
    assert bowl_vec[18:] == [0.0] * 8  # seq cols default to 0


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
