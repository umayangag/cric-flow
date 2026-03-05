"""Adapter between backtest player predictions and reconciliation service.

This module lives in the `ml` package and is imported from `app.prediction_service`
behind an ImportError guard, so production can continue even when the ml stack
is not available.

Responsibilities:
- Build `MatchReconciliationInputs` from `BacktestPlayerPred` plus innings totals.
- Call `reconcile_match_players` to obtain integer per-player stats.
- Compute reconciliation adjustment metrics.
- Optionally run the consistency checker and attach any violations.
"""

from __future__ import annotations

from typing import Any, Dict, Iterable, List, Mapping, Optional, Tuple

from app.models import (
    BacktestPlayerPred,
    BattingPrediction,
    BowlingPrediction,
    InningsReconciliationPreferences,
    MatchReconciliationInputs,
    PlayerReconciliationPreferences,
)

from .config import get_prediction_defaults
from .consistency_checker import (
    InningsTargets,
    ReconciledPlayerStats,
    adjustment_magnitude,
    check_reconciled_scorecard_consistency,
)
from .reconciliation_service import reconcile_match_players

# Internal team id convention for constraint-based reconciliation (partitioning only)
TEAM1_ID = 1
TEAM2_ID = 2


def _default_bowling_deliveries_for_format(format_code: Optional[str]) -> float:
    """Return default bowling deliveries to assume for BacktestPlayerPred."""
    defaults = get_prediction_defaults()
    mapping = defaults.get("bowling_deliveries_by_format") or {}
    fmt_key = (format_code or "").upper()
    try:
        base = float(defaults.get("default_bowling_deliveries", 24.0))
    except (TypeError, ValueError):
        base = 24.0  # 4 overs, suitable for T20 as a pragmatic fallback
    if not fmt_key or not isinstance(mapping, dict):
        return base
    try:
        return float(mapping.get(fmt_key, base))
    except (TypeError, ValueError):
        return base


def build_match_reconciliation_inputs_from_backtest_preds(
    preds: Iterable[BacktestPlayerPred],
    team1_ids: Iterable[int],
    team2_ids: Iterable[int],
    inn1_runs: float,
    inn1_wkts: float,
    inn2_runs: float,
    inn2_wkts: float,
    match_id: int = 0,
    format_code: Optional[str] = None,
) -> MatchReconciliationInputs:
    """Construct MatchReconciliationInputs from raw backtest predictions and innings totals."""
    team1_set = {int(pid) for pid in team1_ids}

    deliveries_bowl = _default_bowling_deliveries_for_format(format_code)

    players: List[PlayerReconciliationPreferences] = []
    for p in preds:
        tid = TEAM1_ID if p.player_id in team1_set else TEAM2_ID
        r_conceded = (p.economy or 0.0) * (deliveries_bowl / 6.0)
        players.append(
            PlayerReconciliationPreferences(
                player_id=p.player_id,
                team_id=tid,
                batting=BattingPrediction(
                    runs_scored=p.runs or 0.0,
                    balls_faced=float(p.balls) if p.balls is not None else 0.0,
                    fours_scored=float(p.fours) if p.fours is not None else 0.0,
                    sixes_scored=float(p.sixes) if p.sixes is not None else 0.0,
                    batting_position=1.0,
                    strike_rate=(p.runs or 0) / (p.balls or 1) * 100.0,
                ),
                bowling=BowlingPrediction(
                    runs_conceded=r_conceded,
                    deliveries=deliveries_bowl,
                    wickets_taken=p.wickets or 0.0,
                    econ=p.economy or 6.0,
                ),
            )
        )

    innings = [
        InningsReconciliationPreferences(
            inning_number=1,
            batting_team_id=TEAM1_ID,
            bowling_team_id=TEAM2_ID,
            preferred_runs=inn1_runs,
            preferred_wickets=inn1_wkts,
        ),
        InningsReconciliationPreferences(
            inning_number=2,
            batting_team_id=TEAM2_ID,
            bowling_team_id=TEAM1_ID,
            preferred_runs=inn2_runs,
            preferred_wickets=inn2_wkts,
        ),
    ]

    return MatchReconciliationInputs(
        match_id=match_id,
        format=format_code,
        players=players,
        innings=innings,
        win=None,
    )


