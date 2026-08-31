"""Selection-consumer metrics for the L4 harness.

These are the numbers that gate an argmax over XIs, per the rearchitecture plan:

* **Swap monotonicity** (H-4): the share of one-player upgrades that *lower* the
  objective's P(win). An optimiser cannot trust a surface where making a player better
  makes the team worse.
* **Specific-XI-beyond-typical-XI**: does knowing the actual eleven beat knowing only the
  team's recent typical eleven? This is the selection-facing replacement for the P-0
  winner-accuracy gate, which an arm optimising both sides must lose regardless of XI
  quality.
* **Leak canary** (H-2): the best single column's AUC, and the TEST-format control from
  S-3c -- a column that discriminates in limited-overs formats but collapses in TEST looks
  like the "who batted" class of leak, whose signal vanishes where everyone bats.
"""

from __future__ import annotations

from collections import defaultdict, deque
from typing import Deque, Dict, List, Optional

import numpy as np
import pandas as pd
from sklearn.metrics import roc_auc_score

from ml.xi import contract as C
from ml.xi.ratings import aggregate_side, xi_feature_vector

# An upgrade must clearly beat float noise before it counts as a violation.
_VIOLATION_EPS = 1e-12
# How many recent matches define a team's "typical" side aggregates.
TYPICAL_WINDOW = 10
# A team needs at least this many prior matches before its typical XI means anything.
MIN_TYPICAL_HISTORY = 3
# Leak-canary thresholds: strong in a limited-overs format, collapsed in TEST.
CANARY_STRONG_AUC = 0.65
CANARY_COLLAPSED_AUC = 0.55


def _objective_probability(model, columns: List[str], own: Dict, opp: Dict) -> float:
    """Marginalised objective P(own side wins): scored batting first and second, averaged,
    exactly as the serving path does (H-3)."""
    x = np.vstack([xi_feature_vector(own, opp, columns), xi_feature_vector(opp, own, columns)])
    p = model.predict_proba(x)[:, 1]
    return float(0.5 * (p[0] + (1.0 - p[1])))


def swap_monotonicity(
    model, columns: List[str], player_rows: pd.DataFrame, format_code: str, max_matches: int = 50
) -> Optional[Dict]:
    """The share of one-player upgrades that lower the objective's P(win) (H-4).

    An upgrade raises one player's scoring, wicket and Elo ratings by one population
    standard deviation each -- strictly better on every axis the objective is constrained
    to like -- so any probability drop is a monotonicity violation, not a trade-off.
    """
    if player_rows.empty:
        return None
    steps = {
        name: float(player_rows[name].std()) for name in ("bat_rate", "bat_wrate", "bowl_rate", "bowl_wrate", "pelo")
    }
    upgrades = 0
    violations = 0
    match_ids = player_rows.match_id.drop_duplicates().tolist()[:max_matches]
    for match_id in match_ids:
        match = player_rows[player_rows.match_id == match_id]
        side1 = match[match.side == 1]
        side2 = match[match.side == 2]
        if side1.empty or side2.empty:
            continue
        vectors = {name: side1[name].to_numpy(dtype=float) for name in C.PLAYER_VECTOR_KEYS}
        opponent = aggregate_side(
            {name: side2[name].to_numpy(dtype=float) for name in C.PLAYER_VECTOR_KEYS}, format_code
        )
        base = _objective_probability(model, columns, aggregate_side(vectors, format_code), opponent)
        for j in range(len(side1)):
            upgraded = {name: values.copy() for name, values in vectors.items()}
            for name, step in steps.items():
                upgraded[name][j] += step
            p = _objective_probability(model, columns, aggregate_side(upgraded, format_code), opponent)
            upgrades += 1
            if p < base - _VIOLATION_EPS:
                violations += 1
    if upgrades == 0:
        return None
    return {
        "upgrades": upgrades,
        "violations": violations,
        "violation_share": violations / upgrades,
    }


def _swap_stems(rows: pd.DataFrame) -> pd.DataFrame:
    """The same fixtures with the sides exchanged, XI columns only (the objective sees no
    team context)."""
    out = rows.copy()
    for stem in C.SIDE_FEATURE_STEMS:
        out[f"t1_{stem}"], out[f"t2_{stem}"] = rows[f"t2_{stem}"], rows[f"t1_{stem}"]
        out[f"d_{stem}"] = -rows[f"d_{stem}"]
    return out


