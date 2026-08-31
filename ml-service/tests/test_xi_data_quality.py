"""The data-quality gate and the source-parity check (H-15).

Both defects this gate exists for were found by a person comparing two numbers by hand:
the database held 22,425 matches for 22,734 files, and the JSON source keyed 469 unnamed
substitute fielders on the empty name. Neither would have survived a run that printed them.
"""

from __future__ import annotations

import json
from dataclasses import replace
from datetime import date, timedelta

import numpy as np
import pytest

from ml.xi import quality
from ml.xi.builder import build
from ml.xi.quality import DataQuality
from ml.xi.sources import Deliveries, MatchRecord, SourceCounts


def _clean() -> DataQuality:
    """A pass whose counts all add up: the shape everything else deviates from."""
    return DataQuality(
        source="PostgresSource",
        offered_matches=100,
        out_of_scope_matches=10,
        unusable_matches=2,
        matches_read=88,
        undecided_matches=5,
        namesake_sides=2,
        oversized_squads=7,
        unknown_player_keys=0,
        player_keys=500,
    )


# ---------------------------------------------------------------------------
# The gate
# ---------------------------------------------------------------------------


def test_a_first_run_passes_and_records_a_baseline() -> None:
    result = quality.check(_clean(), previous=None)

    assert result.passed
    assert result.baseline == _clean().as_dict()


def test_matches_that_are_dropped_without_a_reason_fail_the_gate() -> None:
    """The §10.4 shape: 309 matches went missing because 618 files shared an id, and
    nothing in the pipeline noticed that the counts no longer added up."""
    unaccounted = replace(_clean(), matches_read=79)

    result = quality.check(unaccounted, previous=None)

    assert not result.passed
    assert "unaccounted for" in result.failures[0]
    assert "9 of 100" in result.failures[0]


def test_a_source_that_yields_nothing_fails_the_gate() -> None:
    empty = DataQuality(source="PostgresSource")

    result = quality.check(empty, previous=None)

    assert "the source yielded no matches" in result.failures


def test_a_count_that_was_zero_and_is_not_any_more_fails() -> None:
    """The #224 shape: unresolved player keys had always been zero. One appearing is the
    fallback path firing, which is the path that goes back to identifying people by name."""
    regressed = replace(_clean(), unknown_player_keys=1)

    result = quality.check(regressed, previous=_clean().as_dict())

    assert result.failures == ["unknown_player_keys was 0 and is now 1"]


def test_a_count_that_more_than_doubles_fails() -> None:
    regressed = replace(_clean(), namesake_sides=5)

    result = quality.check(regressed, previous=_clean().as_dict())

    assert result.failures == ["namesake_sides more than doubled: 2 -> 5"]


def test_a_count_that_grows_without_doubling_passes() -> None:
    """Data changes. The gate is for a step change, not for drift."""
    grown = replace(_clean(), namesake_sides=4, oversized_squads=13)

    result = quality.check(grown, previous=_clean().as_dict())

    assert result.passed


def test_more_matches_never_fail_the_gate() -> None:
    """The dataset grows every week; only the ways it is broken are gated."""
    bigger = replace(_clean(), offered_matches=400, matches_read=388, player_keys=2000)

    result = quality.check(bigger, previous=_clean().as_dict())

    assert result.passed


def test_every_broken_count_is_reported_not_just_the_first() -> None:
    regressed = replace(_clean(), namesake_sides=99, unknown_player_keys=3)

    result = quality.check(regressed, previous=_clean().as_dict())

    assert len(result.failures) == 2


# ---------------------------------------------------------------------------
# The baseline
# ---------------------------------------------------------------------------


def test_the_baseline_round_trips(tmp_path) -> None:
    path = quality.save_baseline(str(tmp_path), _clean())

    assert quality.load_baseline(str(tmp_path)) == _clean().as_dict()
    assert path.endswith(quality.BASELINE_NAME)


def test_a_missing_baseline_is_no_baseline(tmp_path) -> None:
    assert quality.load_baseline(str(tmp_path)) is None


def test_an_unreadable_baseline_is_no_baseline(tmp_path) -> None:
    """A corrupt baseline must not refuse the retrain; it must say there is nothing to
    compare against and let the run record a new one."""
    (tmp_path / quality.BASELINE_NAME).write_text("{not json")

    assert quality.load_baseline(str(tmp_path)) is None


def test_a_baseline_that_is_not_an_object_is_no_baseline(tmp_path) -> None:
    (tmp_path / quality.BASELINE_NAME).write_text("[1, 2, 3]")

    assert quality.load_baseline(str(tmp_path)) is None


