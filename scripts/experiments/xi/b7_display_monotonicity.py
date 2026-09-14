"""B-7, part 2: does constraining the display model's team context make its surface cohere?

The display model is the probability a person watches move when they swap a player in the
Team Lab, and the harness now measures how often that move goes the wrong way
(``ml.xi.selection_metrics.display_swap_monotonicity``, part 1). It measures 3-7 %, against
the 2 % H-4 holds the *objective* to. B-7's recorded diagnosis was that the display model's
``monotonic_cst`` gives its team-context columns 0, so nothing constrains them.

Gate ``B-7-display-monotone`` tests that reading directly: the same folds, the same seeds,
the same columns, with ``team_elo_diff``, ``team_form_diff`` and ``venue_fam_diff`` given
+1 instead of 0. Its triple is in ``ml.xi.gates`` and is printed before the run.

Two questions, asked together because they trade off: does the swap-violation share fall
materially toward H-4's line, and does display AUC hold beyond the noise floor? A fall
bought with discrimination is not this gate's to spend -- it is recorded with its fold
table for a deliberate decision.

Beside the arms, one diagnostic that is not an arm and ships nothing: the same boosting
model fitted on ``XI_FEATURE_COLS`` alone, probed the same way. The probe holds team
context at the fixture's values because a selector cannot change it, so if a model that
never sees team context violates just as often, the recorded diagnosis is wrong and the
constraint cannot be the fix. Asking that costs one fit per fold and settles the mechanism.
A second diagnostic asks the same question in feature space, with no model at all: which
columns an upgrade actually moves, and whether any of them is one the contract constrains.

Where those two point, gate ``B-7-pelo-spread`` prices: the same folds with the display
model's ``t1_pelo_std`` / ``t2_pelo_std`` removed. It INFORMS and shipped nothing on its
own judgement -- B-7 scoped a constraint, not a change to what the display model reads --
but its reading (exactly 0.0000 violations everywhere, for -0.0004 / +0.0088 / -0.0055 /
-0.0047 of display AUC) was then taken by decision, so ``DISPLAY_FEATURE_COLS`` no longer
carries the two columns and this script's control names them itself (``PRE_B7_DISPLAY_COLS``).

    python b7_display_monotonicity.py --frames frames.pkl --out b7.json
    python b7_display_monotonicity.py --decide b7.json
"""

from __future__ import annotations

import argparse
import json
import logging
import math
import os
import sys
import time
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from sim_frame_cache import load_frames  # noqa: E402

from ml.xi import contract as C  # noqa: E402
from ml.xi import gates  # noqa: E402
from ml.xi.evaluate import (  # noqa: E402
    MIN_EVAL_ROWS,
    MIN_TRAIN_ROWS,
    SWAP_MAX_MATCHES,
    fold_windows,
)
from ml.xi.ratings import aggregate_side, match_features  # noqa: E402
from ml.xi.selection_metrics import display_swap_monotonicity, upgrade_steps, upgraded_sides  # noqa: E402
from ml.xi.train import _score_marginalised, _xy, make_display_model  # noqa: E402

logger = logging.getLogger("b7_display_monotonicity")

GATE_ID = "B-7-display-monotone"
#: The second, informing-only gate: the column the feature-space diagnostic names.
SPREAD_GATE_ID = "B-7-pelo-spread"
#: The spread of player Elo across an eleven, in each side's raw form. There is no
#: ``d_pelo_std``: the contract never made this stem a differential.
PELO_SPREAD_COLS: Tuple[str, ...] = C.DISPLAY_EXCLUDED_COLS
#: The display columns as they were when this gate ran, in the contract's own order. The
#: reading below was acted on -- ``DISPLAY_FEATURE_COLS`` no longer carries the spread
#: columns -- so the control has to name them itself, or after the ship both arms would be
#: the same list and the gate would compare a model with itself.
PRE_B7_DISPLAY_COLS: List[str] = (
    list(C.XI_FEATURE_COLS) + list(C.TEAM_CONTEXT_COLS) + (list(C.STAKES_COLS) if C.STAKES_FEATURES_KEPT else [])
)
#: H-4's line, which this gate measures the distance to and does not itself enforce.
SWAP_VIOLATION_LIMIT = 0.02
#: How much of the control's own distance from H-4's line the arm must close before the
#: fall counts as material. A-1's lesson: a bar written as "any fall" is a bar a run of
#: noise clears, and a surface improved by a thousandth is not a surface a user notices.
MATERIAL_FALL_FRACTION = 0.25