def _marginalised_auc(model, columns: List[str], rows: pd.DataFrame) -> float:
    y = rows[C.TARGET_COL].to_numpy(dtype=float)
    x_a = rows[columns].fillna(0.0).to_numpy(dtype=float)
    x_b = _swap_stems(rows)[columns].fillna(0.0).to_numpy(dtype=float)
    p = 0.5 * (model.predict_proba(x_a)[:, 1] + (1.0 - model.predict_proba(x_b)[:, 1]))
    return float(roc_auc_score(y, p))


def specific_vs_typical(
    model, columns: List[str], format_frame: pd.DataFrame, eval_start: pd.Timestamp, eval_end: pd.Timestamp
) -> Optional[Dict]:
    """AUC of the objective on the real XIs minus its AUC on each team's *typical* recent
    side aggregates, over the same evaluation matches.

    A positive delta means the specific eleven carries information beyond the team label
    -- which is the property a selection objective needs, and which winner accuracy
    cannot measure (P-0).
    """
    history: Dict[str, Deque[np.ndarray]] = defaultdict(lambda: deque(maxlen=TYPICAL_WINDOW))
    stems = C.SIDE_FEATURE_STEMS
    typical_rows: List[Dict] = []
    ordered = format_frame.sort_values(["match_date", "match_id"], kind="stable")
    for row in ordered.itertuples():
        side1 = np.asarray([getattr(row, f"t1_{stem}") for stem in stems], dtype=float)
        side2 = np.asarray([getattr(row, f"t2_{stem}") for stem in stems], dtype=float)
        history1, history2 = history[row.team1], history[row.team2]
        in_window = eval_start <= row.match_date < eval_end
        if in_window and len(history1) >= MIN_TYPICAL_HISTORY and len(history2) >= MIN_TYPICAL_HISTORY:
            typical1 = np.mean(history1, axis=0)
            typical2 = np.mean(history2, axis=0)
            typical_row = {"match_id": row.match_id, C.TARGET_COL: getattr(row, C.TARGET_COL)}
            for i, stem in enumerate(stems):
                typical_row[f"t1_{stem}"] = typical1[i]
                typical_row[f"t2_{stem}"] = typical2[i]
                typical_row[f"d_{stem}"] = typical1[i] - typical2[i]
            typical_rows.append(typical_row)
        history1.append(side1)
        history2.append(side2)
    if len(typical_rows) < 20:
        return None
    typical = pd.DataFrame(typical_rows)
    if typical[C.TARGET_COL].nunique() < 2:
        return None
    specific = ordered[ordered.match_id.isin(set(typical.match_id))]
    auc_specific = _marginalised_auc(model, columns, specific)
    auc_typical = _marginalised_auc(model, columns, typical)
    return {
        "n": int(len(typical)),
        "auc_specific_xi": auc_specific,
        "auc_typical_xi": auc_typical,
        "delta": auc_specific - auc_typical,
    }


def leak_canary(frame: pd.DataFrame, dev_start: pd.Timestamp, dev_end: pd.Timestamp) -> Dict:
    """Best single column per format over the development windows, plus the TEST control
    (H-2). Never computed on the locked window -- acting on a canary is a choice."""
    dev = frame[(frame.match_date >= dev_start) & (frame.match_date < dev_end)]
    per_format_auc: Dict[str, Dict[str, float]] = {}
    best: Dict[str, Dict] = {}
    for format_code, rows in dev.groupby("format_code", sort=True):
        if len(rows) < 50 or rows[C.TARGET_COL].nunique() < 2:
            continue
        y = rows[C.TARGET_COL].to_numpy(dtype=float)
        aucs: Dict[str, float] = {}
        for column in C.DISPLAY_FEATURE_COLS:
            auc = roc_auc_score(y, rows[column].fillna(0.0))
            aucs[column] = float(max(auc, 1.0 - auc))
        per_format_auc[str(format_code)] = aucs
        best_column = max(aucs, key=aucs.get)
        best[str(format_code)] = {"column": best_column, "auc": aucs[best_column], "n": int(len(rows))}
    suspects: List[Dict] = []
    test_aucs = per_format_auc.get("TEST", {})
    for format_code, aucs in per_format_auc.items():
        if format_code == "TEST" or not test_aucs:
            continue
        for column, auc in aucs.items():
            if auc >= CANARY_STRONG_AUC and test_aucs.get(column, 1.0) <= CANARY_COLLAPSED_AUC:
                suspects.append({"column": column, "format": format_code, "auc": auc, "test_auc": test_aucs[column]})
    return {"best_single_column": best, "test_control_suspects": suspects}
