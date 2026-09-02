"""E3 -- can batting order be optimised? (plan §5, E3; P-7)

The gate, stated before it runs (H-23, `ml.xi.gates`):

* **varies** -- the order of the top seven batting slots of one eleven;
* **fixed** -- the eleven, the opponent eleven, the as-of date, batting first, and the random
  numbers (every order is simulated from the same seed, so the difference between two orders
  is not two draws' noise);
* **decides** -- the share of elevens whose best sampled order moves the *confirmed* simulated
  median first-innings total by more than 3 %; above 30 % of elevens, L3 gains a
  batting-order suggestion; else the number is recorded and order stays with the captain.

Method. For each sampled fixture in a window, each side's eleven is taken as fielded with its
as-of player rows. The expected-slot model gives the baseline order (``exp_bat_position``
ascending, the simulator's own). N random permutations of the top seven -- the identity
included -- each reassign the seven's slot *values* among the seven, so the feature set the
performance model reads is unchanged and only who holds which slot moves; the tail stays as
it is. L2-B is re-predicted under each counterfactual (the expected slot is one of its inputs)
and the first innings is simulated batting first with common random numbers. The best order by
search median is then **re-simulated with a fresh seed** beside the baseline, and that
confirmed delta is what is measured: the best of N noisy medians is biased upward by the
search itself (a winner's curse of about the search's Monte Carlo error times a few), and
the confirmation removes it. The same re-simulation of the baseline alone gives the noise
floor.

Windows. The decision runs on the development window -- fixtures in the last walk-forward
fold (2025-06-01 to 2025-09-01), the performance model fitted before its cutoff -- because
the locked window never decides anything (H-19). The locked window is scored beside it with
its own pre-locked fit, and labelled.

    python scripts/experiments/xi/e3_batting_order.py --postgres --out output/e3_batting_order.json
    python scripts/experiments/xi/e3_batting_order.py --cricsheet-dir data/go-app/cricsheet --cache frames.pkl
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import time
from dataclasses import asdict, dataclass
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "ml-service"))

from ml.xi import gates, perf_baselines, perf_harness, simulator  # noqa: E402
from ml.xi.builder import build  # noqa: E402
from ml.xi.evaluate import LOCKED_START, WALK_FORWARD_CUTOFFS  # noqa: E402
from ml.xi.performance import PerformanceModels  # noqa: E402

logger = logging.getLogger("e3_batting_order")

FORMATS = ("T20", "ODI")
TOP_SLOTS = 7
MOVES_THRESHOLD = 0.03  # a reordering "moves" the total when the confirmed median shifts by more than this
DECISION_SHARE = 0.30  # E3 adds the suggestion when more than this share of elevens move
DEVELOPMENT_WINDOW = (pd.Timestamp(WALK_FORWARD_CUTOFFS[-1]), pd.Timestamp(LOCKED_START))
LOCKED_WINDOW = (pd.Timestamp(LOCKED_START), pd.Timestamp.max)


@dataclass
class ElevenResult:
    match_id: Any
    side: int
    baseline_median: float
    best_search_median: float
    confirmed_baseline_median: float
    confirmed_best_median: float
    confirmed_delta: float  # (best - baseline) / baseline, both re-simulated with the confirmation seed
    noise_delta: float  # the baseline re-simulated under two seeds: the floor any delta must clear
    best_order: List[str]
    baseline_order: List[str]
    best_is_baseline: bool


def _first_innings_median(
    side: simulator.SideForecast,
    context: simulator.MatchContext,
    calibration: simulator.SimulatorCalibration,
    n: int,
    seed: int,
) -> float:
    """Median first-innings total batting first. Every call with the same seed consumes the
    same random numbers in the same order (factor first, then the innings), which is what
    makes two orders comparable draw for draw."""
    rng = np.random.default_rng(seed)
    factor = calibration.shared_factor.sample(rng, n) if calibration.shared_factor is not None else None
    draws = simulator.batting_innings(rng, side, context, n, calibration.runs_balls_rho, factor=factor)
    return float(np.median(draws.total))


def _permuted_rows(rows: pd.DataFrame, top: np.ndarray, permutations: Sequence[np.ndarray]) -> pd.DataFrame:
    """The side's rows repeated once per permutation, with the top seven's slot values
    reassigned among the seven in the permuted order. ``top`` holds the row positions of
    the baseline top seven in slot order; a permutation is an ordering of 0..6."""
    slot_values = np.sort(rows.exp_bat_position.to_numpy(dtype=float)[top])
    copies = []
    for perm in permutations:
        copy = rows.copy()
        positions = copy.exp_bat_position.to_numpy(dtype=float).copy()
        positions[top[perm]] = slot_values  # the player moved into slot j takes slot j's value
        copy["exp_bat_position"] = positions
        copies.append(copy)
    return pd.concat(copies, ignore_index=True)


def evaluate_eleven(
    model: PerformanceModels,
    rows: pd.DataFrame,
    context: simulator.MatchContext,
    match_id: Any,
    side_number: int,
    rng: np.random.Generator,
    n_permutations: int,
    search_draws: int,
    confirm_draws: int,
    seed: int,
) -> ElevenResult:
    rows = rows.reset_index(drop=True)
    k = len(rows)
    calibration = model.simulation or simulator.SimulatorCalibration(0.0)
    baseline_order = np.argsort(rows.exp_bat_position.to_numpy(dtype=float), kind="stable")
    top = baseline_order[:TOP_SLOTS]
    permutations = [np.arange(TOP_SLOTS)] + [rng.permutation(TOP_SLOTS) for _ in range(n_permutations - 1)]
    stacked = _permuted_rows(rows, top, permutations)
    prediction = model.predict_oriented(stacked, True)
    sides = [
        simulator.side_forecast(stacked, prediction, np.arange(j * k, (j + 1) * k)) for j in range(len(permutations))
    ]
    search = np.asarray([_first_innings_median(side, context, calibration, search_draws, seed) for side in sides])
    best = int(np.argmax(search))
    confirm_seed = seed + 1_000_003
    confirmed_baseline = _first_innings_median(sides[0], context, calibration, confirm_draws, confirm_seed)
    confirmed_best = _first_innings_median(sides[best], context, calibration, confirm_draws, confirm_seed)
    noise_baseline = _first_innings_median(sides[0], context, calibration, confirm_draws, confirm_seed + 7)
    keys = rows.player_key.to_numpy()
    return ElevenResult(
        match_id=match_id,
        side=side_number,
        baseline_median=float(search[0]),
        best_search_median=float(search[best]),
        confirmed_baseline_median=confirmed_baseline,
        confirmed_best_median=confirmed_best,
        confirmed_delta=(confirmed_best - confirmed_baseline) / max(confirmed_baseline, 1.0),
        noise_delta=(noise_baseline - confirmed_baseline) / max(confirmed_baseline, 1.0),
        best_order=[str(keys[i]) for i in sides[best].batting_order],
        baseline_order=[str(keys[i]) for i in baseline_order],
        best_is_baseline=best == 0,
    )


def _distribution(values: np.ndarray) -> Dict[str, Optional[float]]:
    if not len(values):
        return {"n": 0}
    q = np.quantile(values, [0.1, 0.5, 0.9])
    return {
        "n": int(len(values)),
        "mean": float(values.mean()),
        "p10": float(q[0]),
        "median": float(q[1]),
        "p90": float(q[2]),
        "max": float(values.max()),
    }


def evaluate_window(
    format_code: str,
    player_frame: pd.DataFrame,
    frame: pd.DataFrame,
    window: Tuple[pd.Timestamp, pd.Timestamp],
    label: str,
    n_fixtures: int,
    n_permutations: int,
    search_draws: int,
    confirm_draws: int,
    seed: int,
) -> Dict[str, Any]:
    cutoff, end = window
    started = time.perf_counter()
    fit = perf_harness.evaluate_fold(player_frame, format_code, cutoff, end, (), frame)
    if fit.model is None:
        return {"label": label, "skipped_reason": fit.report.get("skipped_reason")}
    logger.info(
        "%s %s: performance model fitted before %s in %.0f s",
        format_code,
        label,
        cutoff.date(),
        time.perf_counter() - started,
    )
    matches = frame[(frame.format_code == format_code) & (frame.match_date >= cutoff) & (frame.match_date < end)]
    rng = np.random.default_rng(seed)
    chosen = matches.sample(n=min(n_fixtures, len(matches)), random_state=int(rng.integers(2**31)))
    rows_by_match = player_frame[player_frame.match_id.isin(set(chosen.match_id))].groupby("match_id")
    results: List[ElevenResult] = []
    started = time.perf_counter()
    for i, win_row in enumerate(chosen.itertuples()):
        context = simulator.MatchContext.from_row(format_code, win_row)
        match_rows = rows_by_match.get_group(win_row.match_id)
        for side_number in (1, 2):
            side_rows = match_rows[match_rows.side == side_number]
            if len(side_rows) != 11:
                continue
            results.append(
                evaluate_eleven(
                    fit.model,
                    side_rows,
                    context,
                    win_row.match_id,
                    side_number,
                    rng,
                    n_permutations,
                    search_draws,
                    confirm_draws,
                    seed + 10_000 * i + side_number,
                )
            )
        if (i + 1) % 20 == 0:
            logger.info(
                "%s %s: %d fixtures, %d elevens, %.0f s",
                format_code,
                label,
                i + 1,
                len(results),
                time.perf_counter() - started,
            )
    deltas = np.asarray([r.confirmed_delta for r in results])
    search_gain = np.asarray(
        [(r.best_search_median - r.baseline_median) / max(r.baseline_median, 1.0) for r in results]
    )
    noise = np.abs(np.asarray([r.noise_delta for r in results]))
    moved = deltas > MOVES_THRESHOLD
    share = float(moved.mean()) if len(deltas) else None
    out = {
        "label": label,
        "cutoff": cutoff.date().isoformat(),
        "end": None if end == pd.Timestamp.max else end.date().isoformat(),
        "fixtures": int(len(chosen)),
        "elevens": int(len(results)),
        "permutations_per_eleven": n_permutations,
        "search_draws": search_draws,
        "confirm_draws": confirm_draws,
        "share_moved_over_3pct": share,
        "share_moved_over_3pct_se": float(np.sqrt(share * (1 - share) / len(deltas)))
        if share is not None and len(deltas)
        else None,
        "share_moved_over_1pct": float((deltas > 0.01).mean()) if len(deltas) else None,
        "share_moved_over_5pct": float((deltas > 0.05).mean()) if len(deltas) else None,
        "share_best_is_baseline": float(np.mean([r.best_is_baseline for r in results])) if results else None,
        "confirmed_delta": _distribution(deltas),
        "search_delta_unconfirmed": _distribution(search_gain),
        "noise_floor_abs_delta": _distribution(noise),
        "baseline_median_total": _distribution(np.asarray([r.confirmed_baseline_median for r in results])),
        "elevens_detail": [asdict(r) for r in results],
    }
    logger.info(
        "%s %s: %d elevens; confirmed delta median %.3f p90 %.3f; share > 3%%: %.3f (search-only share %.3f); noise floor p90 %.3f",
        format_code,
        label,
        len(results),
        out["confirmed_delta"].get("median", float("nan")),
        out["confirmed_delta"].get("p90", float("nan")),
        share if share is not None else float("nan"),
        float((search_gain > MOVES_THRESHOLD).mean()) if len(search_gain) else float("nan"),
        out["noise_floor_abs_delta"].get("p90", float("nan")),
    )
    return out


def load_frames(args) -> Tuple[pd.DataFrame, pd.DataFrame]:
    if args.cache and os.path.exists(args.cache):
        logger.info("frames from cache %s", args.cache)
        return pd.read_pickle(args.cache)
    if args.cricsheet_dir:
        from ml.xi.sources import CricsheetJsonSource
        from ml.xi.train import _international_teams_from_config

        source = CricsheetJsonSource(args.cricsheet_dir, _international_teams_from_config())
    else:
        from ml.db import get_db_connection
        from ml.xi.sources import PostgresSource

        source = PostgresSource(get_db_connection())
    result = build(source, progress=lambda i: logger.info("rating pass: %d matches", i))
    frames = (perf_baselines.add_baseline_predictors(result.player_frame), result.frame)
    if args.cache:
        pd.to_pickle(frames, args.cache)
    return frames


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    src = p.add_mutually_exclusive_group(required=True)
    src.add_argument("--postgres", action="store_true")
    src.add_argument("--cricsheet-dir")
    p.add_argument("--cache", default=None, help="pickle of (player_frame, frame) to reuse across runs")
    p.add_argument("--out", default="output/e3_batting_order.json")
    p.add_argument("--fixtures", type=int, default=100, help="fixtures sampled per format and window")
    p.add_argument("--permutations", type=int, default=64, help="N: sampled orders of the top seven, identity included")
    p.add_argument("--search-draws", type=int, default=1000)
    p.add_argument("--confirm-draws", type=int, default=4000)
    p.add_argument("--seed", type=int, default=20260902)
    args = p.parse_args()

    # H-23: the triple is stated before the gate runs.
    logger.info("gate: %s", gates.describe("E3"))
    player_frame, frame = load_frames(args)
    report: Dict[str, Any] = {
        "gate": asdict(gates.REGISTRY["E3"]),
        "top_slots": TOP_SLOTS,
        "moves_threshold": MOVES_THRESHOLD,
        "decision_share": DECISION_SHARE,
        "seed": args.seed,
        "formats": {},
    }
    for format_code in FORMATS:
        development = evaluate_window(
            format_code,
            player_frame,
            frame,
            DEVELOPMENT_WINDOW,
            "development (decides)",
            args.fixtures,
            args.permutations,
            args.search_draws,
            args.confirm_draws,
            args.seed,
        )
        locked = evaluate_window(
            format_code,
            player_frame,
            frame,
            LOCKED_WINDOW,
            "locked window (H-19: labelled, never decides)",
            args.fixtures,
            args.permutations,
            args.search_draws,
            args.confirm_draws,
            args.seed + 1,
        )
        share = development.get("share_moved_over_3pct")
        report["formats"][format_code] = {
            "development": development,
            "locked": locked,
            "decision": {
                "share_moved_over_3pct": share,
                "adds_batting_order_suggestion": None if share is None else bool(share > DECISION_SHARE),
                "reason": (
                    "no development elevens could be scored"
                    if share is None
                    else f"{share:.3f} of development elevens move by more than {MOVES_THRESHOLD:.0%} against the "
                    f"{DECISION_SHARE:.0%} line: {'add the suggestion to L3' if share > DECISION_SHARE else 'order stays with the captain'}"
                ),
            },
        }
        logger.info("%s E3: %s", format_code, report["formats"][format_code]["decision"]["reason"])
    os.makedirs(os.path.dirname(args.out) or ".", exist_ok=True)
    with open(args.out, "w") as fh:
        json.dump(report, fh, indent=2, default=str)
    logger.info("written to %s", args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
