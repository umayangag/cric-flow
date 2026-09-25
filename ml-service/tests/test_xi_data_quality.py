"""The data-quality gate and the source-parity check (H-15).

Both defects this gate exists for were found by a person comparing two numbers by hand:
the database held 22,425 matches for 22,734 files, and the JSON source keyed 469 unnamed
substitute fielders on the empty name. Neither would have survived a run that printed them.
"""

from __future__ import annotations

import json
from dataclasses import replace
from datetime import date, timedelta
from pathlib import Path

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


def _match(match_id: str, day: int, team1, team2, winner="A", result=None) -> MatchRecord:
    z = np.zeros(1)
    deliveries = Deliveries(
        over=np.zeros(1, dtype=int),
        innings=np.zeros(1, dtype=int),
        batter=np.asarray([team1[0]], dtype=object),
        bowler=np.asarray([team2[0]], dtype=object),
        runs_batter=z,
        runs_total=z,
        runs_bowler=z,
        faced=np.ones(1),
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
        result,
        deliveries,
    )


class _CountingSource:
    def __init__(self, matches, counts: SourceCounts):
        self.matches = matches
        self.counts = counts

    def iter_matches(self):
        yield from self.matches

    def birth_dates(self):
        return {}

    def venue_countries(self):
        return {}


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


def test_the_pass_counts_the_draws_and_the_ties_nobody_broke() -> None:
    """FEAT-04: the count that can see ``result``. A draw and an unbroken tie are the
    matches form reads as half a win each; a tie-breaker win is a win, and a no-result is
    undecided without being either."""
    matches = [
        _match("won", 0, ["a1"], ["b1"]),
        _match("drawn", 1, ["a1"], ["b1"], winner=None, result="draw"),
        _match("tied", 2, ["a1"], ["b1"], winner=None, result="tie"),
        _match("super-over", 3, ["a1"], ["b1"], winner="A", result="tie"),
        _match("abandoned", 4, ["a1"], ["b1"], winner=None, result="no result"),
    ]

    result = build(_CountingSource(matches, SourceCounts(offered=5, yielded=5)))

    assert result.quality.undecided_matches == 3
    assert result.quality.drawn_or_tied_matches == 2


def test_the_pass_counts_decided_matches_with_no_deliveries() -> None:
    """FEAT-03: a result recorded without ball-by-ball data. An undecided match without
    deliveries is already an undecided match; the count is for the ones whose win label
    is real and whose player rows would otherwise all have read zero."""
    matches = [
        _match("played", 0, ["a1"], ["b1"]),
        replace(_match("awarded", 1, ["a1"], ["b1"]), deliveries=Deliveries.empty()),
        replace(_match("abandoned", 2, ["a1"], ["b1"], winner=None, result="no result"), deliveries=Deliveries.empty()),
    ]

    result = build(_CountingSource(matches, SourceCounts(offered=3, yielded=3)))

    assert result.quality.decided_matches_without_deliveries == 1
    assert result.quality.undecided_matches == 1
    assert list(result.frame.match_id) == ["played", "awarded"]
    assert set(result.player_frame.match_id) == {"played"}


def test_a_decided_match_without_deliveries_appearing_fails_the_gate() -> None:
    """Zero on the current dataset from either source, which is what makes it worth
    gating: one appearing is an importer that kept a result and lost its ball events."""
    regressed = replace(_clean(), decided_matches_without_deliveries=1)

    result = quality.check(regressed, previous=_clean().as_dict())

    assert result.failures == ["decided_matches_without_deliveries was 0 and is now 1"]


def test_the_pass_counts_the_matches_played_over_more_than_one_day() -> None:
    """FEAT-09: the count that can see ``match_end_date``, so a database migrated but not
    re-imported -- every end date NULL -- is told from the archive by ``make xi-parity``."""
    from ml.xi.parity import compare

    one_day = _match("m0", 0, ["a1"], ["b1"])
    five_days = replace(_match("m1", 1, ["a1"], ["b1"]), match_end_date=date(2024, 1, 6))
    counts = SourceCounts(offered=2, yielded=2)

    with_end_dates = build(_CountingSource([one_day, five_days], counts))
    without = build(_CountingSource([one_day, replace(five_days, match_end_date=None)], counts))

    assert with_end_dates.quality.multi_day_matches == 1 and without.quality.multi_day_matches == 0
    assert any("multi_day_matches" in line for line in compare(without, with_end_dates))


