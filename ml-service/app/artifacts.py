"""Model artifact registry and loader utilities.

This module keeps in-memory registries mapping a format code (e.g., "T20") to
the tuple (scaler, model), or to the bare model for kinds trained without a
scaler. Artifacts are always per-format, with filenames like
`batting_scaler_T20.joblib` and `batting_model_T20.joblib`, discovered under a
configured models directory.

`ARTIFACT_KINDS` is the single source of truth for which artifact families exist
and how they are named on disk. Loading, `/health` and `/artifacts/status` all
derive from it, so adding a model kind takes one entry rather than an edit to
every reader — the omission that once hid the innings model from both endpoints.

Use `reload(models_dir)` to (re)scan a directory and populate the registries.
The lightweight `summary()` function returns a snapshot indicating which
formats are currently loaded.
"""

import os
from dataclasses import dataclass
from typing import Any, Dict, List, MutableMapping, Optional, Tuple

import joblib

from app.logging import get_struct_logger

logger = get_struct_logger()

# Registries: map format code -> (scaler, model).
BAT_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
FIELD_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
EXTRAS_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level extras regressor)
WIN_MODELS: Dict[str, Optional[object]] = {}  # format -> model (match-level win classifier)
INNINGS_MODELS: Dict[
    str, Tuple[Optional[object], Optional[object]]
] = {}  # format -> (scaler, model) for innings runs/wickets
# Phase 3 share models: predict runs_share, wickets_share; multiply by innings totals for consistency
BAT_SHARE_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}
BOWL_SHARE_MODELS: Dict[str, Tuple[Optional[object], Optional[object]]] = {}

# Sidecar metadata per (kind, format). See ml.artifact_sidecar. Keyed by the same format code
# convention (e.g. "T20") so prediction code can look up alongside the model.
INNINGS_META: Dict[str, Dict[str, Any]] = {}
EXTRAS_META: Dict[str, Dict[str, Any]] = {}

# mtime of the model file each registry entry was loaded from, keyed by (kind name, format code).
# Without it "loaded" only means some object sits in the registry; with it `/artifacts/status`
# can say whether that object came from the file currently on disk, which is what a finished
# training run changes underneath a long-running process.
_LOADED_MODEL_MTIMES: Dict[Tuple[str, str], float] = {}


@dataclass(frozen=True)
class ArtifactKind:
    """One family of per-format artifacts: how it is named on disk, and where it loads to.

    `registry` and `meta_registry` hold references to the module-level dicts above, which are
    mutated in place and never rebound, so consumers importing those dicts by name stay in sync.
    """

    name: str
    registry: MutableMapping[str, Any]
    model_prefix: str
    scaler_prefix: Optional[str] = None
    meta_registry: Optional[MutableMapping[str, Dict[str, Any]]] = None
    # Prefix of the JSON sidecars reported by /health, when the kind writes any.
    metadata_prefix: Optional[str] = None

    @property
    def discovery_prefix(self) -> str:
        """Filename prefix scanned to find which formats exist for this kind.

        The scaler anchors discovery where there is one: a model whose scaler is missing
        cannot be used for inference, so a half-written pair must not register.
        """
        return self.scaler_prefix or self.model_prefix

    @property
    def has_scaler(self) -> bool:
        return self.scaler_prefix is not None

    def model_filename(self, format_code: str) -> str:
        return f"{self.model_prefix}{format_code}.joblib"

    def scaler_filename(self, format_code: str) -> Optional[str]:
        return f"{self.scaler_prefix}{format_code}.joblib" if self.scaler_prefix else None

    def joblib_prefixes(self) -> List[str]:
        """Every filename prefix that belongs to this kind, for directory listings."""
        return [self.model_prefix] + ([self.scaler_prefix] if self.scaler_prefix else [])


ARTIFACT_KINDS: Tuple[ArtifactKind, ...] = (
    ArtifactKind(
        name="batting",
        registry=BAT_MODELS,
        model_prefix="batting_model_",
        scaler_prefix="batting_scaler_",
        metadata_prefix="batting_metadata_",
    ),
    ArtifactKind(
        name="bowling",
        registry=BOWL_MODELS,
        model_prefix="bowling_model_",
        scaler_prefix="bowling_scaler_",
        metadata_prefix="bowling_metadata_",
    ),
    ArtifactKind(
        name="fielding",
        registry=FIELD_MODELS,
        model_prefix="fielding_model_",
        scaler_prefix="fielding_scaler_",
        metadata_prefix="fielding_metadata_",
    ),
    ArtifactKind(
        name="extras",
        registry=EXTRAS_MODELS,
        model_prefix="extras_model_",
        meta_registry=EXTRAS_META,
        metadata_prefix="extras_meta_",
    ),
    # Win sidecars are named win_model_<FMT>_metadata.json, so the model prefix finds them.
    ArtifactKind(
        name="win",
        registry=WIN_MODELS,
        model_prefix="win_model_",
        metadata_prefix="win_model_",
    ),
    ArtifactKind(
        name="innings",
        registry=INNINGS_MODELS,
        model_prefix="innings_model_",
        scaler_prefix="innings_scaler_",
        meta_registry=INNINGS_META,
        metadata_prefix="innings_meta_",
    ),
    ArtifactKind(
        name="batting_share",
        registry=BAT_SHARE_MODELS,
        model_prefix="batting_share_model_",
        scaler_prefix="batting_share_scaler_",
    ),
    ArtifactKind(
        name="bowling_share",
        registry=BOWL_SHARE_MODELS,
        model_prefix="bowling_share_model_",
        scaler_prefix="bowling_share_scaler_",
    ),
)

