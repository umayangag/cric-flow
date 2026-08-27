"""
Where a model's training data came from.

A CSV on disk says nothing about the dataset behind its rows, so a model trained from
one carries no way back to it — "which data produced this model?" stays an archaeology
exercise (ops plan P-1). go-app writes an `export-manifest.json` beside the CSVs
recording the dataset digest; this module reads it and shapes it for a model's sidecar.

The provenance travels *with the artifact* rather than staying in the export directory,
because that directory is overwritten by the next export. A model file that cannot say
what it was trained on is one nobody can trust six months later.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any, Dict, Optional

logger = logging.getLogger(__name__)

#: The manifest go-app's exporter leaves beside its CSVs.
EXPORT_MANIFEST_NAME = "export-manifest.json"

#: Fields copied from the export manifest into a model's sidecar. Listed rather than
#: copied wholesale so a future manifest field does not silently start appearing in
#: every artifact.
PROVENANCE_FIELDS = (
    "dataset_sha256",
    "dataset_source_url",
    "dataset_feed",
    "dataset_extracted_at",
    "dataset_match_files",
)


def export_manifest_path(csv_dir: str) -> str:
    """Return the manifest location for an export directory."""
    return os.path.join(csv_dir or ".", EXPORT_MANIFEST_NAME)


def read_export_manifest(csv_dir: str) -> Dict[str, Any]:
    """Read the export manifest, or {} when there is none.

    A missing manifest is a normal state, not an error: CSVs produced before P-1, or
    by hand, have none. Returning {} lets the caller record "unknown", which is the
    honest answer and the one P-2 flags.
    """
    path = export_manifest_path(csv_dir)
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError:
        return {}
    except (json.JSONDecodeError, OSError) as e:
        logger.warning("dataset_provenance.read_failed path=%s error=%s", path, e)
        return {}
    return data if isinstance(data, dict) else {}


def provenance_for(csv_dir: str, cutoff: Optional[str] = None) -> Dict[str, Any]:
    """Build the provenance block for a model's sidecar metadata.

    `cutoff` is included because it already varies per run and is not recoverable from
    the artifact otherwise: two models trained from the same export with different
    cutoffs are different models, and nothing else on disk says so.

    Returns {} when nothing is known, so a caller can omit the key entirely rather
    than write an object full of nulls that reads as "we looked and found nothing"
    when in fact nobody looked.
    """
    manifest = read_export_manifest(csv_dir)
    source = manifest.get("provenance")
    provenance: Dict[str, Any] = {}
    if isinstance(source, dict):
        for field in PROVENANCE_FIELDS:
            value = source.get(field)
            if value not in (None, "", 0):
                provenance[field] = value

    if manifest.get("exported_at"):
        provenance["exported_at"] = manifest["exported_at"]
    if cutoff:
        provenance["training_cutoff"] = cutoff
    return provenance


def default_csv_dir() -> str:
    """The directory training reads CSVs from, resolved the same way trainers do."""
    from ml import config as svc_config

    return os.environ.get("GO_APP_OUTPUT_DIR") or svc_config.default_go_app_export_dir()


def attach(metadata: Dict[str, Any], csv_dir: Optional[str] = None, cutoff: Optional[str] = None) -> Dict[str, Any]:
    """Add a `provenance` block to a model's metadata, in place.

    Never raises: provenance is descriptive, and a model that trained successfully must
    not fail to save because its manifest was unreadable.
    """
    try:
        provenance = provenance_for(csv_dir if csv_dir is not None else default_csv_dir(), cutoff)
        if provenance:
            metadata["provenance"] = provenance
    except Exception as e:  # pragma: no cover - provenance_for is defensive already
        logger.warning("dataset_provenance.attach_failed error=%s", e)
    return metadata