def apply_constraint_reconciliation_from_backtest_preds(
    preds: List[BacktestPlayerPred],
    team1_ids: Iterable[int],
    team2_ids: Iterable[int],
    inn1_runs: float,
    inn1_wkts: float,
    inn2_runs: float,
    inn2_wkts: float,
    match_id: int = 0,
    format_code: Optional[str] = None,
    default_economy: float = 6.0,
) -> Tuple[List[BacktestPlayerPred], Dict[str, Any]]:
    """Run constraint-based reconciliation and return adjusted preds plus adjustment stats."""
    if not preds:
        return preds, {"applied": False, "reason": "empty_predictions"}

    pref = build_match_reconciliation_inputs_from_backtest_preds(
        preds,
        team1_ids=team1_ids,
        team2_ids=team2_ids,
        inn1_runs=inn1_runs,
        inn1_wkts=inn1_wkts,
        inn2_runs=inn2_runs,
        inn2_wkts=inn2_wkts,
        match_id=match_id,
        format_code=format_code,
    )

    reconciled: Mapping[int, ReconciledPlayerStats] = reconcile_match_players(
        pref, team1_id=TEAM1_ID, team2_id=TEAM2_ID
    )
    pid_to_recon = {s.player_id: s for s in reconciled.values()}

    out: List[BacktestPlayerPred] = []
    before_stats: Dict[int, ReconciledPlayerStats] = {}
    after_stats: Dict[int, ReconciledPlayerStats] = {}
    fallback_bowling_balls = _default_bowling_deliveries_for_format(format_code)

    for p in preds:
        r = pid_to_recon.get(p.player_id)
        if r is None:
            # No reconciled stats for this player; keep original.
            out.append(p)
            continue

        econ = (r.bowling_runs / (r.bowling_balls / 6.0)) if r.bowling_balls > 0 else default_economy
        updated = BacktestPlayerPred(
            player_id=p.player_id,
            runs=float(r.batting_runs),
            balls=float(r.batting_balls) if r.batting_balls else p.balls,
            fours=p.fours,
            sixes=p.sixes,
            wickets=float(r.wickets),
            economy=econ,
            catches=p.catches,
            run_outs=p.run_outs,
        )
        out.append(updated)

        # Build before/after stats for adjustment magnitude.
        before_stats[p.player_id] = ReconciledPlayerStats(
            player_id=p.player_id,
            team_id=r.team_id,
            batting_runs=int(p.runs or 0),
            batting_balls=int(p.balls or 0) if p.balls is not None else 0,
            bowling_runs=int((p.economy or default_economy) * (r.bowling_balls or fallback_bowling_balls) / 6.0),
            bowling_balls=int(r.bowling_balls or 0),
            wickets=int(p.wickets or 0),
        )
        after_stats[p.player_id] = r

    metrics = adjustment_magnitude(before_stats, after_stats)
    adjustment: Dict[str, Any] = {
        "applied": True,
        **metrics,
    }

    # Consistency violations, if any.
    inn1_t = inn2_t = None
    for i in pref.innings:
        if i.inning_number == 1:
            inn1_t = InningsTargets(
                batting_team_id=TEAM1_ID,
                bowling_team_id=TEAM2_ID,
                runs=inn1_runs,
                wickets=inn1_wkts,
                legal_balls=float(i.preferred_legal_balls) if i.preferred_legal_balls is not None else None,
            )
        elif i.inning_number == 2:
            inn2_t = InningsTargets(
                batting_team_id=TEAM2_ID,
                bowling_team_id=TEAM1_ID,
                runs=inn2_runs,
                wickets=inn2_wkts,
                legal_balls=float(i.preferred_legal_balls) if i.preferred_legal_balls is not None else None,
            )

    violations = check_reconciled_scorecard_consistency(after_stats, TEAM1_ID, TEAM2_ID, inn1=inn1_t, inn2=inn2_t)
    if violations:
        adjustment["violations"] = violations

    return out, adjustment
