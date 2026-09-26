"""L4: the one temporal evaluation harness (H-19).

Rolling-origin walk-forward for every number a choice may be based on, plus the locked
window -- matches at or after ``LOCKED_START`` -- scored once per release and labeled as
such, never used for a choice. The locked window rotates once its data has guided a release
decision (A-4): the spent window retires into the folds and the line moves to the date of
the decision that read it, with both dates in the report so a number names its window.

The folds are the development surface and the locked window is the holdout, and the
report keeps the two apart by construction rather than by convention (EVAL-11). The
folds are scored from the development rows alone -- ``evaluate_format`` hands the fold
path a frame that ends at ``LOCKED_START``, so nothing computed in a fold can reach a
holdout row; ``fold_windows`` refuses a cutoff inside the holdout, so a rotation cannot
extend the folds into it and no experiment script can obtain a window there; and a gate
whose threshold would read the ``locked`` node is refused at registration
(``ml.xi.gates``). The holdout is a season (``LOCKED_SEASON_DAYS``) that accrues from the
line: its record beside its numbers says how much has accrued and that no gate read it,
and every fold summary carries how many gates consulted the folds (``ml.xi.folds``).
Per format it reports, with mean and spread over cutoffs:

* win-model objective and display AUC / Brier against the base rate;
* the specific-XI-beyond-typical-XI delta and swap monotonicity (the selection gates
  that replace P-0's winner accuracy), and the same swap probe run against the *display*
  surface -- reported, never a gate: H-4's 2 % line is a contract on the objective the
  optimiser reads, while the display model also reads team context nothing constrains
  (B-7);
* the best-single-column leak canary with the TEST-format control (H-2);
* the performance model (L2-B) on the player-match rows, per target and format: within-match
  Spearman, top-3 hit, the median's MAE, pinball loss, and 10-90 interval coverage beside
  its width (H-22), with the career-mean, career-quantile and rating-expectation baselines
  scored on the same unconditional population (H-20); quantile targets whose walk-forward
  coverage is off nominal are recalibrated on a temporal fold for the locked window (H-5);
* the simulator (L2-C, E2): simulated P(win) against the display model's (Brier,
  reliability), simulated totals' 10-90 coverage and width against actual innings totals,
  margins, latency; E2's display rule decided on the folds (``ml.xi.sim_harness``);
* the natural experiment for selection (E5, ``ml.xi.natural_experiment``), lineup-only:
  one side's consecutive matches 1-3 players apart, both elevens scored in the later
  fixture at its as-of, sign agreement with the result change against a bar derived from
  the objective's own claimed effect size -- per fold, pooled (the decision), and on the
  locked window; with the format's selection policy stated beside the verdict;
* the train/serve parity check (H-8): the last ``PARITY_LAST_N`` matches rebuilt from the
  as-of serving path and compared with the training frame -- rows, performance
  predictions and simulator outputs at a fixed seed alike;
* the market benchmark (X-4, ``ml.xi.market``): where closing odds have been cached, the
  market's de-vigged probability scored beside the display model on the matches both cover,
  per format, with the joined coverage stated beside every number. It informs and decides
  nothing, and no model anywhere reads odds as a feature.

Every gate the report prints is registered in ``ml.xi.gates`` with what it varies, what it
holds fixed and what decides (H-23); the report embeds the registry and fails if a gate is
printed without one. Every metric the report prints is explained in ``ml.xi.glossary``
(L-1); the report embeds the glossary, and a metric with no entry is a problem the report
carries rather than a number a reader has to guess at.

One command, one JSON report:

  python -m ml.xi.evaluate --cricsheet-dir <dir>     # offline reproduction
  python -m ml.xi.evaluate --postgres                # the go-app database
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import tempfile
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Callable, Dict, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd

from ml.xi import contract as C
from ml.xi import (
    display_regression,
    gates,
    glossary,
    market,
    natural_experiment,
    perf_baselines,
    perf_harness,
    selection_metrics,
    sim_harness,
    simulator,
)
from ml.xi.asof import round_trip_store, serving_parity
from ml.xi.builder import build
from ml.xi.folds import summarise_over_folds
from ml.xi.optimizer import OPTIMISED_SELECTION_FORMATS
from ml.xi.performance import PerformanceModels
from ml.xi.sources import MatchSource
from ml.xi.store import FormatModels
from ml.xi.train import _score_marginalised, fit_display_model_as_shipped, fit_objective_as_shipped

logger = logging.getLogger(__name__)

REPORT_NAME = "xi_evaluate_report.json"

# Rolling origins for every choice-facing number. Each fold trains on rows strictly before
# its cutoff and scores the window up to the next cutoff; the last window ends where the
# locked window begins. The entries from ``LOCKED_PREVIOUS_START`` on are the retired
# locked window, absorbed into the folds by the rotation below (A-4).
WALK_FORWARD_CUTOFFS: List[str] = [
    "2024-01-01",
    "2024-04-01",
    "2024-07-01",
    "2024-10-01",
    "2025-01-01",
    "2025-04-01",
    "2025-06-01",
    "2025-09-01",
    "2025-12-01",
    "2026-03-01",
    "2026-06-01",
]

# --- The locked window, and when it rotates (H-19, A-4) --------------------------------
#
# A locked window is spent the moment its data has guided a release decision: it is then a
# window whose numbers a choice has read, which is what H-19 exists to prevent. Rotation is
# the answer -- the spent window retires into the walk-forward folds above, and a new one is
# declared starting at the date of the decision that read it, the line no decision has read
# past. Both dates travel in the report so a reader can tell which window a number came
# from. The policy, and when the next rotation is due, is in docs/ml-and-training.md.

#: Matches at or after this date: scored once per release, labeled, never used for a choice.
#: The P-7 merge date -- every choice of the P-0..P-7 migration was made before it.
LOCKED_START = "2026-09-02"
#: When the window above was declared, and the window it replaced. Everything from
#: ``LOCKED_PREVIOUS_START`` to ``LOCKED_START`` is now scored as ordinary folds.
LOCKED_ROTATED_ON = "2026-09-02"
LOCKED_PREVIOUS_START = "2025-09-01"
LOCKED_ROTATION_REASON = (
    "every model choice of the P-0..P-7 migration consulted the >= 2025-09-01 window, so it is "
    "spent as an untouched holdout; it retires into the walk-forward folds and the line moves "
    "to the P-7 merge date, which no decision has read past (A-4)"
)
#: The holdout is a season, not the days since the last decision (EVAL-11): it accrues from
#: ``LOCKED_START`` for this many days of matches before a release verdict can rest on it.
#: A rotation before it completes is honest -- the window was spent -- but the report then
#: records that no season ever completed unread, rather than pretending one did.
LOCKED_SEASON_DAYS = 365
PARITY_LAST_N = 50
# How many evaluation matches feed the swap-monotonicity probe per fold.
SWAP_MAX_MATCHES = 50

MIN_TRAIN_ROWS = 50
MIN_EVAL_ROWS = 20


def fold_windows() -> List[Tuple[pd.Timestamp, pd.Timestamp]]:
    """The walk-forward windows, ending where the holdout begins. A cutoff at or past
    ``LOCKED_START`` is refused: a fold there would score holdout rows, which is the one
    thing the folds may never do (EVAL-11), and every experiment script takes its windows
    from here."""
    locked_start = pd.Timestamp(LOCKED_START)
    inside_holdout = [c for c in WALK_FORWARD_CUTOFFS if pd.Timestamp(c) >= locked_start]
    if inside_holdout:
        raise ValueError(
            f"walk-forward cutoffs {inside_holdout} sit at or past the holdout line {LOCKED_START}; "
            "the folds end where the holdout begins, and a rotation moves LOCKED_START before it extends them"
        )
    boundaries = [pd.Timestamp(c) for c in WALK_FORWARD_CUTOFFS] + [locked_start]
    return list(zip(boundaries[:-1], boundaries[1:]))


def locked_season_end() -> pd.Timestamp:
    """The date the holdout season is complete: ``LOCKED_SEASON_DAYS`` after the line."""
    return pd.Timestamp(LOCKED_START) + pd.Timedelta(days=LOCKED_SEASON_DAYS)


def locked_window() -> Dict[str, object]:
    """The locked window as the report carries it: where the line is now, when it was last
    moved, which window the folds absorbed when it moved (H-19, A-4), and the season it
    has to accrue before a release verdict can rest on it (EVAL-11)."""
    return {
        "start": LOCKED_START,
        "rotated_on": LOCKED_ROTATED_ON,
        "previous_start": LOCKED_PREVIOUS_START,
        "reason": LOCKED_ROTATION_REASON,
        "retired_into_folds": [c for c in WALK_FORWARD_CUTOFFS if c >= LOCKED_PREVIOUS_START],
        "season_days": LOCKED_SEASON_DAYS,
        "season_end": locked_season_end().date().isoformat(),
    }


def locked_note() -> str:
    """The label every locked-window number carries, naming the window it came from."""
    return (
        f"locked window from {LOCKED_START} (H-19): the holdout, scored once per release, never used for a "
        f"choice, read by no gate; rotated on {LOCKED_ROTATED_ON} from {LOCKED_PREVIOUS_START}, which is now "
        "in the folds"
    )


def holdout_record(format_frame: pd.DataFrame) -> Dict[str, object]:
    """What the holdout holds for one format, beside its numbers: its matches, how much of
    the season has accrued, and that no gate consulted it (EVAL-11). ``days_covered`` is
    measured from the matches present, not the clock, so a report says what it scored."""
    start, end = pd.Timestamp(LOCKED_START), locked_season_end()
    rows = format_frame[format_frame.match_date >= start]
    last = rows.match_date.max() if len(rows) else None
    days_covered = 0 if last is None else min(int((last - start).days) + 1, LOCKED_SEASON_DAYS)
    return {
        "n_matches": int(len(rows)),
        "first_match": None if last is None else rows.match_date.min().date().isoformat(),
        "last_match": None if last is None else last.date().isoformat(),
        "season_start": LOCKED_START,
        "season_end": end.date().isoformat(),
        "season_days": LOCKED_SEASON_DAYS,
        "days_covered": days_covered,
        "season_complete": days_covered >= LOCKED_SEASON_DAYS,
        "gates_consulted": 0,
    }


def _evaluate_win_window(
    format_frame: pd.DataFrame, cutoff: pd.Timestamp, end: pd.Timestamp
) -> Tuple[Optional[Dict], Optional[object], Optional[object]]:
    """Win-model metrics for one (train < cutoff, eval [cutoff, end)) split; also returns
    the fitted objective model for the selection metrics and the display model for the
    simulator's consistency check.

    Each model is fitted once. The display model's ``random_state`` is not a replicate
    source (EVAL-02: refits under other seeds were bit-identical below sklearn's 10k-row
    early-stopping threshold), so no seed spread is reported; the spread a difference is
    read against is the one over folds, in the summary."""
    train = format_frame[format_frame.match_date < cutoff]
    evaluation = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
    skip = {
        "cutoff": cutoff.date().isoformat(),
        "end": end.date().isoformat(),
        "n_train": int(len(train)),
        "n_eval": int(len(evaluation)),
    }
    if len(train) < MIN_TRAIN_ROWS or train[C.TARGET_COL].nunique() < 2:
        skip["skipped_reason"] = "insufficient training rows"
        return skip, None, None
    if len(evaluation) < MIN_EVAL_ROWS or evaluation[C.TARGET_COL].nunique() < 2:
        skip["skipped_reason"] = "evaluation window too small or single-class"
        return skip, None, None
    y_train = train[C.TARGET_COL].to_numpy(dtype=float)
    # Both grids run here as they run in a retrain, on this window's training rows, so the
    # models scored are the ones a run at this cutoff would ship (EVAL-06, and EVAL-13 for
    # the objective's); the window records each pick beside its numbers, under the
    # manifest's key.
    objective, objective_record = fit_objective_as_shipped(train)
    display, display_record = fit_display_model_as_shipped(train)
    hyperparameters = {"objective": objective_record, "display": display_record}
    objective_scores = _score_marginalised(objective, evaluation, C.XI_FEATURE_COLS)
    display_scores = _score_marginalised(display, evaluation, C.DISPLAY_FEATURE_COLS)
    y_eval = evaluation[C.TARGET_COL].to_numpy(dtype=float)
    base_rate_brier = float(np.mean((y_eval - y_train.mean()) ** 2))
    metrics = dict(skip)
    metrics.update(
        {
            "eval_positive_rate": float(y_eval.mean()),
            "train_positive_rate": float(y_train.mean()),
            "objective_auc": objective_scores["auc"],
            "objective_brier": objective_scores["brier"],
            # The keys are the wire names the report's readers share with the run
            # manifest and the market benchmark; each is the one display fit's score.
            "display_auc_mean": display_scores["auc"],
            "display_brier_mean": display_scores["brier"],
            "base_rate_brier": base_rate_brier,
            "hyperparameters": hyperparameters,
        }
    )
    return metrics, objective, display


@dataclass
class FoldOutcome:
    """One window's report and the fitted models later stages read.

    ``objective`` is what E5 scores the window's lineup pairs with, ``performance_model``
    is what H-8 serves through the as-of path, and ``display`` is the window's display
    model -- which the market benchmark scores its joined matches with, so the market is
    compared against the very model the fold reported and never a refitted lookalike.
    """

    report: Dict
    performance_model: Optional[PerformanceModels] = None
    objective: Optional[object] = None
    display: Optional[object] = None


def _evaluate_fold(
    format_code: str,
    format_frame: pd.DataFrame,
    player_frame: pd.DataFrame,
    cutoff: pd.Timestamp,
    end: pd.Timestamp,
    recalibrate: Tuple[str, ...] = (),
    benchmark: Optional[market.Benchmark] = None,
    window_label: str = "fold",
) -> FoldOutcome:
    """One window's report, its performance model and its fitted objective."""
    fold, objective, display = _evaluate_win_window(format_frame, cutoff, end)
    if objective is None:
        return FoldOutcome(report=fold)
    if benchmark is not None:
        benchmark.observe(
            format_code,
            window_label,
            cutoff,
            end,
            format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)],
            display,
        )
    format_players = player_frame[player_frame.format_code == format_code]
    window_players = format_players[(format_players.match_date >= cutoff) & (format_players.match_date < end)]
    window_matches = format_frame[(format_frame.match_date >= cutoff) & (format_frame.match_date < end)]
    fold["swap_monotonicity"] = selection_metrics.swap_monotonicity(
        objective, C.XI_FEATURE_COLS, window_players, format_code, max_matches=SWAP_MAX_MATCHES
    )
    # B-7: the same probe on the surface a person watches. It gates nothing.
    fold["display_swap_monotonicity"] = selection_metrics.display_swap_monotonicity(
        display, C.DISPLAY_FEATURE_COLS, window_matches, window_players, format_code, max_matches=SWAP_MAX_MATCHES
    )
    fold["specific_vs_typical"] = selection_metrics.specific_vs_typical(
        objective, C.XI_FEATURE_COLS, format_frame, cutoff, end
    )
    performance = perf_harness.evaluate_fold(player_frame, format_code, cutoff, end, recalibrate, format_frame)
    fold["performance"] = performance.report
    if performance.model is not None:
        fold["simulation"] = sim_harness.evaluate_window(
            performance.model, display, window_matches, window_players, format_code, fold["train_positive_rate"]
        )
    return FoldOutcome(report=fold, performance_model=performance.model, objective=objective, display=display)