# ---------------------------------------------------------------------------
# What the rating pass counts
# ---------------------------------------------------------------------------


def _match(match_id: str, day: int, team1, team2, winner="A") -> MatchRecord:
    z = np.zeros(1)
    deliveries = Deliveries(
        over=np.zeros(1, dtype=int),
        innings=np.zeros(1, dtype=int),
        batter=np.asarray([team1[0]], dtype=object),
        bowler=np.asarray([team2[0]], dtype=object),
        runs_batter=z,
        runs_total=z,
        wicket=z,
        bowler_wicket=z,
        stumping=z,
        fielders=[[]],
    )
    return MatchRecord(
        match_id,
        date(2024, 1, 1) + timedelta(days=day),
        "T20",
        "A",
        "B",
        "V",
        "male",
        list(team1),
        list(team2),
        winner,
        None,
        deliveries,
    )


class _CountingSource:
    def __init__(self, matches, counts: SourceCounts):
        self.matches = matches
        self.counts = counts

    def iter_matches(self):
        yield from self.matches


def test_the_pass_reports_what_the_source_dropped() -> None:
    matches = [_match("m0", 0, ["a1"], ["b1"])]
    counts = SourceCounts(offered=10, out_of_scope=6, unusable=3, yielded=1)

    result = build(_CountingSource(matches, counts))

    assert result.quality.offered_matches == 10
    assert result.quality.out_of_scope_matches == 6
    assert result.quality.unusable_matches == 3
    assert result.quality.matches_read == 1
    assert quality.check(result.quality).passed


def test_the_pass_counts_a_person_named_on_both_sides() -> None:
    """One person cannot play for both teams: a shared key is two people the source cannot
    tell apart, and both sides would be handed the same ratings."""
    matches = [_match("m0", 0, ["a1", "shared"], ["b1", "shared"])]

    result = build(_CountingSource(matches, SourceCounts(offered=1, yielded=1)))

    assert result.quality.namesake_sides == 2


def test_the_pass_counts_sides_of_more_than_eleven() -> None:
    """Concussion and injury replacements, which Cricsheet lists in full."""
    twelve = [f"a{i}" for i in range(12)]
    eleven = [f"b{i}" for i in range(11)]
    matches = [_match("m0", 0, twelve, eleven)]

    result = build(_CountingSource(matches, SourceCounts(offered=1, yielded=1)))

    assert result.quality.oversized_squads == 1


def test_the_pass_counts_player_keys_the_source_could_not_resolve() -> None:
    matches = [_match("m0", 0, ["a1", "name:Someone"], ["b1"])]

    result = build(_CountingSource(matches, SourceCounts(offered=1, yielded=1)))

    assert result.quality.unknown_player_keys == 1


def test_a_source_that_reports_nothing_is_described_by_what_arrived() -> None:
    """Zeros would make the accounting identity hold vacuously, which is the opposite of
    what it is for."""

    class _SilentSource:
        def iter_matches(self):
            yield _match("m0", 0, ["a1"], ["b1"])

    result = build(_SilentSource())

    assert result.quality.offered_matches == 1
    assert result.quality.matches_read == 1
    assert quality.check(result.quality).passed


def test_an_empty_source_fails_the_gate_rather_than_reporting_a_clean_pass() -> None:
    class _EmptySource:
        def iter_matches(self):
            return iter(())

    result = build(_EmptySource())

    assert not quality.check(result.quality).passed


# ---------------------------------------------------------------------------
# Source parity
# ---------------------------------------------------------------------------


@pytest.fixture
def two_passes():
    matches = [_match(f"m{i}", i, ["a1", "a2"], ["b1", "b2"]) for i in range(3)]
    counts = SourceCounts(offered=3, yielded=3)
    return (
        build(_CountingSource(matches, counts)),
        build(_CountingSource(list(matches), SourceCounts(offered=3, yielded=3))),
    )


def test_parity_reports_nothing_when_the_two_sources_agree(two_passes) -> None:
    from ml.xi.parity import compare

    postgres, cricsheet = two_passes

    assert compare(postgres, cricsheet) == []


def test_parity_reports_a_match_the_database_is_missing(two_passes) -> None:
    """The §10.4 shape, seen from the other side: the archive has matches the database
    does not."""
    from ml.xi.parity import compare

    postgres, _ = two_passes
    bigger = [_match(f"m{i}", i, ["a1", "a2"], ["b1", "b2"]) for i in range(4)]
    cricsheet = build(_CountingSource(bigger, SourceCounts(offered=4, yielded=4)))

    differences = compare(postgres, cricsheet)

    assert any("matches_read: postgres 3, cricsheet 4" in d for d in differences)
    assert any("training rows" in d for d in differences)


