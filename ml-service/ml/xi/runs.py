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
import platform
import subprocess
import uuid
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from importlib import metadata
from typing import Any, Dict, Iterable, List, Optional

logger = logging.getLogger(__name__)

#: Directory under the artifacts root holding one subdirectory per run.
RUNS_DIRNAME = "runs"

#: The file that makes a directory a run.
MANIFEST_NAME = "manifest.json"

#: The pointer file naming the run being served. A file rather than a symlink so the
#: same code works on every filesystem the service is deployed to, and so the answer to
#: "which run is current?" is readable without following anything.
CURRENT_POINTER_NAME = "current_run.json"

#: What a provenance field records when its answer cannot be established. The empty string
#: it replaces rendered as a blank on every surface that reads a manifest, which is what
#: "this manifest does not carry the field" also renders as -- so a run built where no
#: commit could be read was indistinguishable from one built before the field existed
#: (EVAL-12, §8.7). A word that is not a sha says which of the two it is, and says it in
#: the one place the answer is read.
UNKNOWN = "unknown"

#: Appended to the commit when the working tree the run was built from carries uncommitted
#: changes. The commit alone does not then name the code that ran, and quoting it as if it
#: did is the same defect as quoting "" as if it were an answer.
DIRTY_SUFFIX = "-dirty"

#: Environment variable naming the commit, for an environment that has no repository to
#: ask. The serving image is exactly that: ``ml-service/Dockerfile`` copies the sources in
#: and carries neither the version-control binary nor the repository directory, so every
#: run built through ``/admin/train/*`` recorded ``git_sha=""`` until the Dockerfile began
#: baking this in as a build argument (EVAL-12).
GIT_SHA_ENV = "GIT_SHA"

#: The libraries whose version decides what a fit produces: the estimators, the array and
#: frame semantics they run on, and the serialiser the artifacts are written with. Pinning
#: the code without pinning these does not reproduce a run -- a scikit-learn minor release
#: is free to change a splitter's tie-breaking, and the artifact would differ with the same
#: commit and the same data.
RECORDED_LIBRARIES = ("scikit-learn", "numpy", "scipy", "pandas", "joblib")

#: What ``dataset_sha`` is a digest of, named and numbered so two runs' digests are only
#: ever compared when they were computed the same way. Before EVAL-12 the digest read the
#: id and date of each decided match and nothing else, so a re-import that rewrote every
#: squad and every delivery produced a byte-identical sha.
DATASET_DIGEST_SCHEME = "matches+xi-outcomes+pass-counts/1"

#: The scheme a run that never walked a source records instead. The block is *present*, so
#: such a run is not mistaken for one written before the digest existed and refused for it;
#: the scheme says there was no dataset to digest. The H-8 round trip
#: (``ml.xi.asof.round_trip_store``) is the case: it writes models it already holds in
#: memory, and the caller -- not the run -- holds the source. §8.7 again: "there was no
#: dataset" and "this field did not exist yet" are different answers and must not render
#: as the same blank.
NO_DATASET_SCHEME = "no-dataset"