def _summarize_folds(folds: List[Dict]) -> Dict:
    scored = [f for f in folds if "objective_auc" in f]

    def over_folds(path: Callable[[Dict], Optional[float]]) -> Optional[Dict]:
        return summarise_over_folds([path(f) for f in scored])

    def nested(fold: Dict, *keys: str) -> Optional[float]:
        node = fold
        for key in keys:
            if node is None:
                return None
            node = node.get(key)
        return node

    summary = {
        "objective_auc": over_folds(lambda f: f["objective_auc"]),
        "objective_brier": over_folds(lambda f: f["objective_brier"]),
        "display_auc": over_folds(lambda f: f["display_auc_mean"]),
        "base_rate_brier": over_folds(lambda f: f["base_rate_brier"]),
        "swap_violation_share": over_folds(lambda f: nested(f, "swap_monotonicity", "violation_share")),
        # H-4's probe per axis (FEAT-14), beside the combined share it is a gate on.
        "swap_violation_share_pelo_only": over_folds(
            lambda f: nested(f, "swap_monotonicity", "by_axis", "pelo_only", "violation_share")
        ),
        "swap_violation_share_rates_only": over_folds(
            lambda f: nested(f, "swap_monotonicity", "by_axis", "rates_only", "violation_share")
        ),
        "display_swap_violation_share": over_folds(lambda f: nested(f, "display_swap_monotonicity", "violation_share")),
        "specific_vs_typical_delta": over_folds(lambda f: nested(f, "specific_vs_typical", "delta")),
        "performance": perf_harness.summarize_folds(
            [f["performance"] for f in scored if "targets" in f.get("performance", {})]
        ),
        # Keyed by the fold's cutoff so the summary can name the windows that shipped
        # without a shared factor rather than only count them (B-12).
        "simulation": sim_harness.summarize_folds({f["cutoff"]: f.get("simulation") for f in scored}),
    }
    return summary