def test_parity_reports_a_player_key_only_one_source_has(two_passes) -> None:
    """The #224 shape: the JSON path carried a fictional cricketer the database did not."""
    from ml.xi.parity import compare

    postgres, _ = two_passes
    phantom = [_match(f"m{i}", i, ["a1", "a2"], ["b1", "b2", "name:"]) for i in range(3)]
    cricsheet = build(_CountingSource(phantom, SourceCounts(offered=3, yielded=3)))

    differences = compare(postgres, cricsheet)

    assert any("only in cricsheet ['name:']" in d for d in differences)


def test_the_parity_table_puts_both_sources_beside_each_other(two_passes) -> None:
    from ml.xi.parity import counts_table

    postgres, cricsheet = two_passes

    table = counts_table(postgres, cricsheet)

    assert "postgres" in table and "cricsheet" in table
    assert "matches_read" in table


def test_the_parity_summary_is_serialisable(two_passes) -> None:
    from ml.xi.parity import summary

    postgres, cricsheet = two_passes

    assert json.loads(json.dumps(summary(postgres, cricsheet)))["differences"] == []


# ---------------------------------------------------------------------------
# The gate through the CLI
# ---------------------------------------------------------------------------


def _tiny_cricsheet_dir(tmp_path, n_files: int = 2):
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    src = tmp_path / "json"
    src.mkdir()
    players = {"X": [f"X{i}" for i in range(11)], "Y": [f"Y{i}" for i in range(11)]}
    for i in range(n_files):
        winner = "X" if i % 2 == 0 else "Y"
        (src / f"{i}.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], players, winner, i + 1)))
    return src


def test_a_clean_run_records_a_baseline_beside_the_artifacts(tmp_path) -> None:
    from ml.xi.train import main as train_main

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"

    rc = train_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert rc == 0
    baseline = json.loads((out / quality.BASELINE_NAME).read_text())
    assert baseline["matches_read"] == 2
    assert baseline["source"] == "CricsheetJsonSource"


def test_a_run_whose_counts_regress_fails_and_leaves_the_baseline_alone(tmp_path) -> None:
    """Re-running must not clear the gate: a failing run that recorded its own counts as
    the baseline would pass the second time, which is not a gate."""
    from ml.xi.train import main as train_main
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"
    out.mkdir()
    (out / quality.BASELINE_NAME).write_text(json.dumps({**_clean().as_dict(), "oversized_squads": 0}))
    # A side of twelve, which the accepted run had none of.
    twelve = {"X": [f"X{i}" for i in range(12)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "9.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], twelve, "X", 9)))

    first = train_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])
    second = train_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert first == 1
    assert second == 1, "a re-run fails the same way; only --accept-data-quality moves the baseline"
    assert json.loads((out / quality.BASELINE_NAME).read_text())["oversized_squads"] == 0


def test_the_report_records_the_counts_and_the_failures_even_when_the_gate_fails(tmp_path) -> None:
    """The artifacts and the report are written either way: an operator has to see what the
    run produced in order to judge whether the new counts are right."""
    from ml.xi.train import REPORT_NAME
    from ml.xi.train import main as train_main

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"
    out.mkdir()
    (out / quality.BASELINE_NAME).write_text(json.dumps({**_clean().as_dict(), "oversized_squads": 0}))
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    twelve = {"X": [f"X{i}" for i in range(12)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "9.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], twelve, "X", 9)))

    rc = train_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert rc == 1
    report = json.loads((out / REPORT_NAME).read_text())
    assert report[quality.REPORT_KEY]["oversized_squads"] == 1
    assert report["data_quality_failures"] == ["oversized_squads was 0 and is now 1"]
    assert (out / "xi_ratings.joblib").exists()


def test_accepting_the_new_counts_moves_the_baseline(tmp_path) -> None:
    from ml.xi.train import main as train_main

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"
    out.mkdir()
    (out / quality.BASELINE_NAME).write_text(json.dumps({**_clean().as_dict(), "oversized_squads": 0}))
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    twelve = {"X": [f"X{i}" for i in range(12)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "9.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], twelve, "X", 9)))

    rc = train_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out), "--accept-data-quality"])

    assert rc == 0
    assert json.loads((out / quality.BASELINE_NAME).read_text())["oversized_squads"] == 1
