"""Unit tests for ml.match_aggregates."""

from ml.match_aggregates import (
    BattingLine,
    BowlingLine,
    aggregate_match_batting,
    aggregate_match_bowling,
    build_batting_scorecard,
    build_bowling_scorecard,
)
from ml.match_schema import BallEvent, InningsState, MatchState


def _make_simple_innings() -> InningsState:
    """Construct a tiny innings with a mix of events for testing."""
    balls = [
        # Dot ball to striker 1 by bowler 101
        BallEvent(
            match_id=1,
            innings=1,
            over=1,
            ball=1,
            ball_seq=1,
            is_legal=True,
            phase="powerplay",
            striker_id=1,
            non_striker_id=2,
            bowler_id=101,
            runs_batter=0,
            runs_extras=0,
            runs_total=0,
            extras_kind=None,
            wicket_kind=None,
            player_out_id=None,
        ),
        # Single to striker 1
        BallEvent(
            match_id=1,
            innings=1,
            over=1,
            ball=2,
            ball_seq=2,
            is_legal=True,
            phase="powerplay",
            striker_id=1,
            non_striker_id=2,
            bowler_id=101,
            runs_batter=1,
            runs_extras=0,
            runs_total=1,
            extras_kind=None,
            wicket_kind=None,
            player_out_id=None,
        ),
        # Four to striker 2 (after strike rotation)
        BallEvent(
            match_id=1,
            innings=1,
            over=1,
            ball=3,
            ball_seq=3,
            is_legal=True,
            phase="powerplay",
            striker_id=2,
            non_striker_id=1,
            bowler_id=101,
            runs_batter=4,
            runs_extras=0,
            runs_total=4,
            extras_kind=None,
            wicket_kind=None,
            player_out_id=None,
        ),
        # Wide ball (no legal delivery, 1 extra)
        BallEvent(
            match_id=1,
            innings=1,
            over=1,
            ball=3,  # same ball number, different ball_seq
            ball_seq=4,
            is_legal=False,
            phase="powerplay",
            striker_id=2,
            non_striker_id=1,
            bowler_id=101,
            runs_batter=0,
            runs_extras=1,
            runs_total=1,
            extras_kind="wide",
            wicket_kind=None,
            player_out_id=None,
        ),
        # Leg bye (1 run extras not charged to bowler, legal ball)
        BallEvent(
            match_id=1,
            innings=1,
            over=1,
            ball=4,
            ball_seq=5,
            is_legal=True,
            phase="powerplay",
            striker_id=2,
            non_striker_id=1,
            bowler_id=101,
            runs_batter=0,
            runs_extras=1,
            runs_total=1,
            extras_kind="leg_bye",
            wicket_kind=None,
            player_out_id=None,
        ),
        # Wicket: striker 2 caught off bowler 101
        BallEvent(
            match_id=1,
            innings=1,
            over=1,
            ball=5,
            ball_seq=6,
            is_legal=True,
            phase="powerplay",
            striker_id=2,
            non_striker_id=1,
            bowler_id=101,
            runs_batter=0,
            runs_extras=0,
            runs_total=0,
            extras_kind=None,
            wicket_kind="caught",
            player_out_id=2,
        ),
    ]
    inn = InningsState(
        match_id=1,
        innings_number=1,
        batting_team_id=10,
        bowling_team_id=20,
        target_runs=None,
        balls_per_innings=120,
        balls=balls,
    )
    return inn


def test_build_batting_scorecard_basic():
    inn = _make_simple_innings()
    lines = build_batting_scorecard(inn)

    # Two batters should appear
    assert set(lines.keys()) == {1, 2}

    p1 = lines[1]
    p2 = lines[2]

    # Batter 1: faced 2 legal balls (dot + single), scored 1 run
    assert isinstance(p1, BattingLine)
    assert p1.balls_faced == 2
    assert p1.runs == 1
    assert p1.fours == 0
    assert p1.sixes == 0
    assert p1.dismissed is False

    # Batter 2: faced 3 legal balls (four + leg bye + wicket ball), scored 4 runs
    assert isinstance(p2, BattingLine)
    assert p2.balls_faced == 3
    assert p2.runs == 4
    assert p2.fours == 1  # one boundary four
    assert p2.dismissed is True  # caught

    # Sanity: sum of batter runs matches sum of runs_batter
    total_bat_runs = p1.runs + p2.runs
    assert total_bat_runs == sum(b.runs_batter for b in inn.balls)


def test_build_bowling_scorecard_basic():
    inn = _make_simple_innings()
    lines = build_bowling_scorecard(inn)

    # Single bowler in this innings
    assert set(lines.keys()) == {101}
    bl = lines[101]
    assert isinstance(bl, BowlingLine)

    # Legal balls: 4 legal scoring/dot balls + wicket ball + leg bye = 5 legal balls
    assert bl.legal_balls == inn.legal_balls

    # Runs conceded: all batter runs + wides + no-balls, but exclude leg-byes
    # In this synthetic innings:
    # - batter runs: 1 + 4 = 5
    # - wide extras (charged to bowler): 1
    # - leg-bye extras (not charged to bowler): 1
    # total team runs: 5 (bat) + 2 (extras) = 7
    # bowler runs conceded: 5 (bat) + 1 (wide) = 6
    assert bl.runs_conceded == 6
    assert inn.total_runs == 7

    # Wickets: one caught dismissal credited to bowler
    assert bl.wickets == inn.total_wickets == 1


def test_match_level_aggregation():
    inn = _make_simple_innings()
    match = MatchState(
        match_id=1,
        format_code="T20",
        venue_id=1,
        season_id=2024,
        outcome_winner_team_id=None,
        innings_list=[inn],
    )

    bat_map = aggregate_match_batting(match)
    bowl_map = aggregate_match_bowling(match)

    # At match level we see the same players and stats as the single innings
    assert set(bat_map.keys()) == {1, 2}
    assert set(bowl_map.keys()) == {101}

    total_bat_runs = sum(l.runs for l in bat_map.values())
    total_bowl_runs = sum(l.runs_conceded for l in bowl_map.values())

    assert total_bat_runs == sum(b.runs_batter for b in inn.balls)
    assert total_bowl_runs == 6
    assert sum(l.legal_balls for l in bowl_map.values()) == inn.legal_balls