def _proba(objective: Optional[object]) -> Optional[natural_experiment.Proba]:
    if objective is None:
        return None
    return lambda x: objective.predict_proba(x)[:, 1]


def locked_recalibration(requested: Tuple[str, ...], model: Optional[PerformanceModels]) -> Dict[str, List[str]]:
    """What H-5 asked the locked window for, and what that window could deliver.

    The two differ when the window's calibration fold is too thin to carry a binned
    empirical quantile (``performance._fit_recalibration``) or when the window fitted no
    performance model at all. ``recalibrated_targets`` -- the gate's own report path --
    carries what was *applied*, so its number never claims a correction that was not made,
    and the shortfall is named beside it rather than being left in a log (plan 8.7)."""
    applied = sorted(model.calibration) if model is not None else []
    return {
        "recalibration_requested": list(requested),
        "recalibrated_targets": applied,
        "recalibration_skipped": sorted(set(requested) - set(applied)),
    }


@dataclass
class ParityModels:
    """What H-8 serves for one format, and which window fitted each: the win models the
    routes answer with and the performance model the simulator draws from. The locked
    window's when it fitted them, else the last fold's that did, so H-8 always has a
    model to serve while a freshly rotated window is still too small to score (A-4)."""

    win: Optional[FormatModels] = None
    win_window: Optional[str] = None
    performance: Optional[PerformanceModels] = None
    performance_window: Optional[str] = None


