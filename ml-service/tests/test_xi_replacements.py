"""The eleven that started, not the squad that finished (FEAT-02).

Cricsheet lists everyone who took the field, so a side that used a concussion substitute,
an impact player or a supersub is listed as twelve, and the rating pass rated and
aggregated the twelve while serving always aggregates eleven. Who the twelfth man was is
a ``replacements.match`` entry on the delivery he came in at; both sources now read it and
leave him out of the side, and the pass counts him so ``make xi-parity`` can see a source
that does not.
"""

from __future__ import annotations

import json
import logging
from datetime import date
from typing import Dict, List, Optional

from ml.xi import contract as C
from ml.xi.builder import build
from ml.xi.parity import compare
from ml.xi.sources import MatchRecord, PostgresSource, parse_cricsheet_file, replacement_keys
from tests.test_xi_data_quality import _CountingSource, _match
from tests.test_xi_service_and_postgres import _FakeConnection
from tests.xi_fixtures import ListSource

# The delivery a replacement entry hangs on: what Cricsheet writes, less the wicket.
_ENTRY = {"reason": "concussion_substitute", "team": "X"}


def _delivery(batter: str, bowler: str, replacements: Optional[dict] = None) -> dict:
    ball = {"batter": batter, "bowler": bowler, "non_striker": "X1", "runs": {"batter": 1, "extras": 0, "total": 1}}
    if replacements is not None:
        ball["replacements"] = replacements
    return ball


def _innings(team: str, *deliveries: dict) -> dict:
    return {"team": team, "overs": [{"over": 0, "deliveries": list(deliveries)}]}


def _doc(players: Dict[str, List[str]], innings: List[dict]) -> dict:
    """One decided T20 between X and Y with the given squads and innings, keyed by registry."""
    return {
        "info": {
            "dates": [date(2024, 3, 1).isoformat()],
            "match_type": "T20",
            "teams": ["X", "Y"],
            "gender": "male",
            "venue": "Ground",
            "registry": {"people": {n: f"id_{n}" for names in players.values() for n in names}},
            "players": players,
            "outcome": {"winner": "X"},
        },
        "innings": innings,
    }


def _twelve_and_eleven() -> Dict[str, List[str]]:
    """X lists twelve, Y eleven; X11 is the man who came in."""
    return {"X": [f"X{i}" for i in range(12)], "Y": [f"Y{i}" for i in range(11)]}


def _parse(tmp_path, doc: dict) -> MatchRecord:
    path = tmp_path / "1.json"
    path.write_text(json.dumps(doc))
    record = parse_cricsheet_file(str(path), [])
    assert record is not None
    return record


# ---------------------------------------------------------------------------
# The rule, on the archive path
# ---------------------------------------------------------------------------


def test_replacement_keys_names_the_man_who_came_in() -> None:
    """The `in` of a replacements.match entry, keyed as the squad is."""
    innings = [
        _innings("X", _delivery("X0", "Y0"), _delivery("X0", "Y0", {"match": [{"in": "X11", "out": "X10", **_ENTRY}]}))
    ]

    assert replacement_keys(innings, {"X11": "id_X11"}) == [("X", "id_X11")]


def test_replacement_keys_reads_a_swap_back_as_one_replacement() -> None:
    """The archive's 1234909: a covid stand-in went back out and the man he stood in for
    came back. The side that started is the one without the stand-in."""
    innings = [
        _innings("X", _delivery("X0", "Y0", {"match": [{"in": "BG Lister", "out": "MS Chapman", **_ENTRY}]})),
        _innings("Y", _delivery("Y0", "X0", {"match": [{"in": "MS Chapman", "out": "BG Lister", **_ENTRY}]})),
    ]

    assert replacement_keys(innings, {}) == [("X", "name:BG Lister")]


def test_replacement_keys_ignores_a_role_replacement() -> None:
    """A substitute finishing an injured bowler's over changes nobody's membership."""
    innings = [_innings("X", _delivery("X0", "Y0", {"role": [{"in": "Y10", "reason": "injury", "role": "bowler"}]}))]

    assert replacement_keys(innings, {}) == []


