"""What a run has to record to be reproducible (EVAL-12).

Two of the manifest's provenance fields answered nothing. ``git_sha`` shelled out to the
version-control binary, which the serving image does not carry -- no binary, no repository
directory -- so every run built through ``/admin/train/*`` recorded an empty string, and an
empty string on a surface is indistinguishable from a field that is not there. And
``dataset_sha`` digested the id and date of each *decided* match and nothing else, so the
one thing this repository does to its data routinely -- re-import the archive -- could
rewrite every squad and every delivery and leave the sha byte-identical.

These tests pin both: the commit says which of "not recorded" and "could not be
established" it is, and the digest sees the cricket rather than the fixture list.
"""

from __future__ import annotations

from types import SimpleNamespace
from typing import List, Optional

import pandas as pd

from ml.xi import builder as builder_module
from ml.xi import contract as C
from ml.xi import retrain as retrain_module
from ml.xi import runs
from ml.xi.builder import build
from tests.xi_fixtures import ListSource, make_deliveries, make_match, xi

TEAM_ONE, TEAM_TWO = xi("a"), xi("b")


def _match(
    index: int,
    team_one: Optional[List[str]] = None,
    team_two: Optional[List[str]] = None,
    runs_per_ball: int = 2,
    winner: Optional[str] = "A",
):
    """One synthetic T20, with the levers a re-import can move: who was in each XI, what
    the deliveries scored, and whether anybody won."""
    team_one = list(TEAM_ONE if team_one is None else team_one)
    team_two = list(TEAM_TWO if team_two is None else team_two)
    n_balls = 12
    batters = [team_one[i % 6] for i in range(n_balls)] + [team_two[i % 6] for i in range(n_balls)]
    bowlers = [team_two[5 + i % 5] for i in range(n_balls)] + [team_one[5 + i % 5] for i in range(n_balls)]
    deliveries = make_deliveries(
        batters=batters,
        bowlers=bowlers,
        runs=[runs_per_ball] * (2 * n_balls),
        wickets=[0] * (2 * n_balls),
        innings=[0] * n_balls + [1] * n_balls,
    )
    return make_match(f"m{index}", index, winner, team_one, team_two, deliveries)


def _dataset_sha(matches, directory) -> str:
    """The dataset sha a retrain actually writes for this history, read back off the
    manifest. Read through the pipeline rather than off the digest helper so the question
    is the one an operator asks -- "do these two runs say they saw the same cricket?" --
    and not "is this function implemented"."""
    written = retrain_module.retrain(
        build(ListSource(matches)), str(directory), pd.Timestamp("2024-06-01"), formats=["T20"]
    )
    return written["manifest"]["dataset_sha"]


def _decided_match_keys(matches) -> List[str]:
    """The digest as it was before EVAL-12: ``match_id|match_date`` over the decided
    matches. Spelled out here rather than imported because the point of every test below
    is that this is not enough, and the assertion has to show what "not enough" was."""
    frame = build(ListSource(matches)).frame
    return sorted(f"{row.match_id}|{row.match_date}" for row in frame.itertuples(index=False))


# --- the commit ------------------------------------------------------------------------


def test_the_commit_is_unknown_rather_than_blank_where_no_checkout_can_be_asked(monkeypatch) -> None:
    """The serving image's case, patched at the boundary the image actually fails at: the
    version-control binary is not installed, so the call raises before it can answer.

    The environment variable is spelled out rather than read off the module so that this
    test asks the same question of any version of ``git_sha``: does a run built where no
    commit can be established record the fact, or a blank that every surface renders
    exactly as "this manifest does not carry the field"?
    """

    def no_version_control(*args, **kwargs):
        raise FileNotFoundError(2, "No such file or directory", "git")

    monkeypatch.setattr(runs.subprocess, "run", no_version_control)
    monkeypatch.delenv("GIT_SHA", raising=False)

    recorded = runs.git_sha()

    assert recorded != "", "an empty commit is read as an absent field, which is the defect"
    assert recorded == runs.UNKNOWN


def test_the_commit_comes_from_the_environment_where_there_is_no_checkout(monkeypatch) -> None:
    """What the Dockerfile's build argument buys: the image has no checkout, but the build
    knew the commit and baked it in, so the run records it."""

    def no_version_control(*args, **kwargs):
        raise FileNotFoundError(2, "No such file or directory", "git")

    monkeypatch.setattr(runs.subprocess, "run", no_version_control)
    monkeypatch.setenv("GIT_SHA", "  0123456789abcdef  ")

    assert runs.git_sha() == "0123456789abcdef"


def test_the_checkout_outranks_the_environment(monkeypatch) -> None:
    """Outside the image the checkout is asked first: it reports where this file actually
    is, and cannot be left stale by an exported variable."""
    monkeypatch.setattr(runs, "_version_control_output", lambda *args: "" if args[0] == "status" else "beefcafe")
    monkeypatch.setenv(runs.GIT_SHA_ENV, "0123456789abcdef")

    assert runs.git_sha() == "beefcafe"