def _win_models(format_code: str, outcome: FoldOutcome) -> Optional[FormatModels]:
    """The window's fitted win models as the artifact ``retrain`` writes them, so the
    round trip serves the same shape a run does."""
    if outcome.objective is None:
        return None
    return FormatModels(
        format_code=format_code,
        objective=outcome.objective,
        display=outcome.display,
        objective_cols=list(C.XI_FEATURE_COLS),
        display_cols=list(C.DISPLAY_FEATURE_COLS),
        metadata={"window": outcome.report.get("cutoff")},
    )


def evaluate_format(
    format_code: str,
    frame: pd.DataFrame,
    player_frame: pd.DataFrame,
    pairs: Sequence[natural_experiment.LineupPair] = (),
    benchmark: Optional[market.Benchmark] = None,
) -> Tuple[Dict, ParityModels]:
    """The format's walk-forward folds and its locked window; also returns the models the
    parity check serves through the as-of path (``ParityModels``). ``pairs`` are E5's
    lineup pairs (all formats; filtered here), already carrying the previous eleven's
    as-of aggregates. ``benchmark``, when given, is handed each window's display models
    to score X-4's market arm beside them; it reads and decides nothing else."""
    format_frame = frame[frame.format_code == format_code]
    # EVAL-11: the folds are scored from the development rows alone. Nothing computed in a
    # fold can reach a holdout row because the frame it is handed ends at the line; only
    # the locked window's own call below sees the whole frame, and it trains on the rows
    # before the line under the same cutoff rule every fold obeys.
    locked_start = pd.Timestamp(LOCKED_START)
    development_frame = format_frame[format_frame.match_date < locked_start]
    development_players = player_frame[player_frame.match_date < locked_start]
    folds: List[Dict] = []
    fold_objectives: List[Tuple[pd.Timestamp, pd.Timestamp, Optional[natural_experiment.Proba]]] = []
    parity = ParityModels()
    for cutoff, end in fold_windows():
        outcome = _evaluate_fold(format_code, development_frame, development_players, cutoff, end, benchmark=benchmark)
        if outcome.performance_model is not None:
            parity.performance, parity.performance_window = outcome.performance_model, cutoff.date().isoformat()
        if outcome.objective is not None:
            parity.win, parity.win_window = _win_models(format_code, outcome), cutoff.date().isoformat()
        folds.append(outcome.report)
        fold_objectives.append((cutoff, end, _proba(outcome.objective)))
    summary = _summarize_folds(folds)
    # H-5: the folds, never the locked window, decide which quantiles get recalibrated.
    recalibrate = tuple(perf_harness.recalibration_needed(summary["performance"]))
    locked_outcome = _evaluate_fold(
        format_code,
        format_frame,
        player_frame,
        locked_start,
        pd.Timestamp.max,
        recalibrate,
        benchmark=benchmark,
        window_label="locked",
    )
    locked, locked_model = locked_outcome.report, locked_outcome.performance_model
    locked["note"] = locked_note()
    locked["holdout"] = holdout_record(format_frame)
    locked.update(locked_recalibration(recalibrate, locked_model))
    e5 = natural_experiment.evaluate_format(
        format_code,
        pairs,
        fold_objectives,
        (locked_start, _proba(locked_outcome.objective)),
        C.XI_FEATURE_COLS,
        served=format_code in OPTIMISED_SELECTION_FORMATS,
    )
    if locked_model is not None:
        parity.performance, parity.performance_window = locked_model, LOCKED_START
    if locked_outcome.objective is not None:
        parity.win, parity.win_window = _win_models(format_code, locked_outcome), LOCKED_START
    return {
        "n_matches": int(len(format_frame)),
        "walk_forward": {"folds": folds, "summary": summary},
        "locked": locked,
        # Which window fitted each model H-8 serves, so a parity number names its models.
        "parity_model_window": parity.performance_window,
        "parity_win_model_window": parity.win_window,
        # E2's rule, applied to the folds only; what the serving path does is the constant
        # ``simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED``, set from this by hand.
        "simulation_decision": {
            **sim_harness.decision(summary["simulation"]),
            "shared_factor": simulator.SHARED_FACTOR,
            "chase_orientation": simulator.CHASE_ORIENTATION,
            "served": simulator.SIMULATED_WIN_PROBABILITY_DISPLAYED.get(format_code, False),
        },
        # E5's rule, applied to the folds only; what the serving path does is the constant
        # ``optimizer.OPTIMISED_SELECTION_FORMATS``, set from this by hand (plan §8.8).
        "e5_lineup_only": e5,
        "selection_decision": e5["decision"],
    }, parity


