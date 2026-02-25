from typing import Any, List

from .feature_config import get_feature_names


def _feature_value(obj: Any, name: str) -> float:
    """Get a feature value from object; use 0.0 for missing or None (e.g. optional seq columns)."""
    v = getattr(obj, name, 0.0)
    if v is None:
        return 0.0
    try:
        return float(v)
    except (TypeError, ValueError):
        return 0.0


def batting_feature_vector(f: Any) -> List[float]:
    """Build the batting feature vector in the exact order defined in config.

    The object `f` is expected to expose attributes matching the configured
    feature names (e.g., an instance of `BattingFeatures`). Optional seq columns
    use 0.0 when missing or None.
    """
    names = get_feature_names("batting")
    return [_feature_value(f, n) for n in names]


def bowling_feature_vector(f: Any) -> List[float]:
    """Build the bowling feature vector in the exact order defined in config.

    The object `f` is expected to expose attributes matching the configured
    feature names (e.g., an instance of `BowlingFeatures`). Optional seq columns
    use 0.0 when missing or None.
    """
    names = get_feature_names("bowling")
    return [_feature_value(f, n) for n in names]


# Config/export names used in feature_vectors.json and training vs FieldingFeatures attribute names
_FIELDING_NAME_TO_ATTR = {
    "inning": "fielding_inning",
    "toss": "fielding_toss",
    "season_id": "fielding_season",
}


def fielding_feature_vector(f: Any) -> List[float]:
    """Build the fielding feature vector in the exact order defined in config.

    Config names (e.g. inning, toss, season_id) are mapped to FieldingFeatures
    attribute names (fielding_inning, fielding_toss, fielding_season) when present.
    """
    names = get_feature_names("fielding")
    return [_feature_value(f, _FIELDING_NAME_TO_ATTR.get(n, n)) for n in names]
