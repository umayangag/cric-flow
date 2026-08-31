"""
Optional feature transforms for training and prediction.

When ml.feature_transforms.<model>.add_interactions is configured, interaction columns
(e.g. form * venue) are appended to the base feature vector. When add_log1p is set,
those features are log1p-transformed. The same transform must be applied at prediction
time; the transform config is saved in metadata.

Config example (ml-service/config.json):
  "ml": {
    "feature_transforms": {
      "batting": {
        "add_interactions": [["batting_mean_w5", "venue"], ["batting_std_w10", "batting_mean_w5"]],
        "add_log1p": ["batting_mean_w5"]
      },
      "bowling": { "add_interactions": [], "add_log1p": [] },
      "fielding": { "add_interactions": [], "add_log1p": [] }
    }
  }

Feature names must match the base feature set. For batting, canonical names from
feature_vectors.json include "venue", "opposition"; CSV columns may use batting_venue.
Training uses csv_column_map to resolve.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Dict, List, Optional, Tuple

import numpy as np

logger = logging.getLogger(__name__)

# Map canonical feature names (prediction) to CSV column names (training) per model.
# Used when applying transforms during training; prediction uses canonical names.
CSV_COLUMN_MAP: Dict[str, Dict[str, str]] = {
    "batting": {
        "venue": "batting_venue",
        "opposition": "batting_opposition",
        "batting_temp": "temp",
        "batting_wind": "wind",
        "batting_rain": "rain",
        "batting_humidity": "humidity",
        "batting_cloud": "cloud",
        "batting_pressure": "pressure",
        "batting_viscosity": "viscosity",
        "batting_inning": "inning",
    },
    "bowling": {
        "batting_inning": "inning",
    },
    "fielding": {},
}


def get_transform_config(model_kind: str) -> Dict[str, Any]:
    """
    Load transform config from ml.feature_transforms.<model_kind>.
    Returns {add_interactions: [(a,b),...], add_log1p: [col,...], ...}.
    """
    try:
        from ml.config import _load

        cfg = _load()
        ft = (cfg.get("ml") or {}).get("feature_transforms") or {}
        block = ft.get(model_kind) if isinstance(ft.get(model_kind), dict) else {}
        raw_int = block.get("add_interactions") or []
        raw_log = block.get("add_log1p") or []
        out_int: List[Tuple[str, str]] = []
        for item in raw_int:
            if isinstance(item, (list, tuple)) and len(item) >= 2:
                a, b = str(item[0]).strip(), str(item[1]).strip()
                if a and b:
                    out_int.append((a, b))
        out_log: List[str] = [str(x).strip() for x in raw_log if isinstance(x, str) and x.strip()]
        return {"add_interactions": out_int, "add_log1p": out_log}
    except Exception:
        return {"add_interactions": [], "add_log1p": []}


def _resolve_for_training(name: str, model_kind: str) -> str:
    """Resolve canonical name to CSV column name for training."""
    m = CSV_COLUMN_MAP.get(model_kind, {})
    return m.get(name, name)


def get_interaction_specs(model_kind: str) -> List[Tuple[str, str]]:
    """Load interaction specs from config for the given model (batting, bowling, fielding)."""
    cfg = get_transform_config(model_kind)
    return cfg.get("add_interactions") or []


def apply_transforms(
    X: np.ndarray,
    feature_names: List[str],
    transform_config: Dict[str, Any],
    model_kind: str = "batting",
) -> Tuple[np.ndarray, List[str]]:
    """
    Apply log1p and interactions. Transform config from get_transform_config.
    For training, feature_names are CSV column names; use model_kind to resolve
    canonical names in config to CSV columns.
    Returns (X_transformed, extended_names).
    """
    X_work = X.astype(np.float64, copy=True)
    names = list(feature_names)
    name_to_idx = {n: i for i, n in enumerate(names)}

    # 1. log1p
    log1p_cols = transform_config.get("add_log1p") or []
    for canon in log1p_cols:
        col = _resolve_for_training(canon, model_kind)
        if col in name_to_idx:
            idx = name_to_idx[col]
            X_work[:, idx] = np.log1p(np.maximum(X_work[:, idx], 0))
        elif canon in name_to_idx:
            idx = name_to_idx[canon]
            X_work[:, idx] = np.log1p(np.maximum(X_work[:, idx], 0))

    # 2. interactions
    specs = transform_config.get("add_interactions") or []
    for a, b in specs:
        ca, cb = _resolve_for_training(a, model_kind), _resolve_for_training(b, model_kind)
        key_a = ca if ca in name_to_idx else a
        key_b = cb if cb in name_to_idx else b
        if key_a in name_to_idx and key_b in name_to_idx:
            col = (X_work[:, name_to_idx[key_a]] * X_work[:, name_to_idx[key_b]]).reshape(-1, 1)
            X_work = np.hstack([X_work, col])
            names.append(f"{a}_x_{b}")
            name_to_idx[f"{a}_x_{b}"] = len(names) - 1
        else:
            logger.warning("feature_transforms.skip_missing a=%s b=%s names=%s", a, b, list(name_to_idx.keys()))

    return X_work, names


def apply_interactions(
    X: np.ndarray,
    feature_names: List[str],
    interaction_specs: List[Tuple[str, str]],
    model_kind: str = "batting",
) -> Tuple[np.ndarray, List[str]]:
    """Convenience: append interaction columns only. Kept for backward compatibility."""
    return apply_transforms(X, feature_names, {"add_interactions": interaction_specs, "add_log1p": []}, model_kind)


def extend_feature_map(
    feature_map: Dict[str, float],
    interaction_specs: List[Tuple[str, str]],
) -> Dict[str, float]:
    """
    Add interaction values to a feature map. Used at prediction when building
    the feature vector from go-app's map. Uses canonical names (venue, opposition).
    """
    if not interaction_specs:
        return dict(feature_map)
    out = dict(feature_map)
    for a, b in interaction_specs:
        va = feature_map.get(a)
        vb = feature_map.get(b)
        if va is not None and vb is not None:
            try:
                out[f"{a}_x_{b}"] = float(va) * float(vb)
            except (TypeError, ValueError):
                pass
    return out


def _training_to_serving_names() -> Dict[str, str]:
    """CSV_COLUMN_MAP read the other way: training column name -> serving feature name.

    An interaction is recorded in the model's metadata under the names the *training*
    frame used, but at prediction the operands are looked up in the map go-app sends,
    which uses the serving names. ``inning`` / ``batting_inning`` is the pair that
    actually differs today; deriving the rest from the same table keeps one source of
    truth for the correspondence.
    """
    out: Dict[str, str] = {}
    for by_kind in CSV_COLUMN_MAP.values():
        for serving_name, csv_name in by_kind.items():
            out.setdefault(csv_name, serving_name)
    return out


def _interaction_operand(name: str, feature_map: Dict[str, float]) -> Any:
    """Look an interaction operand up under its own name, then its serving alias."""
    value = feature_map.get(name)
    if value is None:
        value = feature_map.get(_training_to_serving_names().get(name, name))
    return value


def build_extended_vector_from_features(
    base_values: List[float],
    base_names: List[str],
    feature_map: Dict[str, float],
    transform_config: Dict[str, Any],
) -> List[float]:
    """
    Build extended feature vector at prediction: base_values + log1p + interactions.
    base_values and base_names are from the canonical feature vector (get_feature_names).
    feature_map has canonical keys (venue, opposition, etc.) for interaction lookup.

    Every interaction the model was trained with must be produced. Dropping one silently
    yields a vector one column too narrow, which surfaces far away as an unreadable
    "X has N features, but ... is expecting N+1" from the scaler; so a missing or
    non-numeric operand raises here, where the name that is missing is still known.
    """
    arr = np.array([base_values], dtype=np.float64)
    names = list(base_names)
    name_to_idx = {n: i for i, n in enumerate(names)}

    # log1p (in-place on arr)
    for col in transform_config.get("add_log1p") or []:
        if col in name_to_idx:
            idx = name_to_idx[col]
            arr[:, idx] = np.log1p(np.maximum(arr[:, idx], 0))

    # interactions
    for a, b in transform_config.get("add_interactions") or []:
        va = _interaction_operand(a, feature_map)
        vb = _interaction_operand(b, feature_map)
        missing = [name for name, value in ((a, va), (b, vb)) if value is None]
        if missing:
            raise ValueError(
                f"interaction {a}_x_{b} cannot be built: operand(s) {missing} are absent from the "
                f"feature map; the model expects this column, so the vector would be short"
            )
        try:
            val = float(va) * float(vb)
        except (TypeError, ValueError) as exc:
            raise ValueError(f"interaction {a}_x_{b} has a non-numeric operand: {va!r}, {vb!r}") from exc
        arr = np.hstack([arr, np.array([[val]], dtype=np.float64)])
        names.append(f"{a}_x_{b}")

    return arr.ravel().tolist()


def load_transform_config_from_metadata(
    artifacts_dir: str, model_kind: str, format_suffix: Optional[str]
) -> Dict[str, Any]:
    """Load feature_transforms from model metadata JSON. Returns {} if absent."""
    suffix = (format_suffix or "LEGACY").replace(" ", "_")
    fname = f"{model_kind}_metadata_{suffix}.json"
    path = os.path.join(artifacts_dir, fname)
    if not os.path.isfile(path):
        return {}
    try:
        with open(path, "r", encoding="utf-8") as f:
            meta = json.load(f)
        return meta.get("feature_transforms") or {}
    except Exception:
        return {}
