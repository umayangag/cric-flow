"""Helpers for win-probability coherence metrics.

These functions are intended for offline analysis and monitoring rather than
the core inference path. They provide a simple, transparent way to compare:

- The model's win probability for team1, and
- An "implied" win probability derived from reconciled score margins.

This does not try to be a full generative model; it is a pragmatic bridge
between reconciled score distributions and win-model outputs for diagnostics.
"""

from __future__ import annotations

from math import exp
from typing import Dict


def implied_win_probability_from_margin(margin: float, scale: float = 25.0) -> float:
    """Convert a score margin into an implied win probability for team1.

    Args:
        margin: Runs margin in team1's favour (team1_runs - team2_runs). Positive
            means team1 ahead, negative means behind.
        scale: Scale parameter controlling how quickly the probability moves away
            from 0.5 as the margin grows. Larger values make the curve flatter.

    Returns:
        A float in [0, 1] representing implied P(team1 wins).
    """
    try:
        s = float(scale)
        if s <= 0:
            s = 25.0
    except (TypeError, ValueError):
        s = 25.0
    z = float(margin) / s
    # Clamp z to avoid overflow in exp for extreme margins.
    if z > 50.0:
        p = 1.0
    elif z < -50.0:
        p = 0.0
    else:
        # Logistic transform; clamp to avoid exact 0 or 1.
        p = 1.0 / (1.0 + exp(-z))
    if p < 1e-6:
        return 1e-6
    if p > 1.0 - 1e-6:
        return 1.0 - 1e-6
    return p


def win_probability_coherence_from_margin(
    p_model_team1: float,
    margin: float,
    scale: float = 25.0,
) -> Dict[str, float]:
    """Compute a simple coherence metric between model win prob and score margin.

    This is meant for offline diagnostics: given a reconciled final or projected
    margin and the model's win probability, how far apart are they?

    Returns:
        Dict with the model probability, implied probability, and absolute diff.
    """
    try:
        p_m = float(p_model_team1)
    except (TypeError, ValueError):
        p_m = 0.5
    # Bound model probability to [0, 1] for safety.
    p_m = max(0.0, min(1.0, p_m))
    p_implied = implied_win_probability_from_margin(margin, scale=scale)
    return {
        "p_model_team1": p_m,
        "p_implied_team1": p_implied,
        "abs_diff": abs(p_m - p_implied),
    }

