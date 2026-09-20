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

from typing import List, Optional

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


def _digest(matches) -> str:
    """The dataset sha a retrain would record for this history."""
    sha, _ = build(ListSource(matches)).dataset_digest()
    return sha


def _decided_match_keys(matches) -> List[str]:
    """The digest as it was before EVAL-12: ``match_id|match_date`` over the decided
    matches. Spelled out here rather than imported because the point of every test below
    is that this is not enough, and the assertion has to show what "not enough" was."""
    frame = build(ListSource(matches)).frame
    return sorted(f"{row.match_id}|{row.match_date}" for row in frame.itertuples(index=False))


# --- the commit ------------------------------------------------------------------------


def test_the_commit_is_unknown_rather_than_blank_where_no_checkout_can_be_asked(monkeypatch) -> None:
    """The serving image's case: no version-control binary and no repository directory, so
    the checkout cannot answer. The manifest must say so in a word, not with a blank."""
    monkeypatch.setattr(runs, "_version_control_output", lambda *args: None)
    monkeypatch.delenv(runs.GIT_SHA_ENV, raising=False)

    assert runs.git_sha() == runs.UNKNOWN
    assert runs.git_sha() != "", "an empty commit reads as an absent field, which is the defect"


def test_the_commit_comes_from_the_environment_where_there_is_no_checkout(monkeypatch) -> None:
    """What the Dockerfile's build argument buys: the image has no checkout, but the build
    knew the commit and baked it in, so the run records it."""
    monkeypatch.setattr(runs, "_version_control_output", lambda *args: None)
    monkeypatch.setenv(runs.GIT_SHA_ENV, "  0123456789abcdef  ")

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
    monkeypatch.setattr(runs, "_version_control_output", lambda *args: " M ml/xi/runs.py" if args[0] == "status" else "beefcafe")

    assert runs.git_sha() == f"beefcafe{runs.DIRTY_SUFFIX}"


def test_the_library_versions_name_the_interpreter_and_the_estimators() -> None:
    """Pinning the commit without pinning these does not reproduce a run: a scikit-learn
    release is free to change a splitter's tie-breaking."""
    versions = runs.library_versions()

    assert set(versions) == {"python", *runs.RECORDED_LIBRARIES}
    assert all(value for value in versions.values()), "an unresolved version records the word, never a blank"


# --- the dataset digest ----------------------------------------------------------------


def test_a_re_imported_squad_is_invisible_to_the_decided_match_list_and_not_to_the_digest() -> None:
    """The defect, exactly: an import that swaps a player into an XI leaves every decided
    match's id and date untouched, so the old digest could not tell the two apart."""
    before = [_match(k) for k in range(6)]
    after = [_match(k) for k in range(6)]
    after[3] = _match(3, team_one=[*TEAM_ONE[:10], "substitute"])

    assert _decided_match_keys(before) == _decided_match_keys(after), "the fixture list is identical -- that is the point"
    assert _digest(before) != _digest(after)


def test_a_re_imported_delivery_is_invisible_to_the_decided_match_list_and_not_to_the_digest() -> None:
    """The same for the ball events: the same fixtures, the same XIs, different cricket."""
    before = [_match(k) for k in range(6)]
    after = [_match(k) for k in range(6)]
    after[2] = _match(2, runs_per_ball=5)

    assert _decided_match_keys(before) == _decided_match_keys(after)
    assert _digest(before) != _digest(after)


def test_a_re_imported_undecided_match_changes_the_digest() -> None:
    """An undecided match produces no training row at all, yet still folds into the
    ratings every prediction is made from. The pass-level counts are what covers it."""
    before = [*[_match(k) for k in range(5)], _match(5, winner=None)]
    after = [*[_match(k) for k in range(5)], _match(5, winner=None, runs_per_ball=4)]

    assert _decided_match_keys(before) == _decided_match_keys(after)
    assert _digest(before) != _digest(after)


def test_the_same_cricket_digests_the_same_twice() -> None:
    """The other half of the property: the digest must not move on its own, or it answers
    "different data" every time it is asked."""
    assert _digest([_match(k) for k in range(6)]) == _digest([_match(k) for k in range(6)])


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