def no_dataset_digest(reason: str) -> Dict[str, Any]:
    """The ``dataset_digest`` block for a run built without reading a source."""
    return {"scheme": NO_DATASET_SCHEME, "reason": reason}


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

    The provenance half -- ``git_sha``, ``source``, ``dataset_digest``,
    ``library_versions``, ``model_params``, ``performance_spec`` -- is what EVAL-12 added,
    and it answers one question the rest cannot: could this run be built again? The
    ``hyperparameters`` block is, per format, each win model's grid *choice* -- the
    display model's (EVAL-06) and the objective's ``C`` and recency half-life (EVAL-13) --
    and stays the only record of it; ``model_params`` carries the constants neither grid
    varies, which lived only as literals in ``ml.xi.train``.
    """

    run_id: str
    created_at: str
    cutoff: str
    ratings_through: str
    dataset_sha: str
    git_sha: str
    #: What the digest covered, so two ``dataset_sha`` values are only compared when they
    #: were computed the same way. Required: a manifest without it was written when the
    #: digest could not see a squad or a delivery, so its sha answers a different question
    #: from this one's and ``read_manifest`` refuses the run by name.
    dataset_digest: Dict[str, Any] = field(default_factory=dict)
    #: The rating pass's source class -- ``PostgresSource`` or ``CricsheetJsonSource``.
    #: The two read the same cricket by different routes and have differed before (FEAT-04,
    #: IMPORT-05/06), so which one a run was built from is part of building it again.
    source: str = ""
    #: Library name -> version for the packages a fit's output depends on
    #: (``RECORDED_LIBRARIES``), plus ``python``.
    library_versions: Dict[str, str] = field(default_factory=dict)
    #: The win models' fixed constants (``ml.xi.train.win_model_params``): the objective's
    #: regularisation and the display model's settings the grid does not choose.
    model_params: Dict[str, Any] = field(default_factory=dict)
    #: format code -> the performance model's ``FitSpec`` for that format, quoted from the
    #: run's own report so the two cannot fall out of step.
    performance_spec: Dict[str, Any] = field(default_factory=dict)
    rating_params: Dict[str, Any] = field(default_factory=dict)
    hyperparameters: Dict[str, Any] = field(default_factory=dict)
    metrics: Dict[str, Any] = field(default_factory=dict)
    state_shape: Dict[str, Any] = field(default_factory=dict)
    formats: List[str] = field(default_factory=list)
    #: format code -> why it has no headline metrics. Empty when every format was scored.
    format_notes: Dict[str, str] = field(default_factory=dict)
    report: str = ""
    #: The usability verdict (EVAL-04, ``ml.xi.run_usability``): False when a scored
    #: format's objective does not rank, or fell under the served run's on the same holdout
    #: by more than the AUC's own standard error. A run that is not usable is written so
    #: the operator can read why, and ``set_current`` refuses to publish it.
    usable: bool = True
    #: format code -> why the run is not usable there. Empty when it is usable.
    unusable_reasons: Dict[str, str] = field(default_factory=dict)

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
            "usable": self.usable,
            "unusable_reasons": dict(self.unusable_reasons),
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


def _version_control_output(*args: str) -> Optional[str]:
    """Run a read-only version-control command in this file's directory, or None if the
    tool is absent or the directory is not a checkout. Absence is normal -- the serving
    image has neither -- so it is logged at debug and answered by the environment instead."""
    try:
        out = subprocess.run(
            ["git", *args],
            cwd=os.path.dirname(os.path.abspath(__file__)),
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        logger.debug("runs.git.absent args=%s error=%s", args, exc)
        return None
    if out.returncode != 0:
        logger.debug("runs.git.failed args=%s stderr=%s", args, out.stderr.strip())
        return None
    return out.stdout.strip()


def git_sha() -> str:
    """The commit the running code came from, or ``UNKNOWN`` when nothing can say.

    Three answers are tried in the order of how much they know. The checkout is asked
    first, because it cannot be stale: it reports the commit this very file is at, and
    appends ``DIRTY_SUFFIX`` when the tree has uncommitted changes, since a clean sha over
    a dirty tree names code that was never committed. The ``GIT_SHA`` environment variable
    is asked second, for an environment with no checkout to ask -- the serving image, where
    the Dockerfile bakes the building commit in as a build argument. Failing both, the
    answer is the word ``UNKNOWN`` and a warning, never the empty string: an empty field is
    read as an absent one, and a run whose commit is unrecorded has to say so where it is
    read rather than only in the log that scrolled past (§8.7).
    """
    sha = _version_control_output("rev-parse", "HEAD")
    if sha:
        dirty = _version_control_output("status", "--porcelain")
        suffix = DIRTY_SUFFIX if dirty else ""
        if dirty:
            logger.warning(
                "runs.git_sha.dirty sha=%s -- the tree this run was built from has uncommitted changes, "
                "so the commit alone does not name the code that ran",
                sha,
            )
        return f"{sha}{suffix}"
    from_environment = os.environ.get(GIT_SHA_ENV, "").strip()
    if from_environment:
        logger.info("runs.git_sha.from_environment %s=%s", GIT_SHA_ENV, from_environment)
        return from_environment
    logger.warning(
        "runs.git_sha.unknown -- no checkout to ask and %s is unset, so this run records its commit as %r. "
        "Build the image with --build-arg %s=$(git rev-parse HEAD), or set %s in the environment.",
        GIT_SHA_ENV,
        UNKNOWN,
        GIT_SHA_ENV,
        GIT_SHA_ENV,
    )
    return UNKNOWN


def library_versions() -> Dict[str, str]:
    """The interpreter and the libraries a fit's output depends on (``RECORDED_LIBRARIES``).

    A library that is not installed records ``UNKNOWN`` rather than being left out, for the
    same reason ``git_sha`` does: a key missing from the block reads as "this manifest is
    old", while a key present and unknown reads as "this run could not tell you".
    """
    versions: Dict[str, str] = {"python": platform.python_version()}
    for name in RECORDED_LIBRARIES:
        try:
            versions[name] = metadata.version(name)
        except metadata.PackageNotFoundError:
            logger.warning("runs.library_versions.missing package=%s", name)
            versions[name] = UNKNOWN
    return versions


def dataset_sha(entries: Iterable[str]) -> str:
    """A digest of the cricket the rating pass consumed, over the lines ``entries`` yields.

    It is computed from what was read rather than copied from the archive's checksum
    because ml-service does not mount the dataset directory: the question it has to be
    able to answer is "were these two runs trained on the same cricket?", on either source.

    Order-independent by construction -- the lines are sorted before hashing -- because the
    answer must not depend on the order the pass happened to walk the matches in, and the
    count is folded in so that a line repeated is not a line absent. What goes into the
    lines is ``ml.xi.retrain.dataset_digest``'s decision; this only hashes them.
    """
    lines = sorted(entries)
    digest = hashlib.sha256()
    digest.update(f"scheme={DATASET_DIGEST_SCHEME}\n".encode("utf-8"))
    for line in lines:
        digest.update(line.encode("utf-8"))
        digest.update(b"\n")
    digest.update(f"n={len(lines)}".encode("utf-8"))
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
    if not raw.get("dataset_digest"):
        # Written before EVAL-12. Its dataset_sha is a digest of the id and date of each
        # decided match and nothing else, so it reads the same over a re-import that
        # rewrote every squad and every delivery -- it answers a different question from
        # this code's sha, and comparing the two would answer neither. Its git_sha is "" on
        # any run built in the serving image, which has no checkout to ask. Nothing is
        # backfilled (§10.5, and `ratings_through` above): an older run is retrained.
        raise RunArtifactsInvalid(
            f"run {raw['run_id']}: {MANIFEST_NAME} carries no dataset_digest, so nothing says what its "
            f"dataset_sha is a digest of; it was written before the digest could see a squad or a delivery "
            f"(EVAL-12) and cannot be loaded. Run `make retrain` to produce a run that records its provenance."
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
    """The most recent run whose manifest reads as one and which may be published, or None.

    A run the manifest reader refused is skipped, not picked, and so is one its own
    retrain judged not usable (EVAL-04): a reload with no run named asks for the newest
    run that can be served, and either would only turn the request into a refusal of
    something the operator never asked for.
    """
    for entry in list_runs(artifacts_dir):
        if entry.get("has_manifest") and entry.get("refused") is None and entry.get("usable", True):
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
    """Point ``current`` at a run. Refuses a run that is not one, and a run its own
    retrain judged not usable (EVAL-04): publishing is this pointer, so this is the one
    place the refusal has to live for ``reload`` -- named or newest -- to honour it."""
    directory = run_dir(artifacts_dir, run_id)
    manifest = read_manifest(directory)  # raises RunArtifactsInvalid if this is not a run
    if not manifest.usable:
        reasons = "; ".join(f"{fmt}: {why}" for fmt, why in sorted(manifest.unusable_reasons.items()))
        raise RunArtifactsInvalid(
            f"run {run_id} is not usable and cannot be published -- {reasons}. It stays on disk to be read; "
            f"run `make retrain` to replace it"
        )
    os.makedirs(artifacts_dir, exist_ok=True)
    path = os.path.join(artifacts_dir, CURRENT_POINTER_NAME)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump({"run_id": run_id, "updated_at": datetime.now(timezone.utc).isoformat()}, fh, indent=2)
    logger.info("runs.current.set run_id=%s path=%s", run_id, path)
    return path