def _fold_entry(
    format_frame: pd.DataFrame, cutoff: pd.Timestamp, end: pd.Timestamp
) -> Tuple[Dict[str, Any], Optional[pd.DataFrame], Optional[pd.DataFrame]]:
    """The fold's train / evaluation split, or a skip entry saying why there is none."""
    train = format_frame[format_frame.match_date < cutoff]
    evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
    entry: Dict[str, Any] = {"cutoff": cutoff.date().isoformat(), "n_train": int(len(train))}
    if len(train) < MIN_TRAIN_ROWS or train[C.TARGET_COL].nunique() < 2:
        entry["skipped_reason"] = "insufficient training rows"
        return entry, None, None
    if len(evaluation) < MIN_EVAL_ROWS or evaluation[C.TARGET_COL].nunique() < 2:
        entry["skipped_reason"] = "evaluation window too small or single-class"
        return entry, None, None
    entry["n_eval"] = int(len(evaluation))
    return entry, train, evaluation


def display_arm(
    format_frame: pd.DataFrame,
    player_frame: pd.DataFrame,
    format_code: str,
    constrain_team_context: bool = False,
    columns: Optional[List[str]] = None,
) -> Dict[str, Any]:
    """Walk-forward display AUC, Brier and swap-violation share for one arm.

    Everything but ``constrain_team_context`` and ``columns`` is the harness's own
    setting, so a number here is comparable with the one ``make evaluate`` reports.
    """
    columns = list(columns or PRE_B7_DISPLAY_COLS)
    folds: List[Dict[str, Any]] = []
    format_players = player_frame[player_frame.format_code == format_code]
    for cutoff, end in fold_windows():
        entry, train, evaluation = _fold_entry(format_frame, cutoff, end)
        if train is None:
            folds.append(entry)
            continue
        x, y = _xy(train, columns)
        model = make_display_model(columns, constrain_team_context=constrain_team_context).fit(x, y)
        scores = _score_marginalised(model, evaluation, columns)
        entry["display_auc_mean"] = scores["auc"]
        entry["display_brier_mean"] = scores["brier"]
        window_players = format_players[(format_players.match_date >= cutoff) & (format_players.match_date < end)]
        probe = display_swap_monotonicity(
            model, columns, evaluation, window_players, format_code, max_matches=SWAP_MAX_MATCHES
        )
        entry["swap_violation_share"] = None if probe is None else probe["violation_share"]
        entry["swap_upgrades"] = None if probe is None else probe["upgrades"]
        folds.append(entry)
    return {"constrain_team_context": constrain_team_context, "columns": columns, "folds": folds}


def xi_only_diagnostic(format_frame: pd.DataFrame, player_frame: pd.DataFrame, format_code: str) -> Dict[str, Any]:
    """The mechanism check: the same boosting model that never reads team context at all.

    It is not an arm -- nothing about it could ship, since a display model without team
    context is the objective with a different model class. It answers one question: is the
    violated monotonicity caused by the unconstrained team-context columns, or by the tree
    model's response to the XI columns themselves?
    """
    folds: List[Dict[str, Any]] = []
    format_players = player_frame[player_frame.format_code == format_code]
    for cutoff, end in fold_windows():
        entry, train, evaluation = _fold_entry(format_frame, cutoff, end)
        if train is None:
            folds.append(entry)
            continue
        x, y = _xy(train, C.XI_FEATURE_COLS)
        model = make_display_model(C.XI_FEATURE_COLS).fit(x, y)
        entry["display_auc_mean"] = _score_marginalised(model, evaluation, C.XI_FEATURE_COLS)["auc"]
        window_players = format_players[(format_players.match_date >= cutoff) & (format_players.match_date < end)]
        probe = display_swap_monotonicity(
            model, C.XI_FEATURE_COLS, evaluation, window_players, format_code, max_matches=SWAP_MAX_MATCHES
        )
        entry["swap_violation_share"] = None if probe is None else probe["violation_share"]
        folds.append(entry)
    return {"columns": "XI_FEATURE_COLS", "folds": folds}


