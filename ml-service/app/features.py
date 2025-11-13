from typing import Any, List

from .feature_config import get_feature_names


def batting_feature_vector(f: Any) -> List[float]:
    names = get_feature_names("batting")
    return [getattr(f, n) for n in names]


def bowling_feature_vector(f: Any) -> List[float]:
    names = get_feature_names("bowling")
    return [getattr(f, n) for n in names]