def test_the_archive_path_reads_the_toss_through_the_team_key(tmp_path) -> None:
    """FEAT-05 on the archive path: ``info.toss.winner`` is a team name, keyed as the sides
    are, so the record says whether the side batting first won it; a file without a toss
    reads None. The win row carries it as ``toss_won_by_team1``, 0.5 when unknown."""
    from ml.xi.rows import toss_columns

    with_toss = _doc(_twelve_and_eleven(), [_innings("X", _delivery("X0", "Y0"))])
    with_toss["info"]["toss"] = {"winner": "Y", "decision": "field"}
    without = _doc(_twelve_and_eleven(), [_innings("X", _delivery("X0", "Y0"))])

    put_in, unknown = _parse(tmp_path, with_toss), _parse(tmp_path, without)

    assert put_in.toss_winner == put_in.team2 and put_in.toss_won_by_team1 == 0.0
    assert unknown.toss_winner is None and unknown.toss_won_by_team1 is None
    assert toss_columns(put_in) == {"toss_won_by_team1": 0.0}
    assert toss_columns(unknown) == {"toss_won_by_team1": 0.5}


def test_the_archive_path_leaves_the_replacement_out_of_the_eleven(tmp_path) -> None:
    """Twelve listed, one came in: the side is the other eleven and he is named apart."""
    players = _twelve_and_eleven()
    doc = _doc(players, [_innings("X", _delivery("X0", "Y0", {"match": [{"in": "X11", "out": "X10", **_ENTRY}]}))])

    record = _parse(tmp_path, doc)

    assert len(record.team1_players) == 11
    assert "id_X11" not in record.team1_players
    assert "id_X10" in record.team1_players, "the man he replaced started"
    assert record.replacements == ["id_X11"]
    assert len(record.team2_players) == 11 and record.team2_players == [f"id_Y{i}" for i in range(11)]


def test_the_archive_path_keeps_a_side_whose_replacement_it_cannot_name(tmp_path, caplog) -> None:
    """The archive's 1537342: the entry spells the man who came in differently from the
    squad. Nobody is guessed at; the side stays a twelve and the pass counts it oversized."""
    players = _twelve_and_eleven()
    doc = _doc(
        players, [_innings("X", _delivery("X0", "Y0", {"match": [{"in": "Someone Else", "out": "X10", **_ENTRY}]}))]
    )

    with caplog.at_level(logging.WARNING, logger="ml.xi.sources"):
        record = _parse(tmp_path, doc)

    assert len(record.team1_players) == 12
    assert record.replacements == []
    assert any("replacement not in the side's info.players" in message for message in caplog.messages)


# ---------------------------------------------------------------------------
# The same rule, on the database path
# ---------------------------------------------------------------------------


def test_the_players_query_reads_the_replacement_flag() -> None:
    """match_player.is_replacement (migration 0020) is what the importer wrote; the SQL is
    what makes the two sources agree."""
    from ml.xi.sources import _PLAYERS_SQL

    assert "mp.is_replacement" in _PLAYERS_SQL


def test_the_postgres_path_leaves_the_replacement_out_of_the_eleven() -> None:
    """A flagged row is not one of the eleven, and is named apart like the archive's."""
    squad = [(f"a{i:07x}", 10, False) for i in range(11)] + [("a000000b", 10, True)]
    squad += [(f"b{i:07x}", 20, False) for i in range(11)]
    tables = {
        "matches": [(1, date(2024, 1, 1), "T20I", "male", 5, 10, 20, 10, "", None, "", "", None, 10, date(2024, 1, 1))],
        "players": {1: squad},
        "balls": {1: [(1, 0, "a0000000", "b0000000", 4, 4, None, None, None, 0, 0, 0, 0)]},
    }

    (record,) = PostgresSource(_FakeConnection(tables), formats=["T20I"]).iter_matches()

    assert len(record.team1_players) == 11
    assert "a000000b" not in record.team1_players
    assert record.replacements == ["a000000b"]
    assert len(record.team2_players) == 11


