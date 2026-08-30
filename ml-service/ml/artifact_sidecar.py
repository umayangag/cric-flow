"""Sidecar metadata for trained model artifacts.

Each trained model (innings, extras, per-format or legacy) writes a small JSON
file alongside its ``.joblib`` files. The sidecar pins two things that would
otherwise drift between training and inference:

- ``training_fields``: what the run measured -- holdout ``metrics``, ``trained_at``,
  ``duration_seconds`` and friends, built by
  :func:`ml.training_pipeline.training_metadata_fields`. Top-level rather than nested,
  because that is where ``app.model_stats_service`` looks.

- ``feature_names``: the exact column order the scaler/model were fitted on.
  After introducing per-format ``drop_low_variance_columns`` and optional
  one-hot exclusion, different formats can end up with different feature lists
  than the canonical ``*_FEATURE_COLS`` tuple, and reconciliation cannot assume
  a fixed shape.

Files are named ``{kind}_meta_{FMT}.json``. Readers must tolerate missing files
(old artifacts predate this convention), and the unsuffixed ``{kind}_meta.json``
name is still resolved so a stale sidecar left by the removed unified models can
be read rather than crashing a scan.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Dict, List, Optional

from ml.dataset_provenance import default_csv_dir, provenance_for

logger = logging.getLogger(__name__)


def meta_filename(kind: str, format_code: Optional[str]) -> str:
    """Return the sidecar filename for (kind, format_code)."""
    safe_fmt = (format_code or "").replace(" ", "_")
    if safe_fmt:
        return f"{kind}_meta_{safe_fmt}.json"
    return f"{kind}_meta.json"


def write_artifact_meta(
    out_dir: str,
    kind: str,
    format_code: Optional[str],
    feature_names: List[str],
    training_fields: Optional[Dict[str, Any]] = None,
    extra: Optional[Dict[str, Any]] = None,
) -> str:
    """Atomically write sidecar metadata alongside a trained model artifact.

    Returns the absolute path of the written file.
    """
    os.makedirs(out_dir, exist_ok=True)
    path = os.path.join(out_dir, meta_filename(kind, format_code))
    payload: Dict[str, Any] = {
        "kind": kind,
        "format_code": format_code,
        "feature_names": list(feature_names),
    }
    # Provenance sits beside feature_names because it answers the same kind of
    # question about the artifact: feature_names says what shape the model expects,
    # provenance says what data it learned from (ops plan P-1). Both are useless in
    # the export directory that produced them, because the next export overwrites it.
    provenance = provenance_for(default_csv_dir())
    if provenance:
        payload["provenance"] = provenance
    # Merged at the top level, not nested under "extra", because that is where
    # app.model_stats_service reads metrics, trained_at and duration_seconds from.
    if training_fields:
        payload.update(training_fields)
    if extra:
        payload["extra"] = dict(extra)
    tmp = f"{path}.tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2, sort_keys=True)
    os.replace(tmp, path)
    return path


def read_artifact_meta(models_dir: str, kind: str, format_code: Optional[str]) -> Optional[Dict[str, Any]]:
    """Read sidecar metadata; return ``None`` when file is missing or malformed."""
    path = os.path.join(models_dir, meta_filename(kind, format_code))
    if not os.path.isfile(path):
        return None
    try:
        with open(path, encoding="utf-8") as f:
            data = json.load(f)
    except (OSError, ValueError) as exc:
        logger.warning("artifact_sidecar.read_failed path=%s error=%s", path, exc)
        return None
    if not isinstance(data, dict):
        logger.warning("artifact_sidecar.read_invalid_shape path=%s", path)
        return None
    if not isinstance(data.get("feature_names"), list):
        logger.warning("artifact_sidecar.read_missing_feature_names path=%s", path)
        return None
    return data
