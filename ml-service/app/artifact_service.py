"""Artifact discovery and status reporting for the ML service.

Extracted from app.main. Provides:
- Per-format artifact file discovery
- Artifact status endpoint logic (formats × kinds matrix)
- Health endpoint artifact/metadata info helpers

Every kind reported here comes from `app.artifacts.ARTIFACT_KINDS`, so a model family
cannot exist in the loader and be invisible to these endpoints.
"""

import os
import time
from typing import Any, Dict, List, Optional, Tuple

from ml.config import get_format_codes

from .artifacts import ARTIFACT_KINDS, ARTIFACT_KINDS_BY_NAME, ArtifactKind, loaded_model_mtime
from .logging import get_struct_logger

logger = get_struct_logger()

SUPPORTED_FORMATS = get_format_codes()
ARTIFACT_KIND_NAMES = [kind.name for kind in ARTIFACT_KINDS]


def _iso(epoch_seconds: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(epoch_seconds))


def find_per_format_artifact(
    models_dir: str,
    fmt: str,
    kind: str,
) -> Optional[Tuple[str, float]]:
    """Return (path, mtime) of the model file for a per-format artifact, or None.

    A kind that has a scaler needs both files present: the model alone cannot be loaded.
    """
    artifact_kind = ARTIFACT_KINDS_BY_NAME.get(kind)
    if artifact_kind is None:
        return None
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.debug(
            "artifacts_status.find_per_format.listdir_failed",
            models_dir=models_dir,
            fmt=fmt,
            kind=kind,
            error=str(e),
        )
        return None
    scaler_name = artifact_kind.scaler_filename(fmt)
    if scaler_name is not None and scaler_name not in entries:
        return None
    model_name = artifact_kind.model_filename(fmt)
    if model_name not in entries:
        return None
    path = os.path.join(models_dir, model_name)
    try:
        st = os.stat(path)
        if not os.path.isfile(path):
            return None
        return path, st.st_mtime
    except Exception:
        return None


def find_artifact(models_dir: str, fmt: str, batting: bool) -> Optional[Tuple[str, float]]:
    """Return (path, mtime) for the first matching artifact if found."""
    return find_per_format_artifact(models_dir, fmt, "batting" if batting else "bowling")


def build_artifacts_status(models_dir: str) -> Dict[str, Any]:
    """Build the full artifacts status response (formats × kinds)."""
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    formats_out: Dict[str, Dict[str, Any]] = {
        fmt: {kind.name: _artifact_cell(models_dir, fmt, kind) for kind in ARTIFACT_KINDS} for fmt in SUPPORTED_FORMATS
    }
    return {"timestamp": ts, "root": models_dir, "formats": formats_out}


def _artifact_cell(models_dir: str, fmt: str, kind: ArtifactKind) -> Dict[str, Any]:
    """Presence, loaded state and staleness for one (format, kind) cell.

    `loaded` means the registry holds an object for that format. `stale` means the file on
    disk is newer than the one that object was loaded from — the state a finished training
    run leaves behind in a long-running process until the artifacts are reloaded, and the
    reason "loaded" alone cannot answer "is the current model serving?".
    """
    cell: Dict[str, Any] = {"exists": False}
    hit = find_per_format_artifact(models_dir, fmt, kind.name)
    if hit is not None:
        path, mtime = hit
        cell["exists"] = True
        cell["path"] = path
        cell["modified"] = _iso(mtime)
    if fmt not in kind.registry:
        return cell
    cell["loaded"] = True
    served_mtime = loaded_model_mtime(kind.name, fmt)
    if served_mtime is None:
        # In the registry but not attributable to a file this process loaded, so being current
        # cannot be claimed. Report stale: a reload is cheap and idempotent.
        cell["stale"] = True
        return cell
    cell["loaded_modified"] = _iso(served_mtime)
    cell["stale"] = hit is not None and hit[1] > served_mtime
    return cell


def build_health_response(models_dir: str) -> Dict[str, Any]:
    """Build the /health endpoint response with artifact and metadata info."""
    response: Dict[str, Any] = {
        "status": "ok",
        "models_dir": models_dir,
        "artifacts": {},
        "metadata": {},
        "counters": {},
    }
    for kind in ARTIFACT_KINDS:
        loaded = sorted(kind.registry.keys())
        response[f"loaded_{kind.name}_formats"] = loaded
        response["artifacts"][kind.name] = _artifacts_info(models_dir, kind)
        if kind.metadata_prefix is not None:
            response["metadata"][kind.name] = _metadata_info(models_dir, kind.metadata_prefix)
        response["counters"][f"{kind.name}_formats"] = len(loaded)
    return response


def _artifacts_info(models_dir: str, kind: ArtifactKind) -> List[dict]:
    """List a kind's .joblib files with size and mtime.

    Matching the kind's own filename prefixes rather than its bare name is what keeps
    `batting_share_*` out of the `batting` group.
    """
    prefixes = kind.joblib_prefixes()
    out = []
    try:
        for fname in os.listdir(models_dir):
            lf = fname.lower()
            if not lf.endswith(".joblib") or not any(lf.startswith(p) for p in prefixes):
                continue
            fpath = os.path.join(models_dir, fname)
            try:
                st = os.stat(fpath)
                out.append(
                    {
                        "file": fname,
                        "size_bytes": st.st_size,
                        "modified": int(st.st_mtime),
                    }
                )
            except OSError as e:
                logger.warning("health.artifacts_info.stat_failed", file=fname, error=str(e))
                out.append({"file": fname})
    except OSError as e:
        logger.warning("health.artifacts_info.listdir_failed", models_dir=models_dir, error=str(e))
    return sorted(out, key=lambda x: x.get("file", ""))


def _metadata_info(models_dir: str, prefix: str) -> List[str]:
    """List .json metadata files matching prefix."""
    names: List[str] = []
    try:
        for fname in os.listdir(models_dir):
            lf = fname.lower()
            if lf.startswith(prefix) and lf.endswith(".json"):
                names.append(fname)
    except OSError as e:
        logger.warning("health.metadata_info.listdir_failed", models_dir=models_dir, error=str(e))
    return sorted(names)
