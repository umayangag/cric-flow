from typing import Any, List, TYPE_CHECKING

from .feature_config import get_feature_names

if TYPE_CHECKING:
    from .main import BattingFeatures, BowlingFeatures

def batting_feature_vector(f: BattingFeatures) -> List[float]:
    names = get_feature_names("batting")
    return [getattr(f, n) for n in names]


def bowling_feature_vector(f: BowlingFeatures) -> List[float]:
    names = get_feature_names("bowling")
    return [getattr(f, n) for n in names]