def _market_benchmark(source: MatchSource, frame: pd.DataFrame, market_odds_dir: Optional[str]) -> market.Benchmark:
    """X-4's collector, loaded and joined before any fold is scored.

    The join runs once, here, so every fold reads one settled table rather than re-deciding
    which quote belongs to which match. An empty cache directory is not an error: the
    collector then reports no coverage, which is the honest state of a harness run with no
    odds beside it.
    """
    directory = market.cache_dir(market_odds_dir)
    quotes, load_counts = market.load_quotes(directory)
    if not quotes:
        return market.Benchmark(pd.DataFrame(columns=["match_id"]), load_counts, market.JoinCounts(), directory)
    joined, join_counts = market.join_to_matches(quotes, frame, source.team_key_for)
    return market.Benchmark(joined, load_counts, join_counts, directory)


def read_previous_report(path: str) -> Optional[Dict]:
    """The report a run is about to overwrite, for display-regression to read its previous
    accepted numbers from; None when there is none or it cannot be read. Both are logged
    and neither is a failure -- absent evidence decides nothing (the gate's node says so),
    but an unreadable file is an error worth seeing, so it is logged as one."""
    try:
        with open(path, encoding="utf-8") as fh:
            previous = json.load(fh)
    except FileNotFoundError:
        logger.info("display-regression: no previous report at %s; this run is the baseline", path)
        return None
    except (json.JSONDecodeError, OSError) as exc:
        logger.error(
            "display-regression: the previous report at %s is unreadable (%s); this run is the baseline", path, exc
        )
        return None
    if not isinstance(previous, dict):
        logger.error("display-regression: the previous report at %s is not a report; this run is the baseline", path)
        return None
    return previous


