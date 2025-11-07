from typing import Any, List


def batting_feature_vector(f: Any) -> List[float]:
    return [
        f.batting_consistency,
        f.batting_form,
        f.batting_temp,
        f.batting_wind,
        f.batting_rain,
        f.batting_humidity,
        f.batting_cloud,
        f.batting_pressure,
        f.batting_viscosity,
        f.batting_inning,
        f.batting_session,
        f.toss,
        f.venue,
        f.opposition,
        f.season,
    ]


def bowling_feature_vector(f: Any) -> List[float]:
    return [
        f.bowling_consistency,
        f.bowling_form,
        f.bowling_temp,
        f.bowling_wind,
        f.bowling_rain,
        f.bowling_humidity,
        f.bowling_cloud,
        f.bowling_pressure,
        f.bowling_viscosity,
        f.batting_inning,
        f.bowling_session,
        f.toss,
        f.bowling_venue,
        f.bowling_opposition,
        f.season,
    ]