def feature_move_diagnostic(format_frame: pd.DataFrame, player_frame: pd.DataFrame, format_code: str) -> Dict[str, Any]:
    """What an "upgrade" does to the feature vector, with no model in the way.

    A per-column monotone constraint can only keep a probability from falling when every
    constrained column moves the way it likes. This counts how often that is untrue: the
    same upgrade the probe applies, and the share of upgrades that move at least one
    constrained column *against* its declared direction, or move a column the contract
    leaves unconstrained. It is arithmetic on the aggregates, not a measurement of any
    model, which is why it explains a result the arms can only report.
    """
    # Only the XI columns are looked at: the team-context columns are held at the
    # fixture's values by construction, so their delta is exactly zero whatever anyone
    # constrains them to -- which is the first half of this diagnostic's answer.
    directions = dict(zip(C.XI_FEATURE_COLS, C.monotone_directions(C.XI_FEATURE_COLS)))
    format_players = player_frame[player_frame.format_code == format_code]
    upgrades = 0
    against_direction = 0
    unconstrained_moved = 0
    per_column_against: Dict[str, int] = {}
    per_column_unconstrained: Dict[str, int] = {}
    for cutoff, end in fold_windows():
        entry, train, evaluation = _fold_entry(format_frame, cutoff, end)
        if train is None:
            continue
        window_players = format_players[(format_players.match_date >= cutoff) & (format_players.match_date < end)]
        if window_players.empty:
            continue
        steps = upgrade_steps(window_players)
        by_match = {match_id: group for match_id, group in window_players.groupby("match_id", sort=False)}
        for match in evaluation.head(SWAP_MAX_MATCHES).itertuples():
            players = by_match.get(match.match_id)
            if players is None:
                continue
            side1, side2 = players[players.side == 1], players[players.side == 2]
            if side1.empty or side2.empty:
                continue
            vectors = {name: side1[name].to_numpy(dtype=float) for name in C.PLAYER_VECTOR_KEYS}
            opponent = aggregate_side(
                {name: side2[name].to_numpy(dtype=float) for name in C.PLAYER_VECTOR_KEYS}, format_code
            )
            variants = upgraded_sides(vectors, steps, len(side1))
            rows = [match_features(aggregate_side(variant, format_code), opponent) for variant in variants]
            base = rows[0]
            for row in rows[1:]:
                upgrades += 1
                fell = [c for c, d in directions.items() if d and (row[c] - base[c]) * d < -1e-12]
                moved = [c for c, d in directions.items() if not d and abs(row[c] - base[c]) > 1e-12]
                against_direction += bool(fell)
                unconstrained_moved += bool(moved)
                for column in fell:
                    per_column_against[column] = per_column_against.get(column, 0) + 1
                for column in moved:
                    per_column_unconstrained[column] = per_column_unconstrained.get(column, 0) + 1
    if upgrades == 0:
        return {"upgrades": 0}

    def share(counts: Dict[str, int]) -> Dict[str, float]:
        return {c: n / upgrades for c, n in sorted(counts.items(), key=lambda kv: -kv[1])}

    return {
        "upgrades": upgrades,
        "share_moving_a_constrained_column_against_its_direction": against_direction / upgrades,
        "share_moving_an_unconstrained_column": unconstrained_moved / upgrades,
        "per_column_against_direction": share(per_column_against),
        "per_column_unconstrained_moved": share(per_column_unconstrained),
    }


# --- Reading the arms -------------------------------------------------------------------


def _paired(arm: List[Dict], control: List[Dict], key: str) -> Dict[str, Any]:
    """Arm minus control, fold by fold: the mean, and the standard error of that mean."""
    values = np.asarray(
        [
            a[key] - b[key]
            for a, b in zip(arm, control)
            if a.get(key) is not None and b.get(key) is not None and a["cutoff"] == b["cutoff"]
        ],
        dtype=float,
    )
    if not len(values):
        return {"mean": None, "se": None, "n_folds": 0}
    se = float(np.std(values, ddof=1) / math.sqrt(len(values))) if len(values) > 1 else float("nan")
    return {"mean": float(values.mean()), "se": se, "n_folds": int(len(values))}


def _mean(folds: List[Dict], key: str) -> Optional[float]:
    values = [f[key] for f in folds if f.get(key) is not None]
    return float(np.mean(values)) if values else None