# ---------------------------------------------------------------------------
# What the pass then does with the eleven
# ---------------------------------------------------------------------------


def test_the_pass_rates_and_aggregates_the_eleven_that_started(tmp_path) -> None:
    """The match's win row aggregates eleven, it yields eleven player rows for the side,
    and the replacement's Elo is untouched by a result he was not picked for -- while his
    deliveries still reach his own ledger."""
    players = _twelve_and_eleven()
    doc = _doc(
        players,
        [
            _innings(
                "X", _delivery("X0", "Y0"), _delivery("X0", "Y0", {"match": [{"in": "X11", "out": "X10", **_ENTRY}]})
            ),
            _innings("Y", _delivery("Y0", "X11"), _delivery("Y0", "X11")),
        ],
    )
    record = _parse(tmp_path, doc)

    result = build(ListSource([record]))

    row = result.frame.iloc[0]
    assert row["t1_n_debutants"] == 11.0, "everyone is a debutant on day one, and the side is eleven"
    assert row["t2_n_debutants"] == 11.0
    assert len(result.player_frame) == 22
    assert "id_X11" not in set(result.player_frame["player_key"])
    f = C.FORMAT_INDEX["T20"]
    slot_of = result.state.players.key_to_slot
    assert result.state.pelo[f, slot_of["id_X0"]] > C.ELO_INITIAL, "the eleven who started are rewarded"
    assert result.state.pelo[f, slot_of["id_X11"]] == C.ELO_INITIAL, "the replacement is not"
    assert result.state.career[f, slot_of["id_X11"]] == 0.0
    bowled = result.state.side_vectors("T20", ["id_X11"])
    assert bowled["exp_balls_bowled"][0] == 0.0, "no XI appearance, so no involvement"
    assert result.quality.replacement_players == 1
    assert result.quality.oversized_squads == 0


def test_the_pass_counts_the_replacements_it_left_out() -> None:
    matches = [_match("m0", 0, [f"a{i}" for i in range(11)], [f"b{i}" for i in range(11)])]
    matches[0].replacements = ["a11"]

    result = build(_CountingSource(matches, _counts(1)))

    assert result.quality.replacement_players == 1
    assert result.quality.oversized_squads == 0


def test_parity_reports_a_replacement_only_one_source_flags() -> None:
    """A database imported before 0020 hands over the twelve and flags nobody; it agrees
    with the archive on every other count, and the check must still name this one."""
    eleven = [f"a{i}" for i in range(11)]
    flagged = _match("m0", 0, eleven, [f"b{i}" for i in range(11)])
    flagged.replacements = ["a11"]
    unflagged = _match("m0", 0, eleven, [f"b{i}" for i in range(11)])

    differences = compare(
        build(_CountingSource([unflagged], _counts(1))), build(_CountingSource([flagged], _counts(1)))
    )

    assert differences == ["replacement_players: postgres 0, cricsheet 1"]


def _counts(n: int):
    from ml.xi.sources import SourceCounts

    return SourceCounts(offered=n, yielded=n)


def test_a_record_carries_no_replacements_unless_told() -> None:
    """The serving path builds records with nobody replaced."""
    record = _match("m0", 0, ["a1"], ["b1"])

    assert record.replacements == []


def test_a_replacement_named_for_one_side_is_not_looked_for_in_the_other(tmp_path, caplog) -> None:
    """The archive's 1537342: the entry names as the man who came in for X a player Y
    lists. He started for Y and stays there; X stays a twelve, and the pass says so."""
    players = _twelve_and_eleven()
    doc = _doc(players, [_innings("X", _delivery("X0", "Y0", {"match": [{"in": "Y10", "out": "X10", **_ENTRY}]}))])

    with caplog.at_level(logging.WARNING, logger="ml.xi.sources"):
        record = _parse(tmp_path, doc)

    assert len(record.team1_players) == 12
    assert "id_Y10" in record.team2_players and len(record.team2_players) == 11
    assert record.replacements == []
    assert any("X: id_Y10" in message for message in caplog.messages)
