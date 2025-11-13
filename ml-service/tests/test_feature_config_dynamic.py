import importlib
import json
from pathlib import Path


def write_temp_config(tmp_path: Path):
    data = {
        "batting": [
            "batting_form",
            "batting_consistency",
            "batting_temp",
            "batting_wind",
            "batting_rain",
            "batting_humidity",
            "batting_cloud",
            "batting_pressure",
            "batting_viscosity",
            "batting_inning",
            "batting_session",
            "toss",
            "venue",
            "opposition",
            "season",
        ],
        "bowling": [
            "bowling_form",
            "bowling_consistency",
            "bowling_temp",
            "bowling_wind",
            "bowling_rain",
            "bowling_humidity",
            "bowling_cloud",
            "bowling_pressure",
            "bowling_viscosity",
            "batting_inning",
            "bowling_session",
            "toss",
            "bowling_venue",
            "bowling_opposition",
            "season",
        ],
    }
    p = tmp_path / "feature_vectors.json"
    p.write_text(json.dumps(data), encoding="utf-8")
    return p, data


def test_dynamic_order_from_feature_config(tmp_path, monkeypatch):
    # Point to temp config overriding order (first two swapped for both kinds)
    cfg_path, cfg = write_temp_config(tmp_path)
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(cfg_path))

    m = importlib.import_module("app.main")
    importlib.reload(m)

    # Build BattingFeatures with sentinel distinct values per attribute
    bat_kwargs = {
        "batting_consistency": 1.1,
        "batting_form": 2.2,
        "batting_temp": 3,
        "batting_wind": 4,
        "batting_rain": 5,
        "batting_humidity": 6,
        "batting_cloud": 7,
        "batting_pressure": 8,
        "batting_viscosity": 9,
        "batting_inning": 1,
        "batting_session": 2,
        "toss": 1,
        "venue": 10.0,
        "opposition": 11.0,
        "season": 2024,
        "player_name": "P",
        "format": "ODI",
    }
    bf = m.BattingFeatures(**bat_kwargs)
    vec = m._batting_feature_vector(bf)
    assert vec == [bat_kwargs[name] for name in cfg["batting"]]

    bowl_kwargs = {
        "bowling_consistency": 1.1,
        "bowling_form": 2.2,
        "bowling_temp": 3,
        "bowling_wind": 4,
        "bowling_rain": 5,
        "bowling_humidity": 6,
        "bowling_cloud": 7,
        "bowling_pressure": 8,
        "bowling_viscosity": 9,
        "batting_inning": 1,
        "bowling_session": 2,
        "toss": 1,
        "bowling_venue": 10.0,
        "bowling_opposition": 11.0,
        "season": 2024,
        "player_name": "P",
        "format": "ODI",
    }
    bwf = m.BowlingFeatures(**bowl_kwargs)
    vec2 = m._bowling_feature_vector(bwf)
    assert vec2 == [bowl_kwargs[name] for name in cfg["bowling"]]
