r"""Core structures for constrained reconciliation of model outputs.

This module defines:
- The variable space \(x\) over which reconciliation operates.
- Linear constraints expressing cricket rules (runs, wickets, etc.).
- Preferred values μ and per-variable weights W for the objective
  \((x - μ)^T W (x - μ)\).

The actual numerical solver (QP / heuristic) is implemented separately; this
module is solver-agnostic and focused on **correctly encoding** the problem.
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Dict, List, Optional

import numpy as np

from app.models.reconciliation import MatchReconciliationInputs


class VariableKind(str, Enum):
    """Types of decision variables in the reconciliation vector x.

    We start with the most important score accounting pieces:
    - BAT_RUNS: per-player total batting runs (per match)
    - BAT_BALLS: per-player balls faced (optional; reserved for later)
    - BOWL_RUNS: per-player runs conceded (per match)
    - BOWL_BALLS: per-player balls bowled (optional; reserved for later)
    - BOWL_WKTS: per-player wickets taken (per match)

    The design is intentionally extensible so extras, fielding, and richer
    structures can be added without disrupting existing code.
    """

    BAT_RUNS = "bat_runs"
    BAT_BALLS = "bat_balls"
    BOWL_RUNS = "bowl_runs"
    BOWL_BALLS = "bowl_balls"
    BOWL_WKTS = "bowl_wkts"


@dataclass(frozen=True)
class Variable:
    """Metadata for one element of x."""

    index: int
    kind: VariableKind
    player_id: int
    team_id: int


class ConstraintKind(str, Enum):
    """Whether a constraint is hard (must hold) or soft (penalized if violated)."""

    HARD = "hard"
    SOFT = "soft"


@dataclass
class LinearConstraint:
    """One linear equality constraint of the form sum_i a_i x_i = rhs."""

    coefficients: Dict[int, float]
    rhs: float
    description: str
    kind: ConstraintKind = ConstraintKind.HARD


@dataclass
class ReconciliationProblem:
    """Fully specified reconciliation problem (without solution).

    Attributes:
        variables: metadata for each decision variable in x.
        mu: preferred values μ for each variable (shape (n_vars,)).
        weights: diagonal weights W (shape (n_vars,)), interpreted as
            per-variable penalties in \\sum_i w_i (x_i - μ_i)^2.
        constraints: list of LinearConstraint objects encoding cricket rules.
    """

    variables: List[Variable]
    mu: np.ndarray
    weights: np.ndarray
    constraints: List[LinearConstraint]


class ProblemBuilder:
    """Builder to construct a ReconciliationProblem from model outputs.

    This focuses initially on limited-overs, two-innings matches, where:
    - team1 bats in innings 1, bowls in innings 2
    - team2 bowls in innings 1, bats in innings 2

    It encodes the following hard constraints when the relevant preferred
    values are present:
    - Sum of batting runs for team1 players == preferred runs for innings 1.
    - Sum of batting runs for team2 players == preferred runs for innings 2.
    - Sum of bowling runs conceded by team2 bowlers == preferred runs for innings 1.
    - Sum of bowling runs conceded by team1 bowlers == preferred runs for innings 2.
    - Sum of wickets taken by team2 bowlers == preferred wickets for innings 1.
    - Sum of wickets taken by team1 bowlers == preferred wickets for innings 2.
    """

    def __init__(
        self,
        runs_weight: float = 1.0,
        wickets_weight: float = 1.0,
        soft_favor_top_order_balls: float = 0.0,
    ) -> None:
        self._runs_weight = float(runs_weight)
        self._wickets_weight = float(wickets_weight)
        # Soft realism: scale BAT_BALLS weight so top-order batsmen are trusted more
        # (penalty for deviating from their μ is higher). 0 = no effect.
        self._soft_favor_top_order_balls = float(soft_favor_top_order_balls)

    def build_from_match(
        self,
        pref: MatchReconciliationInputs,
        team1_id: int,
        team2_id: int,
    ) -> ReconciliationProblem:
        """Construct a reconciliation problem from preferred values.

        Args:
            pref: Preferred outputs from batting/bowling/innings/extras/win models.
            team1_id: Team id batting first.
            team2_id: Team id batting second.

        Returns:
            A ReconciliationProblem with variables, μ, weights, and constraints.
        """
        variables: List[Variable] = []
        mu_list: List[float] = []
        weights_list: List[float] = []
        constraints: List[LinearConstraint] = []

        # Map (kind, player_id) -> index
        index_by_key: Dict[tuple, int] = {}

        def _add_var(
            kind: VariableKind,
            player_id: int,
            team_id: int,
            mu_val: float,
            weight: float,
            batting_position: Optional[float] = None,
        ) -> int:
            key = (kind, player_id)
            if key in index_by_key:
                idx = index_by_key[key]
                return idx
            # Soft realism: favor top-order batsmen for BAT_BALLS (higher weight = trust μ more)
            w = float(weight)
            if kind == VariableKind.BAT_BALLS and batting_position is not None and self._soft_favor_top_order_balls > 0:
                # position 1 -> factor (1 + scale); position 11 -> factor 1
                pos = max(1.0, min(12.0, float(batting_position)))
                w *= 1.0 + self._soft_favor_top_order_balls * (12.0 - pos) / 11.0
            idx = len(variables)
            index_by_key[key] = idx
            variables.append(Variable(index=idx, kind=kind, player_id=player_id, team_id=team_id))
            mu_list.append(float(mu_val))
            weights_list.append(w)
            return idx

        # 1) Add per-player variables and μ from batting/bowling models.
        for p in pref.players:
            pid = p.player_id
            tid = p.team_id

            if p.batting is not None:
                _add_var(
                    VariableKind.BAT_RUNS,
                    player_id=pid,
                    team_id=tid,
                    mu_val=p.batting.runs_scored,
                    weight=self._runs_weight,
                )
                if p.batting.balls_faced is not None:
                    _add_var(
                        VariableKind.BAT_BALLS,
                        player_id=pid,
                        team_id=tid,
                        mu_val=p.batting.balls_faced,
                        weight=self._runs_weight,
                        batting_position=getattr(p.batting, "batting_position", None),
                    )

            if p.bowling is not None:
                _add_var(
                    VariableKind.BOWL_RUNS,
                    player_id=pid,
                    team_id=tid,
                    mu_val=p.bowling.runs_conceded,
                    weight=self._runs_weight,
                )
                _add_var(
                    VariableKind.BOWL_WKTS,
                    player_id=pid,
                    team_id=tid,
                    mu_val=p.bowling.wickets_taken,
                    weight=self._wickets_weight,
                )
                if p.bowling.deliveries is not None:
                    _add_var(
                        VariableKind.BOWL_BALLS,
                        player_id=pid,
                        team_id=tid,
                        mu_val=p.bowling.deliveries,
                        weight=self._runs_weight,
                    )

        # 2) Encode innings-level constraints for runs, wickets, and (optionally) balls where targets exist.
        #
        # We assume at most two innings here; additional innings can be handled
        # later by extending the mapping logic.
        innings_by_number = {i.inning_number: i for i in pref.innings}
        inn1 = innings_by_number.get(1)
        inn2 = innings_by_number.get(2)

        # Helper to add a constraint: sum(coeffs[idx] * x_idx) = rhs
        def _add_constraint(
            coeffs_by_player_and_kind, rhs: float, desc: str, kind: ConstraintKind = ConstraintKind.HARD
        ) -> None:
            coeffs: Dict[int, float] = {}
            for (kind_var, pid), coef in coeffs_by_player_and_kind.items():
                key = (kind_var, pid)
                idx = index_by_key.get(key)
                if idx is None:
                    continue
                if coef == 0.0:
                    continue
                coeffs[idx] = coeffs.get(idx, 0.0) + coef
            if coeffs:
                constraints.append(LinearConstraint(coefficients=coeffs, rhs=rhs, description=desc, kind=kind))

        # 2.1 Runs constraints
        if inn1 is not None and inn1.preferred_runs is not None:
            # Team1 bats in innings 1 -> batting runs of team1 players sum to inn1.preferred_runs
            coeffs = {}
            for p in pref.players:
                if p.team_id == team1_id:
                    coeffs[(VariableKind.BAT_RUNS, p.player_id)] = 1.0
            _add_constraint(coeffs, rhs=inn1.preferred_runs, desc="team1 batting runs = innings1 runs")

            # Team2 bowls in innings 1 -> bowling runs conceded by team2 players sum to inn1.preferred_runs
            coeffs = {}
            for p in pref.players:
                if p.team_id == team2_id:
                    coeffs[(VariableKind.BOWL_RUNS, p.player_id)] = 1.0
            _add_constraint(coeffs, rhs=inn1.preferred_runs, desc="team2 bowling runs = innings1 runs")

        if inn2 is not None and inn2.preferred_runs is not None:
            # Team2 bats in innings 2
            coeffs = {}
            for p in pref.players:
                if p.team_id == team2_id:
                    coeffs[(VariableKind.BAT_RUNS, p.player_id)] = 1.0
            _add_constraint(coeffs, rhs=inn2.preferred_runs, desc="team2 batting runs = innings2 runs")

            # Team1 bowls in innings 2
            coeffs = {}
            for p in pref.players:
                if p.team_id == team1_id:
                    coeffs[(VariableKind.BOWL_RUNS, p.player_id)] = 1.0
            _add_constraint(coeffs, rhs=inn2.preferred_runs, desc="team1 bowling runs = innings2 runs")

        # 2.2 Wickets constraints
        if inn1 is not None and inn1.preferred_wickets is not None:
            # Wickets in innings 1 are taken by team2 bowlers
            coeffs = {}
            for p in pref.players:
                if p.team_id == team2_id:
                    coeffs[(VariableKind.BOWL_WKTS, p.player_id)] = 1.0
            _add_constraint(coeffs, rhs=inn1.preferred_wickets, desc="team2 wickets = innings1 wickets")

        if inn2 is not None and inn2.preferred_wickets is not None:
            # Wickets in innings 2 are taken by team1 bowlers
            coeffs = {}
            for p in pref.players:
                if p.team_id == team1_id:
                    coeffs[(VariableKind.BOWL_WKTS, p.player_id)] = 1.0
            _add_constraint(coeffs, rhs=inn2.preferred_wickets, desc="team1 wickets = innings2 wickets")

        # 2.3 Balls constraints (legal balls per innings)
        # When preferred_legal_balls is provided, ensure that batting balls for the
        # batting team and bowling balls for the bowling team match this target.
        if inn1 is not None and getattr(inn1, "preferred_legal_balls", None) is not None:
            # Team1 bats in innings 1
            coeffs = {}
            for p in pref.players:
                if p.team_id == team1_id:
                    coeffs[(VariableKind.BAT_BALLS, p.player_id)] = 1.0
            _add_constraint(
                coeffs,
                rhs=float(inn1.preferred_legal_balls),
                desc="team1 batting balls = innings1 legal_balls",
            )

            # Team2 bowls in innings 1
            coeffs = {}
            for p in pref.players:
                if p.team_id == team2_id:
                    coeffs[(VariableKind.BOWL_BALLS, p.player_id)] = 1.0
            _add_constraint(
                coeffs,
                rhs=float(inn1.preferred_legal_balls),
                desc="team2 bowling balls = innings1 legal_balls",
            )

        if inn2 is not None and getattr(inn2, "preferred_legal_balls", None) is not None:
            # Team2 bats in innings 2
            coeffs = {}
            for p in pref.players:
                if p.team_id == team2_id:
                    coeffs[(VariableKind.BAT_BALLS, p.player_id)] = 1.0
            _add_constraint(
                coeffs,
                rhs=float(inn2.preferred_legal_balls),
                desc="team2 batting balls = innings2 legal_balls",
            )

            # Team1 bowls in innings 2
            coeffs = {}
            for p in pref.players:
                if p.team_id == team1_id:
                    coeffs[(VariableKind.BOWL_BALLS, p.player_id)] = 1.0
            _add_constraint(
                coeffs,
                rhs=float(inn2.preferred_legal_balls),
                desc="team1 bowling balls = innings2 legal_balls",
            )

        mu = np.asarray(mu_list, dtype=float)
        weights = np.asarray(weights_list, dtype=float)

        return ReconciliationProblem(
            variables=variables,
            mu=mu,
            weights=weights,
            constraints=constraints,
        )