def _display_regression_node(
    format_code: str, node: Dict, frame: pd.DataFrame, generated_at: str, previous_report: Optional[Dict]
) -> Dict:
    """One format's display-regression verdict: this run's reference (its display summary,
    its fold windows, and the decided development rows per competition level) against the
    previous accepted run's, resolved from the report this one overwrites."""
    current = display_regression.Reference(
        generated_at=generated_at,
        display_auc=node["walk_forward"]["summary"].get("display_auc"),
        development_rows_by_level=display_regression.development_rows_by_level(
            frame[frame.format_code == format_code], LOCKED_START
        ),
        windows=display_regression.windows(WALK_FORWARD_CUTOFFS, LOCKED_START),
    )
    return display_regression.decide(current, display_regression.baseline_for(previous_report, format_code))


def evaluate(
    source: MatchSource,
    parity_source_factory: Callable[[], MatchSource],
    gender_split_context: bool = False,
    market_odds_dir: Optional[str] = None,
    previous_report: Optional[Dict] = None,
) -> Dict:
    """Run the harness over a source and return the report dict. ``previous_report`` is
    the report this run overwrites, when there is one: display-regression reads the
    previous accepted run's display AUC and row counts from it and nothing else does."""
    result = build(
        source,
        progress=lambda i: logger.info("rating pass: %d matches", i),
        gender_split_context=gender_split_context,
    )
    player_frame = perf_baselines.add_baseline_predictors(result.player_frame)
    dev_start, dev_end = pd.Timestamp(WALK_FORWARD_CUTOFFS[0]), pd.Timestamp(LOCKED_START)
    # The canary decides which columns get reviewed, so it reads development rows only.
    development_frame = result.frame[result.frame.match_date < dev_end]
    # E5's pairs, with the previous eleven read from the as-of serving path at the later
    # match's date: one advancing pass over the source, before any fold is scored.
    pairs = natural_experiment.build_pairs(result.frame, result.player_frame)
    logger.info("E5: reading %d previous elevens from the as-of serving path", len(pairs))
    # The pass this harness just ran is the authority on which arms are on, so every as-of
    # pass beside it is given that state's own flags rather than the ones this signature
    # happens to take (SERVE-07).
    state_flags = result.state.flags()
    previous_elevens = natural_experiment.score_previous_elevens(pairs, parity_source_factory(), state_flags)
    report = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "source": type(source).__name__,
        "cutoffs": list(WALK_FORWARD_CUTOFFS),
        "locked_start": LOCKED_START,
        "locked_window": locked_window(),
        "gender_split_context": gender_split_context,
        "n_rows": int(len(result.frame)),
        "n_player_rows": int(len(result.player_frame)),
        "data_quality": result.quality.as_dict(),
        "leak_canary": selection_metrics.leak_canary(development_frame, dev_start, dev_end),
        "e5_previous_elevens": previous_elevens,
        "formats": {},
    }
    # X-4: the market arm, joined once and scored inside each fold beside that fold's own
    # display models. It informs and decides nothing, and nothing else in the run reads it.
    benchmark = _market_benchmark(source, result.frame, market_odds_dir)
    win_models: Dict[str, FormatModels] = {}
    performance_models: Dict[str, PerformanceModels] = {}
    for format_code in C.FORMAT_CODES:
        logger.info("evaluating %s", format_code)
        report["formats"][format_code], parity_models = evaluate_format(
            format_code, result.frame, player_frame, pairs, benchmark=benchmark
        )
        # display-regression: this run's display AUC against the previous accepted run's,
        # judged only where the two scored the same population; the node carries the move,
        # both references and the like-for-like working either way (§8.7).
        report["formats"][format_code][display_regression.NODE] = _display_regression_node(
            format_code, report["formats"][format_code], result.frame, report["generated_at"], previous_report
        )
        if parity_models.win is not None:
            win_models[format_code] = parity_models.win
        if parity_models.performance is not None:
            performance_models[format_code] = parity_models.performance
    report["market_benchmark"] = benchmark.report(C.FORMAT_CODES)
    logger.info("serving parity (H-8): rebuilding the last %d matches from the as-of path", PARITY_LAST_N)
    # The models are served from a run directory written and loaded back the way retrain
    # and reload do it, never from the objects in memory (EVAL-10): a run this code
    # cannot serve is refused here by name (D-6), and the numbers compared are the ones
    # the routes answer with.
    with tempfile.TemporaryDirectory(prefix="h8-round-trip-") as directory:
        store = round_trip_store(result.state, win_models, performance_models, directory)
        report["serving_parity"] = serving_parity(
            parity_source_factory(),
            result.frame,
            result.player_frame,
            last_n=PARITY_LAST_N,
            state_flags=state_flags,
            store=store,
        )
    # H-23: the report carries every gate's varied / fixed / decides triple, and is checked
    # against the registry -- a gate printed without one is a defect of the report -- and
    # every standing gate's clause is evaluated on the number the report carries, so a
    # served format that has lost its evidence fails the run rather than passing (EVAL-04).
    report["gates"] = {"registry": gates.as_dict()}
    problems = gates.check_report(report)
    report["gates"].update({"passed": not problems, "problems": problems})
    # L-1: the report carries the glossary of every metric it prints, so the surfaces that
    # render it hold no metric prose of their own, and says so when it prints one it cannot
    # explain.
    report["glossary"] = {"entries": glossary.as_dict()}
    unexplained = glossary.check_report(report)
    report["glossary"].update({"passed": not unexplained, "problems": unexplained})
    return report


