"""
Ball-by-ball data loader for the generalized ML pipeline.

Loads ball_event rows joined with match, match_inning, and format for era normalization
and feature engineering. Supports both database and CSV sources.
"""

from __future__ import annotations

import logging
from datetime import date
from typing import Optional

import pandas as pd

logger = logging.getLogger(__name__)

# Expected columns from ball-by-ball export / DB query
BALL_COLS = [
    "match_id",
    "innings",
    "over",
    "ball",
    "ball_seq",
    "is_legal",
    "phase",
    "striker_id",
    "non_striker_id",
    "bowler_id",
    "runs_batter",
    "runs_extras",
    "runs_total",
    "extras_kind",
    "wicket_kind",
    "player_out_id",
]

MATCH_CONTEXT_COLS = [
    "match_id",
    "match_date",
    "format_id",
    "format_code",
    "venue_id",
    "target_runs",
    "balls_per_innings",
    "innings",
]


def load_ball_by_ball_from_db(
    cutoff_date: Optional[date] = None,
    format_codes: Optional[list[str]] = None,
) -> pd.DataFrame:
    """Load ball-by-ball events from PostgreSQL with match and inning context.

    Args:
        cutoff_date: Only include matches on or before this date (for walk-forward).
        format_codes: Filter by format (e.g. ["ODI", "T20I"]). None = all formats.

    Returns:
        DataFrame with ball events + match_date, format_code, target_runs, balls_per_innings.
    """
    from ml.db import get_db_connection

    format_filter = ""
    params: list = []
    if format_codes:
        placeholders = ", ".join("%s" for _ in format_codes)
        format_filter = f" AND mf.code IN ({placeholders})"
        params.extend(format_codes)

    if cutoff_date:
        params.append(cutoff_date)
        cutoff_sql = " AND m.match_date <= %s"
    else:
        cutoff_sql = ""

    q = f"""
    SELECT
        be.match_id,
        be.innings,
        be."over",
        be.ball,
        be.ball_seq,
        be.is_legal,
        be.phase,
        be.striker_id,
        be.non_striker_id,
        be.bowler_id,
        be.runs_batter,
        be.runs_extras,
        be.runs_total,
        be.extras_kind,
        be.wicket_kind,
        be.player_out_id,
        m.match_date,
        m.format_id,
        mf.code AS format_code,
        m.venue_id,
        m.season_id,
        m.outcome_winner_opposition_id,
        mi.target_runs,
        mi.batting_team_opposition_id,
        mi.bowling_team_opposition_id,
        COALESCE(m.scheduled_overs_per_innings * m.balls_per_over, 120) AS balls_per_innings
    FROM ball_event be
    JOIN match m ON m.match_id = be.match_id
    JOIN match_format mf ON mf.id = m.format_id
    JOIN match_inning mi ON mi.match_id = be.match_id AND mi.inning_number = be.innings
    WHERE be.is_legal = TRUE
    {cutoff_sql}
    {format_filter}
    ORDER BY be.match_id, be.innings, be.ball_seq
    """
    conn = get_db_connection()
    try:
        df = pd.read_sql(q, conn, params=params if params else None)
    finally:
        conn.close()

    if "match_date" in df.columns:
        df["match_date"] = pd.to_datetime(df["match_date"]).dt.date
    logger.info(
        "ball_by_ball_loader.loaded_from_db rows=%d matches=%d",
        len(df),
        df["match_id"].nunique() if not df.empty else 0,
    )
    return df


def load_ball_by_ball_from_csv(path: str) -> pd.DataFrame:
    """Load ball-by-ball data from CSV.

    CSV should have columns compatible with BALL_COLS + MATCH_CONTEXT_COLS.
    """
    df = pd.read_csv(path)
    if "match_date" in df.columns:
        df["match_date"] = pd.to_datetime(df["match_date"]).dt.date
    logger.info(
        "ball_by_ball_loader.loaded_from_csv path=%s rows=%d",
        path,
        len(df),
    )
    return df
