"""Whether a run may be published: the manifest's ``usable`` verdict and what it rests on
(EVAL-04).

A retrain used to write every run as publishable. The run's own holdout report carried the
objective's AUC and nothing read it, so a run whose objective had stopped ranking -- a
broken feature join, an inverted label -- would be written, listed and served by the next
``reload`` exactly as a sound one. This module is the clause a run has to pass to be
published, evaluated at retrain time from the run's own report; ``runs.set_current``
refuses a run that failed it, so the refusal reaches ``reload`` as a 409 and the run that
was serving keeps serving.

Two clauses, per format the run scored:

* **The base rate.** A constant predictor ranks no pair and scores an AUC of 0.5 -- the
  base-rate equivalent for a ranking metric. An objective not above it does not rank, and
  a run whose objective does not rank in a format has nothing to select on there.
* **The served run, on the same holdout.** Only when the run ``current`` points at was
  trained at the same cutoff -- the same holdout window -- is its AUC the same measurement
  as this run's, and then this run may not fall short of it by more than the measurement's
  own noise. Across different cutoffs the two AUCs score different matches, and the
  harness's fold-to-fold spread (0.04-0.11, plan §8.1) dwarfs any margin a same-window
  comparison would use; so no comparison is made and nothing is pretended.

The margin is one standard error of the AUC on this run's holdout (Hanley & McNeil 1982),
computed from the counts the report already carries. It is the noise of the number itself:
the objective is a logistic regression with no seed, so a seed spread is zero by
construction rather than a floor (EVAL-02), and the harness's fold spread is between-window
variance. One standard error is the floor every gate in ``gates.py`` already uses. On the
one scored run on record (cutoff 2025-09-01) it is 0.013 in T20 (1,635 holdout rows),
0.036 T20I (182), 0.028 ODI (375) and 0.046 TEST (157).

What this gate cannot judge is a run with no holdout -- which is every run a scheduled
retrain at today's cutoff writes (B-3). Such a run is written usable, with ``format_notes``
saying it was not scored: the gate has nothing to read and says so rather than inventing a
number. Giving every retrain a number to read is a change to what a retrain scores, and
is not this module's.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Dict, Optional

from ml.xi.runs import RunManifest

#: A constant predictor's AUC: the base-rate equivalent for a ranking metric.
BASE_RATE_AUC = 0.5


@dataclass(frozen=True)
class Usability:
    usable: bool
    #: format code -> why the run is not usable there. Empty when it is usable.
    reasons: Dict[str, str]


def auc_standard_error(auc: float, n_positive: int, n_negative: int) -> float:
    """Hanley & McNeil (1982): the standard error of an AUC from its value and the two
    class counts it was scored over."""
    if n_positive < 1 or n_negative < 1:
        raise ValueError(f"an AUC needs both classes; got {n_positive} positive and {n_negative} negative rows")
    q1 = auc / (2.0 - auc)
    q2 = 2.0 * auc * auc / (1.0 + auc)
    variance = (auc * (1.0 - auc) + (n_positive - 1) * (q1 - auc * auc) + (n_negative - 1) * (q2 - auc * auc)) / (
        n_positive * n_negative
    )
    return math.sqrt(max(variance, 0.0))


def decide(summary: Dict, cutoff: str, served: Optional[RunManifest]) -> Usability:
    """The verdict on a run from its own report (``train_all``'s summary), against the run
    being served when the two scored the same holdout. A format the run did not score is
    not judged; ``format_notes`` says why it carries no number."""
    reasons: Dict[str, str] = {}
    for report in summary.get("formats", []):
        # The served reading -- marginalised over the toss -- is the one the manifest
        # quotes as ``objective_auc`` and the one the served run's is compared against, so
        # it is the one judged here; the toss-aware score beside it is not (EVAL-05).
        objective = report.get("objective_marginalised")
        if objective is None:
            continue
        format_code = report["format_code"]
        auc, n_holdout = float(objective["auc"]), int(report["n_holdout"])
        if auc <= BASE_RATE_AUC:
            reasons[format_code] = (
                f"objective holdout AUC {auc:.4f} is not above the base rate's {BASE_RATE_AUC} on {n_holdout} "
                "holdout rows: the objective does not rank"
            )
            continue
        regression = _regression_against_served(format_code, auc, report, cutoff, served)
        if regression is not None:
            reasons[format_code] = regression
    return Usability(usable=not reasons, reasons=reasons)


def _regression_against_served(
    format_code: str, auc: float, report: Dict, cutoff: str, served: Optional[RunManifest]
) -> Optional[str]:
    if served is None or served.cutoff != cutoff:
        return None
    previous = (served.metrics.get(format_code) or {}).get("objective_auc")
    if previous is None:
        return None
    n_holdout = int(report["n_holdout"])
    n_positive = round(float(report["holdout_positive_rate"]) * n_holdout)
    margin = auc_standard_error(auc, n_positive, n_holdout - n_positive)
    if auc < float(previous) - margin:
        return (
            f"objective holdout AUC {auc:.4f} is under the served run {served.run_id}'s {float(previous):.4f} on "
            f"the same holdout (cutoff {cutoff}, {n_holdout} rows) by more than one standard error of the AUC "
            f"({margin:.4f})"
        )
    return None