def verdict(arm: Dict[str, Any], control: Dict[str, Any], diagnostic: Dict[str, Any]) -> Dict[str, Any]:
    """The registered rule, applied to one format's folds.

    Both clauses are read against a floor: a fall inside the paired standard error is not
    a measurement, and a fall too small to close a quarter of the gap to H-4's line is not
    worth a change to a shipped surface.
    """
    control_swap = _mean(control["folds"], "swap_violation_share")
    arm_swap = _mean(arm["folds"], "swap_violation_share")
    swap_delta = _paired(arm["folds"], control["folds"], "swap_violation_share")
    auc_delta = _paired(arm["folds"], control["folds"], "display_auc_mean")

    material_floor = None
    if control_swap is not None:
        material_floor = -MATERIAL_FALL_FRACTION * max(control_swap - SWAP_VIOLATION_LIMIT, 0.0)
    swap_beyond_noise = (
        swap_delta["mean"] is not None and swap_delta["se"] is not None and swap_delta["mean"] < -abs(swap_delta["se"])
    )
    swap_material = (
        material_floor is not None and swap_delta["mean"] is not None and swap_delta["mean"] <= material_floor
    )
    # One fold-level standard error of the paired difference. The clause once also named
    # the control's seed-to-seed sd; that spread was identically zero (EVAL-02).
    auc_floor = abs(auc_delta["se"] or 0.0)
    auc_holds = auc_delta["mean"] is not None and auc_delta["mean"] >= -auc_floor
    return {
        "control_swap_violation_share": control_swap,
        "arm_swap_violation_share": arm_swap,
        "swap_delta": swap_delta,
        "swap_material_floor": material_floor,
        "swap_falls_beyond_noise": bool(swap_beyond_noise),
        "swap_fall_is_material": bool(swap_material),
        "control_display_auc": _mean(control["folds"], "display_auc_mean"),
        "arm_display_auc": _mean(arm["folds"], "display_auc_mean"),
        "display_auc_delta": auc_delta,
        "display_auc_floor": auc_floor,
        "display_auc_holds": bool(auc_holds),
        "xi_only_swap_violation_share": _mean(diagnostic["folds"], "swap_violation_share"),
        "xi_only_display_auc": _mean(diagnostic["folds"], "display_auc_mean"),
        "ships": bool(swap_beyond_noise and swap_material and auc_holds),
    }


def run(frames_path: str, out: str) -> Dict[str, Any]:
    logger.info("%s", gates.describe(GATE_ID))
    player_frame, frame = load_frames(None, frames_path)
    logger.info("frames: %d matches, %d player rows", len(frame), len(player_frame))
    result: Dict[str, Any] = {
        "gate": gates.as_dict()[GATE_ID],
        "informing_gate": gates.as_dict()[SPREAD_GATE_ID],
        "swap_violation_limit": SWAP_VIOLATION_LIMIT,
        "material_fall_fraction": MATERIAL_FALL_FRACTION,
        "formats": {},
    }
    for format_code in C.FORMAT_CODES:
        started = time.perf_counter()
        format_frame = frame[frame.format_code == format_code]
        control = display_arm(format_frame, player_frame, format_code, constrain_team_context=False)
        arm = display_arm(format_frame, player_frame, format_code, constrain_team_context=True)
        without_spread = display_arm(
            format_frame,
            player_frame,
            format_code,
            columns=[column for column in PRE_B7_DISPLAY_COLS if column not in PELO_SPREAD_COLS],
        )
        diagnostic = xi_only_diagnostic(format_frame, player_frame, format_code)
        result["formats"][format_code] = {
            "control": control,
            "constrained": arm,
            "xi_only_diagnostic": diagnostic,
            "without_pelo_spread": without_spread,
            "feature_move_diagnostic": feature_move_diagnostic(format_frame, player_frame, format_code),
            "verdict": verdict(arm, control, diagnostic),
            "pelo_spread_reading": {
                "swap_violation_share": _mean(without_spread["folds"], "swap_violation_share"),
                "swap_delta": _paired(without_spread["folds"], control["folds"], "swap_violation_share"),
                "display_auc": _mean(without_spread["folds"], "display_auc_mean"),
                "display_auc_delta": _paired(without_spread["folds"], control["folds"], "display_auc_mean"),
            },
        }
        logger.info(
            "%s done in %.0f s: %s",
            format_code,
            time.perf_counter() - started,
            result["formats"][format_code]["verdict"]["ships"],
        )
    with open(out, "w") as fh:
        json.dump(result, fh, indent=2)
    logger.info("wrote %s", out)
    return result


def _f(value: Optional[float], pattern: str = "%.4f") -> str:
    return "—" if value is None else pattern % value


