"""Helpers to build WinFeatures in a standardized way.

.. deprecated::
    This module is superseded by ``ml.win_features`` which uses distribution
    statistics (mean, std, max, min, top3_mean) instead of simple sums. It
    is retained only as a legacy fallback for ``generate_match`` when the
    enhanced win module is unavailable.  New code should use
    ``ml.win_features.aggregate_team_features_from_player_maps`` instead.

This module defines a single helper that constructs `WinFeatures` from the
canonical inputs used by the legacy (sum-only) win model:

- Format / venue / season
- Opposition ids (team1, team2, toss winner)
- Aggregated batting/bowling consistency and form sums per team
"""

from __future__ import annotations

from app.models import WinFeatures


def build_win_features_standardized(
    *,
    format_code: str,
    format_id: int,
    venue_id: int,
    season_id: int,
    team1_opposition_id: int,
    team2_opposition_id: int,
    toss_winner_opposition_id: int,
    team1_bat_consistency_sum: float,
    team1_bowl_consistency_sum: float,
    team2_bat_consistency_sum: float,
    team2_bowl_consistency_sum: float,
    team1_bat_form_sum: float,
    team1_bowl_form_sum: float,
    team2_bat_form_sum: float,
    team2_bowl_form_sum: float,
) -> WinFeatures:
    """Build a WinFeatures instance from standardized scalar inputs.

    This function is intentionally thin but provides a single place where the
    canonical mapping from scalar match-level summaries to `WinFeatures` is
    defined. Callers are responsible for computing the consistency/form sums
    (typically from pre-game features) and for providing the correct ids.
    """
    fmt = (format_code or "").strip().upper()
    return WinFeatures(
        format_id=int(format_id),
        venue_id=int(venue_id),
        team1_opposition_id=int(team1_opposition_id),
        team2_opposition_id=int(team2_opposition_id),
        toss_winner_opposition_id=int(toss_winner_opposition_id),
        temp=0,
        wind=0,
        rain=0,
        humidity=0,
        cloud=0,
        pressure=0,
        viscosity=0,
        team1_bat_consistency_sum=float(team1_bat_consistency_sum),
        team1_bowl_consistency_sum=float(team1_bowl_consistency_sum),
        team2_bat_consistency_sum=float(team2_bat_consistency_sum),
        team2_bowl_consistency_sum=float(team2_bowl_consistency_sum),
        team1_bat_form_sum=float(team1_bat_form_sum),
        team1_bowl_form_sum=float(team1_bowl_form_sum),
        team2_bat_form_sum=float(team2_bat_form_sum),
        team2_bowl_form_sum=float(team2_bowl_form_sum),
        format=fmt,
    )