def test_a_commit_over_uncommitted_changes_says_so(monkeypatch) -> None:
    """A clean sha over a dirty tree names code that was never committed, which is the
    same false confidence as quoting an empty string."""
    monkeypatch.setattr(
        runs, "_version_control_output", lambda *args: " M ml/xi/runs.py" if args[0] == "status" else "beefcafe"
    )

    assert runs.git_sha() == f"beefcafe{runs.DIRTY_SUFFIX}"


def test_the_library_versions_name_the_interpreter_and_the_estimators() -> None:
    """Pinning the commit without pinning these does not reproduce a run: a scikit-learn
    release is free to change a splitter's tie-breaking."""
    versions = runs.library_versions()

    assert set(versions) == {"python", *runs.RECORDED_LIBRARIES}
    assert all(value for value in versions.values()), "an unresolved version records the word, never a blank"


def test_asking_a_checkout_that_is_not_there_answers_nothing_rather_than_raising(monkeypatch) -> None:
    """The serving image's two failure modes, at the level they actually happen: the
    binary is missing (the image), or it runs and reports that this is no checkout."""

    def missing(*args, **kwargs):
        raise FileNotFoundError(2, "No such file or directory", "git")

    monkeypatch.setattr(runs.subprocess, "run", missing)
    assert runs._version_control_output("rev-parse", "HEAD") is None

    monkeypatch.setattr(
        runs.subprocess,
        "run",
        lambda *args, **kwargs: SimpleNamespace(returncode=128, stdout="", stderr="not a repository"),
    )
    assert runs._version_control_output("rev-parse", "HEAD") is None


def test_a_library_that_is_not_installed_records_the_word_not_a_blank(monkeypatch) -> None:
    """The same rule as the commit: a version that could not be established says so, and
    a reader is never left to decide whether a blank means old or unknown."""

    def absent(name: str) -> str:
        raise runs.metadata.PackageNotFoundError(name)

    monkeypatch.setattr(runs.metadata, "version", absent)

    versions = runs.library_versions()

    assert versions["numpy"] == runs.UNKNOWN
    assert versions["python"], "the interpreter is always known"


# --- the dataset digest ----------------------------------------------------------------


def test_a_re_imported_squad_is_invisible_to_the_decided_match_list_and_not_to_the_digest(tmp_path) -> None:
    """The defect, exactly: an import that swaps a player into an XI leaves every decided
    match's id and date untouched, so the old digest could not tell the two apart."""
    before = [_match(k) for k in range(6)]
    after = [_match(k) for k in range(6)]
    after[3] = _match(3, team_one=[*TEAM_ONE[:10], "substitute"])

    assert _decided_match_keys(before) == _decided_match_keys(after), (
        "the fixture list is identical -- that is the point"
    )
    assert _dataset_sha(before, tmp_path / "before") != _dataset_sha(after, tmp_path / "after")


def test_a_re_imported_delivery_is_invisible_to_the_decided_match_list_and_not_to_the_digest(tmp_path) -> None:
    """The same for the ball events: the same fixtures, the same XIs, different cricket."""
    before = [_match(k) for k in range(6)]
    after = [_match(k) for k in range(6)]
    after[2] = _match(2, runs_per_ball=5)

    assert _decided_match_keys(before) == _decided_match_keys(after)
    assert _dataset_sha(before, tmp_path / "before") != _dataset_sha(after, tmp_path / "after")


def test_a_re_imported_undecided_match_changes_the_digest(tmp_path) -> None:
    """An undecided match produces no training row at all, yet still folds into the
    ratings every prediction is made from. The pass-level counts are what covers it."""
    before = [*[_match(k) for k in range(5)], _match(5, winner=None)]
    after = [*[_match(k) for k in range(5)], _match(5, winner=None, runs_per_ball=4)]

    assert _decided_match_keys(before) == _decided_match_keys(after)
    assert _dataset_sha(before, tmp_path / "before") != _dataset_sha(after, tmp_path / "after")


def test_the_same_cricket_digests_the_same_twice(tmp_path) -> None:
    """The other half of the property: the digest must not move on its own, or it answers
    "different data" every time it is asked."""
    assert _dataset_sha([_match(k) for k in range(6)], tmp_path / "once") == _dataset_sha(
        [_match(k) for k in range(6)], tmp_path / "again"
    )


def test_the_digest_says_what_it_covered() -> None:
    """Two shas are only comparable when they were computed the same way, so the manifest
    records the scheme beside the sha rather than leaving a reader to assume."""
    result = build(ListSource([_match(k) for k in range(6)]))

    _, coverage = result.dataset_digest()

    assert coverage["scheme"] == runs.DATASET_DIGEST_SCHEME
    assert coverage["matches"] == len(result.frame)
    assert coverage["player_rows"] == len(result.player_frame)
    assert coverage["pass_counts"] == len(builder_module.DIGEST_PASS_COUNTS)


def test_the_digest_does_not_read_the_ratings_it_produced() -> None:
    """The digest is of the data, never partly of the model: the as-of feature columns are
    a function of the rating pass, so digesting them would make a run's dataset sha depend
    on the code that read it."""
    feature_columns = set(C.PLAYER_MATCH_FEATURE_COLS)

    assert not feature_columns.intersection(builder_module.DIGEST_PLAYER_COLS)
