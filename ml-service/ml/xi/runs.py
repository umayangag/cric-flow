"""Run identity (H-16): every retrain writes one directory, and every artifact in it is
named by the run that produced it.

Before this module an artifact was trusted because it loaded. Nothing asked which run
wrote it, what data it saw, or whether the arrays it holds are the arrays this code
knows how to read -- and D-6 (§10.5) is what that costs: a rating artifact written
before P-2 loaded without complaint, reported ``loaded: true``, and then raised
``IndexError: index 13433 is out of bounds for axis 1 with size 1024`` on the first
request touching a player past slot 1024, because ``_state_from_payload`` assigned only
the arrays the payload happened to carry.

The fix is not to zero-fill the missing arrays. It is for the loader to refuse a shape
it cannot serve and to say which run the artifacts are from, which is what a manifest
makes possible:

    <artifacts_dir>/
        current_run.json          -- {"run_id": "..."}: `current` is a pointer, not files
        runs/<run_id>/
            manifest.json         -- what this module writes and reads
            xi_ratings.joblib
            xi_win_<FMT>.joblib
            xi_perf_<FMT>.joblib
            xi_win_report.json

A directory of joblib files with no manifest is not a run and is refused as one.
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import subprocess
import uuid
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)

#: Directory under the artifacts root holding one subdirectory per run.
RUNS_DIRNAME = "runs"

#: The file that makes a directory a run.
MANIFEST_NAME = "manifest.json"

#: The pointer file naming the run being served. A file rather than a symlink so the
#: same code works on every filesystem the service is deployed to, and so the answer to
#: "which run is current?" is readable without following anything.
CURRENT_POINTER_NAME = "current_run.json"


class RunArtifactsInvalid(Exception):
    """A run's artifacts cannot be served, and the message says which run and why.

    Raised for a missing manifest and for an array whose width is not the width this
    code expects -- the two halves of D-6. It is deliberately not an ``IndexError``
    thrown from inside inference: the point is to fail at load, where the operator can
    still be told to retrain.
    """


@dataclass
class RunManifest:
    """What a run says about itself (H-16).

    Every field answers a question that was previously unanswerable from the artifacts:
    which code wrote them (``git_sha``), what they were trained on (``dataset_sha``,
    ``cutoff``), what the rating pass and the grid chose (``rating_params``,
    ``hyperparameters``), what the run measured (``metrics``), and what shape the arrays
    are in (``state_shape``), which is the one the loader checks before serving.

    ``formats`` names what the run *trained*; ``format_notes`` says, per format, why one
    of them carries no holdout metrics -- or why it was not trained at all (B-3). The two
    together are what make "trained, nothing to score" and "nothing trained" different
    answers rather than the same empty manifest.

    ``cutoff`` and ``ratings_through`` are two dates, not one written twice (P2-2). The
    cutoff is the training boundary the operator asked for -- today, for a refresh --
    and rows at or after it are the holdout. ``ratings_through`` is the last match date
    the rating pass actually consumed, which is the date every served prediction is "as
    of". On the dev box the two differed by a day on the served run (cutoff 2026-09-03,
    ratings through 2026-09-02), because the archive lags the calendar. Without the
    second date, "what date is this run's data?" could only be answered by loading the
    run's joblib, so the listing of runs on disk could not say which was worth loading.
    It is required, and the loader asserts it against the state (``XiStore.load``): a
    manifest without it, or one that disagrees with the state it describes, is refused
    by name rather than served with a date read from somewhere else (§8.7).
    """

    run_id: str
    created_at: str
    cutoff: str
    ratings_through: str
    dataset_sha: str
    git_sha: str
    rating_params: Dict[str, Any] = field(default_factory=dict)
    hyperparameters: Dict[str, Any] = field(default_factory=dict)
    metrics: Dict[str, Any] = field(default_factory=dict)
    state_shape: Dict[str, Any] = field(default_factory=dict)
    formats: List[str] = field(default_factory=list)
    #: format code -> why it has no headline metrics. Empty when every format was scored.
    format_notes: Dict[str, str] = field(default_factory=dict)
    report: str = ""

    def as_dict(self) -> Dict[str, Any]:
        return asdict(self)

    def summary(self) -> Dict[str, Any]:
        """The short form ``/xi/status`` and the runs listing carry.

        ``ratings_through`` is the manifest's own record of the date; the status's
        top-level ``ratings_through`` is read off the loaded state. The loader asserts
        the two agree, which is what lets a listing answer for a run nobody has loaded.
        """
        return {
            "run_id": self.run_id,
            "created_at": self.created_at,
            "cutoff": self.cutoff,
            "ratings_through": self.ratings_through,
            "dataset_sha": self.dataset_sha,
            "git_sha": self.git_sha,
            "formats": list(self.formats),
            "format_notes": dict(self.format_notes),
            "hyperparameters": dict(self.hyperparameters),
            "metrics": dict(self.metrics),
        }


def new_run_id() -> str:
    """A sortable, unique run id: the UTC minute-second stamp plus eight random hex.

    Sortable because "the newest run" has to be answerable from a directory listing;
    random-suffixed because two retrains started in the same second must not collide.
    """
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    return f"{stamp}-{uuid.uuid4().hex[:8]}"


def runs_dir(artifacts_dir: str) -> str:
    return os.path.join(artifacts_dir, RUNS_DIRNAME)


def run_dir(artifacts_dir: str, run_id: str) -> str:
    return os.path.join(runs_dir(artifacts_dir), run_id)


def manifest_path(directory: str) -> str:
    return os.path.join(directory, MANIFEST_NAME)


def git_sha() -> str:
    """The commit the running code came from, or "" when it cannot be established.

    Empty rather than a guess: a manifest that names the wrong commit is worse than one
    that admits it does not know, because only the first is quoted with confidence.
    """
    try:
        out = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=os.path.dirname(os.path.abspath(__file__)),
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:  # pragma: no cover - environment dependent
        logger.warning("runs.git_sha.failed error=%s", exc)
        return ""
    if out.returncode != 0:
        logger.warning("runs.git_sha.unavailable stderr=%s", out.stderr.strip())
        return ""
    return out.stdout.strip()


def dataset_sha(match_keys: List[str]) -> str:
    """A digest of the matches the rating pass consumed.

    It is computed from what was read rather than copied from the archive's checksum
    because ml-service does not mount the dataset directory: the question it has to be
    able to answer is "were these two runs trained on the same cricket?", and the list
    of match identities the pass actually walked answers it exactly, on either source.
    """
    digest = hashlib.sha256()
    for key in sorted(match_keys):
        digest.update(key.encode("utf-8"))
        digest.update(b"\n")
    digest.update(f"n={len(match_keys)}".encode("utf-8"))
    return digest.hexdigest()


def write_manifest(directory: str, manifest: RunManifest) -> str:
    """Write the run's manifest. Written last, so a directory only becomes a run once
    everything it names is on disk."""
    os.makedirs(directory, exist_ok=True)
    path = manifest_path(directory)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(manifest.as_dict(), fh, indent=2)
    logger.info("runs.manifest.written run_id=%s path=%s", manifest.run_id, path)
    return path


def read_manifest(directory: str) -> RunManifest:
    """Read a run's manifest, or raise ``RunArtifactsInvalid`` naming the directory.

    A missing or unreadable manifest is refused rather than defaulted, because the whole
    point of H-16 is that an artifact set nothing can attribute to a run is not one.
    """
    path = manifest_path(directory)
    try:
        with open(path, encoding="utf-8") as fh:
            raw = json.load(fh)
    except FileNotFoundError as exc:
        raise RunArtifactsInvalid(
            f"{directory} has no {MANIFEST_NAME}, so nothing says which run produced its artifacts "
            f"or what shape they are in (H-16); run `make retrain` to produce a run this code wrote"
        ) from exc
    except (json.JSONDecodeError, OSError) as exc:
        raise RunArtifactsInvalid(f"{path} is not readable as a run manifest: {exc}") from exc
    if not isinstance(raw, dict) or not raw.get("run_id"):
        raise RunArtifactsInvalid(f"{path} carries no run_id, so it does not identify a run")
    if not raw.get("ratings_through"):
        # Written before P2-2. The date is inside the joblib, but reading it from there
        # would be serving a date the manifest never recorded (§8.7); the project is not
        # live and nothing is backfilled -- an older run is retrained, not patched.
        raise RunArtifactsInvalid(
            f"run {raw['run_id']}: {MANIFEST_NAME} carries no ratings_through, so the date its data runs "
            f"through is not written down; it was written before the field existed and cannot be loaded. "
            f"Run `make retrain` to produce a run that records its date."
        )
    known = {f for f in RunManifest.__dataclass_fields__}  # noqa: SIM118 - dataclass field names
    return RunManifest(**{k: v for k, v in raw.items() if k in known})


def list_runs(artifacts_dir: str) -> List[Dict[str, Any]]:
    """Every run on disk, newest first, each saying whether it can be loaded and why not.

    A directory that is not a loadable run -- no manifest, or a manifest written before
    it recorded its date -- is listed rather than hidden, with the reason in ``refused``:
    an operator looking for the artifacts that will not load needs to see them, and a
    blank beside a run id would read as "nothing to say" rather than "cannot be served"
    (§8.7). ``refused`` is ``None`` on every loadable run, so the key is always there.
    """
    root = runs_dir(artifacts_dir)
    try:
        entries = sorted((e for e in os.listdir(root) if os.path.isdir(os.path.join(root, e))), reverse=True)
    except OSError:
        return []
    out: List[Dict[str, Any]] = []
    for name in entries:
        directory = os.path.join(root, name)
        try:
            manifest = read_manifest(directory)
        except RunArtifactsInvalid as exc:
            has_manifest = os.path.exists(manifest_path(directory))
            out.append({"run_id": name, "path": directory, "has_manifest": has_manifest, "refused": str(exc)})
            continue
        entry = manifest.summary()
        entry.update({"path": directory, "has_manifest": True, "refused": None})
        out.append(entry)
    return out


def newest_run_id(artifacts_dir: str) -> Optional[str]:
    """The most recent run whose manifest reads as one, or None.

    A run the manifest reader refused is skipped, not picked: a reload with no run named
    asks for the newest run that can be served, and an unloadable one would only turn
    the request into a refusal of something the operator never asked for.
    """
    for entry in list_runs(artifacts_dir):
        if entry.get("has_manifest") and entry.get("refused") is None:
            return str(entry["run_id"])
    return None


def read_current(artifacts_dir: str) -> Optional[str]:
    """The run id ``current`` points at, or None when nothing has been published."""
    path = os.path.join(artifacts_dir, CURRENT_POINTER_NAME)
    try:
        with open(path, encoding="utf-8") as fh:
            raw = json.load(fh)
    except FileNotFoundError:
        return None
    except (json.JSONDecodeError, OSError) as exc:
        logger.warning("runs.current.unreadable path=%s error=%s", path, exc)
        return None
    run_id = raw.get("run_id") if isinstance(raw, dict) else None
    return str(run_id) if run_id else None


def set_current(artifacts_dir: str, run_id: str) -> str:
    """Point ``current`` at a run. Refuses a run that is not one."""
    directory = run_dir(artifacts_dir, run_id)
    read_manifest(directory)  # raises RunArtifactsInvalid if this is not a run
    os.makedirs(artifacts_dir, exist_ok=True)
    path = os.path.join(artifacts_dir, CURRENT_POINTER_NAME)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump({"run_id": run_id, "updated_at": datetime.now(timezone.utc).isoformat()}, fh, indent=2)
    logger.info("runs.current.set run_id=%s path=%s", run_id, path)
    return path