def _log_performance(format_code: str, performance: Optional[Dict]) -> None:
    """One line per headline target: the model against the career mean, over folds."""
    if not performance or "targets" not in performance:
        return
    for target, entry in performance["targets"].items():
        model, delta = entry["model"], entry["vs_career_mean"]
        if entry.get("headline") is not True or model.get("interval") is None or delta.get("spearman") is None:
            continue
        logger.info(
            "%-5s %-14s Spearman %.3f (vs career mean %+.3f) | pinball %.3f (%+.3f) | coverage %.3f width %.2f",
            format_code,
            target,
            model["within_match_spearman"]["mean"],
            delta["spearman"]["mean"],
            model["pinball"]["mean"],
            delta["pinball"]["mean"],
            model["interval"]["coverage_80"]["mean"],
            model["interval"]["width_80"]["mean"],
        )


def _log_market_benchmark(format_code: str, entry: Dict) -> None:
    """One line per format: the market against the display model, and over how much of
    the format the comparison was possible (X-4)."""
    pooled = entry.get("pooled")
    if not pooled:
        logger.info(
            "%-5s market benchmark (X-4): no joined closing prices (%d of %d matches covered)",
            format_code,
            entry["matches_joined"],
            entry["matches_in_windows"],
        )
        return
    logger.info(
        "%-5s market benchmark (X-4): market AUC %.3f vs display %.3f (toss-aware %.3f), "
        "Brier %.4f vs %.4f, over %d of %d matches (%.1f%%)",
        format_code,
        pooled["market_auc"],
        pooled["display_auc_mean"],
        pooled["display_toss_aware_auc"],
        pooled["market_brier"],
        pooled["display_brier_mean"],
        pooled["n"],
        entry["matches_in_windows"],
        100.0 * entry["joined_share"],
    )