def decide(path: str) -> None:
    """Print the fold tables and the verdict, whatever it is."""
    with open(path) as fh:
        result = json.load(fh)
    gate = result["gate"]
    print(f"### {GATE_ID} — {gate['name']}\n")
    print(f"- **Varies:** {gate['varies']}")
    print(f"- **Fixed:** {gate['fixed']}")
    print(f"- **Decides:** {gate['decides']}\n")
    print(
        "| format | swap (control) | swap (constrained) | Δ ± se | material floor | display AUC (control) | Δ AUC ± se | AUC floor | verdict |"
    )
    print("|---|---:|---:|---|---:|---:|---|---:|---|")
    for format_code, node in result["formats"].items():
        v = node["verdict"]
        print(
            f"| {format_code} | {_f(v['control_swap_violation_share'])} | {_f(v['arm_swap_violation_share'])} | "
            f"{_f(v['swap_delta']['mean'], '%+.4f')} ± {_f(v['swap_delta']['se'])} | "
            f"{_f(v['swap_material_floor'], '%+.4f')} | {_f(v['control_display_auc'])} | "
            f"{_f(v['display_auc_delta']['mean'], '%+.4f')} ± {_f(v['display_auc_delta']['se'])} | "
            f"{_f(v['display_auc_floor'])} | "
            f"{'SHIPS' if v['ships'] else 'no'}"
            f"{'' if v['swap_falls_beyond_noise'] else '; swap fall inside noise'}"
            f"{'' if v['swap_fall_is_material'] else '; fall not material'}"
            f"{'' if v['display_auc_holds'] else '; AUC degrades'} |"
        )
    print("\n**Mechanism diagnostic — the same boosting model with no team context at all:**\n")
    print("| format | swap share (XI-only) | swap share (control, with context) | display AUC (XI-only) |")
    print("|---|---:|---:|---:|")
    for format_code, node in result["formats"].items():
        v = node["verdict"]
        print(
            f"| {format_code} | {_f(v['xi_only_swap_violation_share'])} | "
            f"{_f(v['control_swap_violation_share'])} | {_f(v['xi_only_display_auc'])} |"
        )
    print("\n**Feature-space diagnostic — what an upgrade does to the columns, no model involved:**\n")
    print(
        "| format | upgrades | share moving a *constrained* column against its direction | "
        "share moving an unconstrained column | worst offenders |"
    )
    print("|---|---:|---:|---:|---|")
    for format_code, node in result["formats"].items():
        d = node["feature_move_diagnostic"]
        worst = ", ".join(f"{c} {s:.3f}" for c, s in list(d.get("per_column_against_direction", {}).items())[:3])
        print(
            f"| {format_code} | {d['upgrades']} | "
            f"{_f(d.get('share_moving_a_constrained_column_against_its_direction'), '%.3f')} | "
            f"{_f(d.get('share_moving_an_unconstrained_column'), '%.3f')} | {worst or '—'} |"
        )
    informing = result["informing_gate"]
    print(f"\n### {SPREAD_GATE_ID} — {informing['name']} (informs; ships nothing here)\n")
    print(f"- **Varies:** {informing['varies']}")
    print(f"- **Fixed:** {informing['fixed']}")
    print(f"- **Decides:** {informing['decides']}\n")
    print("| format | swap (control) | swap (no pelo spread) | Δ ± se | display AUC (control) | Δ AUC ± se |")
    print("|---|---:|---:|---|---:|---|")
    for format_code, node in result["formats"].items():
        reading, v = node["pelo_spread_reading"], node["verdict"]
        print(
            f"| {format_code} | {_f(v['control_swap_violation_share'])} | {_f(reading['swap_violation_share'])} | "
            f"{_f(reading['swap_delta']['mean'], '%+.4f')} ± {_f(reading['swap_delta']['se'])} | "
            f"{_f(v['control_display_auc'])} | "
            f"{_f(reading['display_auc_delta']['mean'], '%+.4f')} ± {_f(reading['display_auc_delta']['se'])} |"
        )
    ships = [f for f, node in result["formats"].items() if node["verdict"]["ships"]]
    print(f"\n**Ships in:** {', '.join(ships) if ships else 'no format'}. The rule asks for every format.")


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--frames", help="cached frames (sim_frame_cache.py)")
    parser.add_argument("--out", default="b7_display_monotonicity.json")
    parser.add_argument("--decide", help="read a finished run and print its tables")
    args = parser.parse_args(argv)
    if args.decide:
        decide(args.decide)
        return 0
    if not args.frames:
        parser.error("--frames is required to run the gate")
    run(args.frames, args.out)
    decide(args.out)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