def test_a_source_that_reports_nothing_is_described_by_what_arrived() -> None:
    """Zeros would make the accounting identity hold vacuously, which is the opposite of
    what it is for."""

    class _SilentSource:
        def iter_matches(self):
            yield _match("m0", 0, ["a1"], ["b1"])

        def birth_dates(self):
            return {}

        def venue_countries(self):
            return {}

    result = build(_SilentSource())

    assert result.quality.offered_matches == 1
    assert result.quality.matches_read == 1
    assert quality.check(result.quality).passed


def test_an_empty_source_fails_the_gate_rather_than_reporting_a_clean_pass() -> None:
    class _EmptySource:
        def iter_matches(self):
            return iter(())

        def birth_dates(self):
            return {}

        def venue_countries(self):
            return {}

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


def _passes_with_a_draw_and_a_tie(postgres_reads_result: bool):
    """Both sources over the same three matches -- a win, a drawn Test and a tie nobody
    broke -- with the database either reading ``result`` or, as it did before IMPORT-02,
    hard-coding it to None. Every other count is identical either way, which is how the
    difference survived: a draw is undecided whether or not its result is read."""
    archive = [
        _match("won", 0, ["a1", "a2"], ["b1", "b2"]),
        _match("drawn", 1, ["a1", "a2"], ["b1", "b2"], winner=None, result="draw"),
        _match("tied", 2, ["a1", "a2"], ["b1", "b2"], winner=None, result="tie"),
    ]
    database = [replace(m, result=m.result if postgres_reads_result else None) for m in archive]
    return (
        build(_CountingSource(database, SourceCounts(offered=3, yielded=3))),
        build(_CountingSource(archive, SourceCounts(offered=3, yielded=3))),
    )


def test_parity_reports_nothing_when_both_sources_read_a_draw_and_a_tie() -> None:
    from ml.xi.parity import compare

    postgres, cricsheet = _passes_with_a_draw_and_a_tie(postgres_reads_result=True)

    assert postgres.quality.drawn_or_tied_matches == cricsheet.quality.drawn_or_tied_matches == 2
    assert compare(postgres, cricsheet) == []


def test_parity_reports_a_result_only_one_source_reads() -> None:
    """FEAT-04, the guarantee itself: the pre-fix database agrees with the archive on
    every other count, and the parity check must still fail, naming the result count."""
    from ml.xi.parity import compare

    postgres, cricsheet = _passes_with_a_draw_and_a_tie(postgres_reads_result=False)

    differences = compare(postgres, cricsheet)

    assert postgres.quality.undecided_matches == cricsheet.quality.undecided_matches == 2
    assert differences == ["drawn_or_tied_matches: postgres 0, cricsheet 2"]


def test_parity_reports_a_match_whose_deliveries_only_one_source_holds() -> None:
    """FEAT-03, the guarantee itself: a database that kept a match's result and squads
    but lost its ball events agrees with the archive on every other count -- the win row
    is built either way -- and the parity check must still fail, naming this one."""
    from ml.xi.parity import compare

    archive = [_match(f"m{i}", i, ["a1", "a2"], ["b1", "b2"]) for i in range(3)]
    database = archive[:2] + [replace(archive[2], deliveries=Deliveries.empty())]
    postgres = build(_CountingSource(database, SourceCounts(offered=3, yielded=3)))
    cricsheet = build(_CountingSource(archive, SourceCounts(offered=3, yielded=3)))

    differences = compare(postgres, cricsheet)

    assert len(postgres.frame) == len(cricsheet.frame) == 3
    assert differences == ["decided_matches_without_deliveries: postgres 1, cricsheet 0"]


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


def test_a_clean_run_records_a_baseline_beside_the_runs(tmp_path) -> None:
    """The baseline lives at the artifacts root, not inside a run: it is the last accepted
    counts *across* runs, and one written inside a run directory would compare every run
    against nothing."""
    from ml.xi.retrain import main as retrain_main

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"

    rc = retrain_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert rc == 0
    baseline = json.loads((out / quality.BASELINE_NAME).read_text())
    assert baseline["matches_read"] == 2
    assert baseline["source"] == "CricsheetJsonSource"