def _parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    src = p.add_mutually_exclusive_group(required=True)
    src.add_argument("--cricsheet-dir", help="directory of Cricsheet JSON files")
    src.add_argument("--postgres", action="store_true", help="read the go-app database (POSTGRES_* env vars)")
    p.add_argument(
        "--birth-dates",
        default=None,
        help=(
            "archive path only: CSV of player_key,birth_date written by `python -m ml.xi.biography --export` "
            "(X-1b); without it every player's age reads as unknown"
        ),
    )
    p.add_argument("--out", default=None, help="report directory (default: ml.config.default_artifacts_dir())")
    p.add_argument(
        "--gender-split-context",
        action="store_true",
        help="E7 (H-7): split the context baselines (runs/wickets per format x over) by gender",
    )
    p.add_argument(
        "--market-odds-dir",
        default=None,
        help=(
            "X-4: directory of cached closing-odds CSVs for the market benchmark "
            f"(default: ${market.CACHE_DIR_ENV}, else {market.DEFAULT_CACHE_DIR}). "
            "Absent or empty, the report says the benchmark covered nothing."
        ),
    )
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    if args.cricsheet_dir:
        from ml.xi.sources import CricsheetJsonSource

        def source_factory() -> MatchSource:
            return CricsheetJsonSource(args.cricsheet_dir, birth_dates_path=args.birth_dates)

    else:
        from ml.db import get_db_connection
        from ml.xi.sources import PostgresSource

        connection = get_db_connection()

        def source_factory() -> MatchSource:
            return PostgresSource(connection)

    out_dir = args.out
    if out_dir is None:
        from ml.config import default_artifacts_dir

        out_dir = default_artifacts_dir()
    path = os.path.join(out_dir, REPORT_NAME)
    # Read before the run, not before the write: the previous report is the one this run
    # overwrites, and display-regression compares against it.
    previous_report = read_previous_report(path)
    report = evaluate(
        source_factory(),
        source_factory,
        gender_split_context=args.gender_split_context,
        market_odds_dir=args.market_odds_dir,
        previous_report=previous_report,
    )
    os.makedirs(out_dir, exist_ok=True)
    with open(path, "w") as fh:
        json.dump(report, fh, indent=2)
    logger.info("report written to %s", path)
    for format_code, entry in report["formats"].items():
        summary = entry["walk_forward"]["summary"]
        objective = summary.get("objective_auc")
        if objective:
            logger.info(
                "%-5s walk-forward objective AUC %.3f ± %.3f over %d folds (development surface, read by %d gates)",
                format_code,
                objective["mean"],
                objective["sd"],
                objective["n_folds"],
                objective["gates_consulted"],
            )
        holdout = entry["locked"].get("holdout", {})
        logger.info(
            "%-5s holdout from %s: %d matches, %d of %d season days accrued, read by no gate%s",
            format_code,
            LOCKED_START,
            holdout.get("n_matches", 0),
            holdout.get("days_covered", 0),
            LOCKED_SEASON_DAYS,
            "" if holdout.get("season_complete") else " -- the season is incomplete, a verdict on it is not due",
        )
        _log_performance(format_code, summary.get("performance"))
        decision = report["formats"][format_code]["simulation_decision"]
        if "delta_brier_mean" in decision:
            logger.info(
                "%-5s simulation (E2): Δ Brier %+.4f ± %.4f over %d folds -> simulated P(win) %s",
                format_code,
                decision["delta_brier_mean"],
                decision["delta_brier_sd"],
                decision["n_folds"],
                "a probability" if decision["simulated_win_probability_within_tolerance"] else "a description only",
            )
        logger.info(
            "%-5s selection (E5): %s", format_code, report["formats"][format_code]["selection_decision"]["reason"]
        )
        regression = entry.get(display_regression.NODE)
        if regression is not None:
            logger.info("%-5s display-regression: %s -- %s", format_code, regression["verdict"], regression["reason"])
        _log_market_benchmark(format_code, report["market_benchmark"]["formats"][format_code])
    if not report["serving_parity"]["passed"]:
        logger.error("serving parity (H-8) FAILED: %s", report["serving_parity"]["mismatches"][:5])
        return 1
    if not report["gates"]["passed"]:
        logger.error("gates (H-23 registry, standing thresholds) FAILED: %s", report["gates"]["problems"])
        return 1
    glossary_node = report.get("glossary", {})
    if not glossary_node.get("passed", True):
        logger.error("metric glossary (L-1) incomplete: %s", glossary_node["problems"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
