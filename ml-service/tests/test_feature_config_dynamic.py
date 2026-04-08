import importlib
import json
from pathlib import Path


def _default_config_path() -> Path:
    """Path to repo configs/feature_vectors.json (v2 raw stat schema)."""
    return Path(__file__).resolve().parent.parent.parent / "configs" / "feature_vectors.json"


def write_temp_config(tmp_path: Path):
    """Copy repo config and swap first two names for batting and bowling to test order."""
    path = _default_config_path()
    data = json.loads(path.read_text(encoding="utf-8"))
    batting = list(data["batting"])
    bowling = list(data["bowling"])
    batting[0], batting[1] = batting[1], batting[0]
    bowling[0], bowling[1] = bowling[1], bowling[0]
    data = {"batting": batting, "bowling": bowling}
    p = tmp_path / "feature_vectors.json"
    p.write_text(json.dumps(data), encoding="utf-8")
    return p, data


def test_dynamic_order_from_feature_config(tmp_path, monkeypatch):
    # Point to temp config overriding order (first two swapped for both kinds)
    cfg_path, cfg = write_temp_config(tmp_path)
    monkeypatch.setenv("FEATURE_CONFIG_PATH", str(cfg_path))

    m_config = importlib.import_module("app.feature_config")
    importlib.reload(m_config)
    m = importlib.import_module("app.main")
    importlib.reload(m)

    # Sentinel values: first config slot = 2.2, second = 1.1, then 3, 4, 5, ...
    # Constrained int fields must stay within model bounds (e.g. viscosity le=1, inning le=2).
    def kwargs_for_order(names, int_keys=None, clamp_int=None):
        int_keys = set(int_keys or ())
        clamp_int = clamp_int or {}
        out = {}
        for i, name in enumerate(names):
            if i == 0:
                out[name] = 2.2
            elif i == 1:
                out[name] = 1.1
            else:
                val = (i + 1) if name in int_keys else float(i + 1)
                if name in clamp_int:
                    lo, hi = clamp_int[name]
                    clamped = lo if val < lo else (hi if val > hi else val)
                    val = int(clamped) if name in int_keys else float(clamped)
                out[name] = val
        return out

    # BattingFeatures int fields and their valid ranges (from app.models)
    bat_int = {
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
    }
    bat_clamp = {
        "batting_viscosity": (0, 1), "batting_inning": (1, 2), "batting_session": (1, 3), "toss": (0, 1),
        "month_sin": (-1, 1), "month_cos": (-1, 1), "day_of_week_sin": (-1, 1), "day_of_week_cos": (-1, 1),
    }
    bat_kwargs = kwargs_for_order(cfg["batting"], bat_int, bat_clamp)
    bat_kwargs["player_name"] = "P"
    bat_kwargs["format"] = "ODI"
    bf = m.BattingFeatures(**bat_kwargs)
    vec = m.batting_feature_vector(bf)
    expected_bat = [float(bat_kwargs[n]) for n in cfg["batting"]]
    assert vec == expected_bat, (vec, expected_bat)

    bowl_int = {
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
    }
    bowl_clamp = {
        "bowling_viscosity": (0, 1), "batting_inning": (1, 2), "bowling_session": (1, 3), "toss": (0, 1),
        "month_sin": (-1, 1), "month_cos": (-1, 1), "day_of_week_sin": (-1, 1), "day_of_week_cos": (-1, 1),
    }
    bowl_kwargs = kwargs_for_order(cfg["bowling"], bowl_int, bowl_clamp)
    bowl_kwargs["player_name"] = "P"
    bowl_kwargs["format"] = "ODI"
    bwf = m.BowlingFeatures(**bowl_kwargs)
    vec2 = m.bowling_feature_vector(bwf)
    expected_bowl = [float(bowl_kwargs[n]) for n in cfg["bowling"]]
    assert vec2 == expected_bowl, (vec2, expected_bowl)