def test_a_run_whose_counts_regress_fails_and_leaves_the_baseline_alone(tmp_path) -> None:
    """Re-running must not clear the gate: a failing run that recorded its own counts as
    the baseline would pass the second time, which is not a gate."""
    from ml.xi.retrain import main as retrain_main
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"
    out.mkdir()
    (out / quality.BASELINE_NAME).write_text(json.dumps({**_clean().as_dict(), "oversized_squads": 0}))
    # A side of twelve, which the accepted run had none of.
    twelve = {"X": [f"X{i}" for i in range(12)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "9.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], twelve, "X", 9)))

    first = retrain_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])
    second = retrain_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert first == 1
    assert second == 1, "a re-run fails the same way; only --accept-data-quality moves the baseline"
    assert json.loads((out / quality.BASELINE_NAME).read_text())["oversized_squads"] == 0


def test_the_report_records_the_counts_and_the_failures_even_when_the_gate_fails(tmp_path) -> None:
    """The artifacts and the report are written either way: an operator has to see what the
    run produced in order to judge whether the new counts are right."""
    from ml.xi.retrain import main as retrain_main
    from ml.xi.train import REPORT_NAME

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"
    out.mkdir()
    (out / quality.BASELINE_NAME).write_text(json.dumps({**_clean().as_dict(), "oversized_squads": 0}))
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    twelve = {"X": [f"X{i}" for i in range(12)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "9.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], twelve, "X", 9)))

    rc = retrain_main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert rc == 1
    from ml.xi import runs

    run_id = runs.newest_run_id(str(out))
    assert run_id, "the run and its manifest are written even when the gate fails"
    directory = Path(runs.run_dir(str(out), run_id))
    report = json.loads((directory / REPORT_NAME).read_text())
    assert report[quality.REPORT_KEY]["oversized_squads"] == 1
    assert report["data_quality_failures"] == ["oversized_squads was 0 and is now 1"]
    assert (directory / "xi_ratings.joblib").exists()


def test_accepting_the_new_counts_moves_the_baseline(tmp_path) -> None:
    from ml.xi.retrain import main as retrain_main

    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"
    out.mkdir()
    (out / quality.BASELINE_NAME).write_text(json.dumps({**_clean().as_dict(), "oversized_squads": 0}))
    from tests.test_xi_optimizer_and_store import _cricsheet_doc

    twelve = {"X": [f"X{i}" for i in range(12)], "Y": [f"Y{i}" for i in range(11)]}
    (src / "9.json").write_text(json.dumps(_cricsheet_doc("ODI", ["X", "Y"], twelve, "X", 9)))

    rc = retrain_main(
        ["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out), "--accept-data-quality"]
    )

    assert rc == 0
    assert json.loads((out / quality.BASELINE_NAME).read_text())["oversized_squads"] == 1


# ---------------------------------------------------------------------------
# Team identity in the Cricsheet source (I-3 and I-4)
# ---------------------------------------------------------------------------


def _doc(teams, gender: str, winner: str, day: int = 0) -> dict:
    from datetime import date as _date
    from datetime import timedelta as _timedelta

    players = {t: [f"{t[:2]}{i}" for i in range(11)] for t in teams}
    return {
        "info": {
            "dates": [(_date(2024, 3, 1) + _timedelta(days=day)).isoformat()],
            "match_type": "T20",
            "teams": teams,
            "gender": gender,
            "venue": "Ground",
            "registry": {"people": {n: f"id_{n}" for t in players.values() for n in t}},
            "players": players,
            "outcome": {"winner": winner},
        },
        "innings": [
            {
                "team": teams[0],
                "overs": [
                    {
                        "over": 0,
                        "deliveries": [
                            {
                                "batter": players[teams[0]][0],
                                "bowler": players[teams[1]][0],
                                "non_striker": players[teams[0]][1],
                                "runs": {"batter": 1, "extras": 0, "total": 1},
                            }
                        ],
                    }
                ],
            }
        ],
    }


def _parse(tmp_path, doc, lineage=None):
    from ml.xi.sources import parse_cricsheet_file

    path = tmp_path / "1.json"
    path.write_text(json.dumps(doc))
    return parse_cricsheet_file(str(path), [], lineage)


def test_the_archive_keys_a_mens_and_a_womens_side_separately(tmp_path) -> None:
    """130 of the 394 team names belong to both. One key for both gave Australia's two
    sides one Elo and one head-to-head record (I-3)."""
    mens = _parse(tmp_path, _doc(["India", "Australia"], "male", "Australia"))
    womens = _parse(tmp_path, _doc(["India", "Australia"], "female", "Australia"))

    assert mens.team1 != womens.team1
    assert mens.team1 == "India|male" and womens.team1 == "India|female"


def test_the_archive_keys_a_renamed_club_as_one_club(tmp_path) -> None:
    from ml.xi.lineage import TeamLineage

    lineage = TeamLineage([{"from": "Kings XI Punjab", "to": "Punjab Kings", "gender": "male"}])

    before = _parse(tmp_path, _doc(["Kings XI Punjab", "Mumbai Indians"], "male", "Kings XI Punjab"), lineage)
    after = _parse(tmp_path, _doc(["Punjab Kings", "Mumbai Indians"], "male", "Punjab Kings"), lineage)

    assert before.team1 == after.team1 == "Punjab Kings|male"
    assert before.outcome == 1.0 and after.outcome == 1.0, "the winner is mapped with the sides"


def test_a_rename_recorded_for_one_gender_does_not_touch_the_other(tmp_path) -> None:
    from ml.xi.lineage import TeamLineage

    lineage = TeamLineage([{"from": "Lightning", "to": "The Blaze", "gender": "female"}])

    womens = _parse(tmp_path, _doc(["Lightning", "Sunrisers"], "female", "Lightning"), lineage)
    mens = _parse(tmp_path, _doc(["Lightning", "Sunrisers"], "male", "Lightning"), lineage)

    assert womens.team1 == "The Blaze|female"
    assert mens.team1 == "Lightning|male"


def test_the_lineage_loader_returns_an_empty_mapping_when_there_is_no_file(tmp_path, monkeypatch) -> None:
    from ml.xi import lineage as lineage_module

    monkeypatch.setenv(lineage_module.ENV_VAR, str(tmp_path / "absent.json"))
    monkeypatch.setattr(lineage_module, "_search_paths", lambda: ())

    mapping = lineage_module.load()

    assert len(mapping) == 0
    assert mapping.club("Kings XI Punjab", "male") == "Kings XI Punjab"


def test_the_committed_lineage_is_the_one_both_sources_read() -> None:
    """The go-app importer writes opposition.canonical_id from this same file; a mapping
    applied on one side only is the divergence make xi-parity exists to catch."""
    from ml.xi.lineage import load as load_lineage

    mapping = load_lineage()

    assert len(mapping) > 0
    assert mapping.club("Royal Challengers Bangalore", "male") == "Royal Challengers Bengaluru"
    assert mapping.club("Royal Challengers Bangalore", "female") == "Royal Challengers Bengaluru"
    assert mapping.club("Mumbai Indians", "male") == "Mumbai Indians"


def _passes_with_a_no_ball_and_four_leg_byes(postgres_reads_extras: bool):
    """Both sources over one match holding IMPORT-04's delivery -- a no-ball that ran
    away for four leg-byes -- with the database either reading the extras by kind or, as
    a database imported before migration 0018 does, holding zeros and charging the bowler
    all five. Every other count is identical either way."""
    match = _match("m0", 0, ["a1", "a2"], ["b1", "b2"])
    charged_one = replace(
        match, deliveries=replace(match.deliveries, runs_total=np.array([5.0]), runs_bowler=np.array([1.0]))
    )
    charged_five = replace(
        match, deliveries=replace(match.deliveries, runs_total=np.array([5.0]), runs_bowler=np.array([5.0]))
    )
    database = [charged_one if postgres_reads_extras else charged_five]
    return (
        build(_CountingSource(database, SourceCounts(offered=1, yielded=1))),
        build(_CountingSource([charged_one], SourceCounts(offered=1, yielded=1))),
    )


def test_the_pass_counts_the_runs_no_bowler_is_charged() -> None:
    """FEAT-08: the count that can see the extras breakdown -- byes, leg-byes and
    penalty runs, summed over every delivery read."""
    postgres, _ = _passes_with_a_no_ball_and_four_leg_byes(postgres_reads_extras=True)

    assert postgres.quality.runs_not_charged_to_bowler == 4


def test_parity_reports_extras_only_one_source_leaves_off_the_bowler() -> None:
    """FEAT-08, the guarantee itself: a database that charges the bowler every run agrees
    with the archive on every other count, and the parity check must still fail, naming
    the runs count."""
    from ml.xi.parity import compare

    postgres, cricsheet = _passes_with_a_no_ball_and_four_leg_byes(postgres_reads_extras=False)

    differences = compare(postgres, cricsheet)

    assert postgres.quality.matches_read == cricsheet.quality.matches_read == 1
    assert differences == ["runs_not_charged_to_bowler: postgres 0, cricsheet 4"]


def _passes_with_two_wickets_on_one_ball(postgres_holds_both: bool):
    """Both sources over one match holding IMPORT-06's delivery -- a batter bowled while
    his partner is run out on the same ball -- with the database either holding both
    wickets or, as a database that kept one wicket per delivery did, holding the first.
    Every other count is identical either way."""
    match = _match("m0", 0, ["a1", "a2"], ["b1", "b2"])
    both = replace(match, deliveries=replace(match.deliveries, wicket=np.array([2.0]), bowler_wicket=np.array([1.0])))
    first = replace(match, deliveries=replace(match.deliveries, wicket=np.array([1.0]), bowler_wicket=np.array([1.0])))
    database = [both if postgres_holds_both else first]
    return (
        build(_CountingSource(database, SourceCounts(offered=1, yielded=1))),
        build(_CountingSource([both], SourceCounts(offered=1, yielded=1))),
    )


def test_the_pass_counts_the_dismissals() -> None:
    """IMPORT-06: the count that can see a wicket record the store dropped -- every
    dismissal on every ball, summed over every delivery read."""
    postgres, _ = _passes_with_two_wickets_on_one_ball(postgres_holds_both=True)

    assert postgres.quality.dismissals == 2


def test_parity_reports_a_wicket_only_one_source_holds() -> None:
    """IMPORT-06, the guarantee itself: a database that kept the first wicket of a
    delivery agrees with the archive on every other count, and the parity check must
    still fail, naming the dismissals count."""
    from ml.xi.parity import compare

    postgres, cricsheet = _passes_with_two_wickets_on_one_ball(postgres_holds_both=False)

    differences = compare(postgres, cricsheet)

    assert postgres.quality.matches_read == cricsheet.quality.matches_read == 1
    assert differences == ["dismissals: postgres 1, cricsheet 2"]


def _passes_with_a_wide(postgres_reads_wides: bool):
    """Both sources over one match holding a wide, with the database either reading
    ``extras_wides`` or, as a database imported before migration 0018 does, holding a
    zero and counting the wide as faced. Every other count is identical either way."""
    match = _match("m0", 0, ["a1", "a2"], ["b1", "b2"])
    not_faced = replace(match, deliveries=replace(match.deliveries, faced=np.array([0.0])))
    faced = replace(match, deliveries=replace(match.deliveries, faced=np.array([1.0])))
    database = [not_faced if postgres_reads_wides else faced]
    return (
        build(_CountingSource(database, SourceCounts(offered=1, yielded=1))),
        build(_CountingSource([not_faced], SourceCounts(offered=1, yielded=1))),
    )


def test_the_pass_counts_the_deliveries_no_batter_faced() -> None:
    """IMPORT-05: the count that can see a wide -- the deliveries no batter faced, over
    every delivery read."""
    postgres, _ = _passes_with_a_wide(postgres_reads_wides=True)

    assert postgres.quality.deliveries_not_faced == 1


def test_parity_reports_a_wide_only_one_source_leaves_off_the_batter() -> None:
    """IMPORT-05, the guarantee itself: a database that counts every delivery as faced
    agrees with the archive on every other count, and the parity check must still fail,
    naming the faced count."""
    from ml.xi.parity import compare

    postgres, cricsheet = _passes_with_a_wide(postgres_reads_wides=False)

    differences = compare(postgres, cricsheet)

    assert postgres.quality.matches_read == cricsheet.quality.matches_read == 1
    assert differences == ["deliveries_not_faced: postgres 0, cricsheet 1"]
