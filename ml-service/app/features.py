from typing import Any, List

from .feature_config import get_feature_names


def batting_feature_vector(f: Any) -> List[float]:
    """Build the batting feature vector in the exact order defined in config.

    The object `f` is expected to expose attributes matching the configured
    feature names (e.g., an instance of `BattingFeatures`).
    """
    names = get_feature_names("batting")
    return [getattr(f, n) for n in names]


def bowling_feature_vector(f: Any) -> List[float]:
    """Build the bowling feature vector in the exact order defined in config.

    The object `f` is expected to expose attributes matching the configured
    feature names (e.g., an instance of `BowlingFeatures`).
    """
    names = get_feature_names("bowling")
    return [getattr(f, n) for n in names]


def fielding_feature_vector(f: Any) -> List[float]:
    """Build the fielding feature vector in the exact order defined in config."""
    names = get_feature_names("fielding")
    return [getattr(f, n) for n in names]