ARTIFACT_KINDS_BY_NAME: Dict[str, ArtifactKind] = {kind.name: kind for kind in ARTIFACT_KINDS}


def loaded_model_mtime(kind_name: str, format_code: str) -> Optional[float]:
    """mtime of the file the loaded (kind, format) model came from, or None if not loaded here."""
    return _LOADED_MODEL_MTIMES.get((kind_name, format_code))


def _load_meta(models_dir: str, kind: str, format_code: Optional[str]) -> Optional[Dict[str, Any]]:
    """Best-effort sidecar load; returns ``None`` when the ml package is unavailable."""
    try:
        from ml.artifact_sidecar import read_artifact_meta
    except ImportError:
        return None
    return read_artifact_meta(models_dir, kind, format_code)


def _use_share_models() -> bool:
    """True if ml.use_share_models is enabled in config."""
    try:
        from ml.config import get_config

        return bool((get_config().get("ml") or {}).get("use_share_models"))
    except Exception:
        return False


def _discover_format_codes(entries: List[str], kind: ArtifactKind) -> List[str]:
    """Format codes this kind has artifacts for, read off the directory listing."""
    prefix = kind.discovery_prefix
    suffix = ".joblib"
    codes: List[str] = []
    for fname in entries:
        lf = fname.lower()
        if lf.startswith(prefix) and lf.endswith(suffix):
            codes.append(fname[len(prefix) : -len(suffix)].upper())
    return codes


def _load_format(models_dir: str, kind: ArtifactKind, format_code: str) -> None:
    """Load one (kind, format) artifact into its registry, with its sidecar when there is one."""
    scaler = None
    scaler_name = kind.scaler_filename(format_code)
    if scaler_name is not None:
        scaler = joblib.load(os.path.join(models_dir, scaler_name))
    model_path = os.path.join(models_dir, kind.model_filename(format_code))
    if not os.path.exists(model_path):
        logger.warning(
            f"artifacts.load_per_format.{kind.name}_model_missing",
            format=format_code,
            artifact_type=kind.name,
            path=model_path,
        )
        return
    model = joblib.load(model_path)
    kind.registry[format_code] = (scaler, model) if kind.has_scaler else model
    if kind.meta_registry is not None:
        meta = _load_meta(models_dir, kind.name, format_code)
        if meta is not None:
            kind.meta_registry[format_code] = meta
    _LOADED_MODEL_MTIMES[(kind.name, format_code)] = os.path.getmtime(model_path)
    logger.info(
        f"artifacts.load_per_format.{kind.name}",
        format=format_code,
        artifact_type=kind.name,
        models_dir=models_dir,
    )


def _load_per_format(models_dir: str) -> None:
    """Load per-format joblib artifacts for every kind in ``ARTIFACT_KINDS``.

    Security: joblib uses pickle; only load artifacts from trusted sources and restrict
    filesystem access to models_dir to avoid insecure deserialization.
    """
    try:
        entries = os.listdir(models_dir)
    except OSError as e:
        logger.error("artifacts.load_per_format.listdir_failed", models_dir=models_dir, error=str(e))
        return
    for kind in ARTIFACT_KINDS:
        for format_code in _discover_format_codes(entries, kind):
            try:
                _load_format(models_dir, kind, format_code)
            except Exception as e:
                logger.error(
                    "artifacts.load_per_format.load_failed",
                    fname=f"{kind.discovery_prefix}{format_code}.joblib",
                    models_dir=models_dir,
                    error=str(e),
                )


def reload(models_dir: str) -> dict:
    logger.info("artifacts.reload.start", models_dir=models_dir)
    for kind in ARTIFACT_KINDS:
        kind.registry.clear()
        if kind.meta_registry is not None:
            kind.meta_registry.clear()
    _LOADED_MODEL_MTIMES.clear()
    _load_per_format(models_dir)
    out = summary()
    logger.info(
        "artifacts.reload.done",
        models_dir=models_dir,
        **{f"{kind.name}_formats": sorted(kind.registry.keys()) for kind in ARTIFACT_KINDS},
    )
    return out


def summary() -> dict:
    return {f"loaded_{kind.name}_formats": sorted(kind.registry.keys()) for kind in ARTIFACT_KINDS}
