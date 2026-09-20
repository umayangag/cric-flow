"""The usability gate a retrain writes into its manifest (ml.xi.run_usability, EVAL-04).

Before it, a run whose objective had stopped ranking was written, listed and published
exactly as a sound one. These tests pin the two clauses -- the base rate, and the served
run on the same holdout -- and what the gate declines to judge.
"""

from __future__ import annotations

from typing import Dict, Optional

import pytest

from ml.xi import run_usability
from ml.xi.runs import RunManifest


def _scored(
    format_code: str, auc: float, n_holdout: int = 400, positive_rate: float = 0.5, toss_aware_auc: float = 0.7
) -> Dict:
    """One scored format as ``train_all`` reports it: the served (marginalised) reading at
    ``auc``, and a toss-aware reading beside it that the gate must not read."""
    return {
        "format_code": format_code,
        "n_train": 4000,
        "n_holdout": n_holdout,
        "holdout_positive_rate": positive_rate,
        "objective_marginalised": {"auc": auc, "brier": 0.22},
        "objective_toss_aware": {"auc": toss_aware_auc, "brier": 0.22},
        "display_marginalised": {"auc": auc + 0.02, "brier": 0.22},
        "display_toss_aware": {"auc": toss_aware_auc + 0.02, "brier": 0.22},
    }


def _served(cutoff: str = "2025-09-01", metrics: Optional[Dict] = None) -> RunManifest:
    return RunManifest(
        run_id="20260902T102135Z-2818a6b7",
        created_at="2026-09-02T10:21:35+00:00",
        cutoff=cutoff,
        ratings_through="2025-08-31",
        dataset_sha="abc",
        git_sha="def",
        metrics=metrics if metrics is not None else {"T20": {"objective_auc": 0.72, "n_holdout": 400}},
    )


def test_an_objective_that_ranks_leaves_the_run_usable() -> None:
    verdict = run_usability.decide({"formats": [_scored("T20", 0.70)]}, "2025-09-01", served=None)

    assert verdict.usable is True
    assert verdict.reasons == {}


@pytest.mark.parametrize("auc", [0.5, 0.48, 0.31])
def test_an_objective_not_above_the_base_rate_makes_the_run_unusable(auc: float) -> None:
    """A constant predictor scores 0.5; at or under it the objective does not rank, and a
    run with nothing to select on in a format is not published."""
    verdict = run_usability.decide({"formats": [_scored("T20", auc)]}, "2025-09-01", served=None)

    assert verdict.usable is False
    assert verdict.reasons == {
        "T20": f"objective holdout AUC {auc:.4f} is not above the base rate's 0.5 on 400 holdout rows: "
        "the objective does not rank"
    }


def test_the_gate_judges_the_served_reading_not_the_toss_aware_one() -> None:
    """EVAL-05: the manifest quotes the marginalised AUC as ``objective_auc`` and compares
    it against the served run's, so that is the number the gate judges. A toss-aware
    reading under the base rate beside a served one above it leaves the run usable, and
    the reverse does not."""
    ranks_as_served = run_usability.decide(
        {"formats": [_scored("T20", 0.70, toss_aware_auc=0.45)]}, "2025-09-01", served=None
    )
    ranks_only_toss_aware = run_usability.decide(
        {"formats": [_scored("T20", 0.45, toss_aware_auc=0.70)]}, "2025-09-01", served=None
    )

    assert ranks_as_served.usable is True
    assert ranks_only_toss_aware.usable is False
    assert "0.4500 is not above the base rate" in ranks_only_toss_aware.reasons["T20"]


def test_a_format_the_run_did_not_score_is_not_judged() -> None:
    """A retrain at today's cutoff has no holdout (B-3): the gate has nothing to read and
    says nothing, and ``format_notes`` carries why the number is missing."""
    unscored = {"format_code": "T20", "n_train": 12000, "n_holdout": 0, "holdout_note": "holdout too small"}

    verdict = run_usability.decide({"formats": [unscored]}, "2026-09-13", served=_served())

    assert verdict.usable is True


def test_a_regression_against_the_served_run_on_the_same_holdout_makes_the_run_unusable() -> None:
    """Same cutoff, so the same holdout: falling under the served run's AUC by more than
    the AUC's own standard error is a regression, not noise."""
    verdict = run_usability.decide({"formats": [_scored("T20", 0.64)]}, "2025-09-01", served=_served())

    assert verdict.usable is False
    assert verdict.reasons["T20"].startswith(
        "objective holdout AUC 0.6400 is under the served run 20260902T102135Z-2818a6b7's 0.7200 on the same "
        "holdout (cutoff 2025-09-01, 400 rows) by more than one standard error of the AUC (0.0"
    )


def test_a_fall_inside_one_standard_error_is_not_a_regression() -> None:
    """The margin is the measurement's noise: 0.72 -> 0.71 on 400 rows (SE ~0.026) passes."""
    verdict = run_usability.decide({"formats": [_scored("T20", 0.71)]}, "2025-09-01", served=_served())

    assert verdict.usable is True


def test_a_served_run_at_another_cutoff_is_not_compared_against() -> None:
    """A different cutoff is a different holdout: the two AUCs score different matches, and
    the harness's fold spread says the difference between windows dwarfs any margin."""
    verdict = run_usability.decide(
        {"formats": [_scored("T20", 0.55)]}, "2025-12-01", served=_served(cutoff="2025-09-01")
    )

    assert verdict.usable is True


def test_a_served_run_with_no_number_for_the_format_is_not_compared_against() -> None:
    verdict = run_usability.decide(
        {"formats": [_scored("T20", 0.55)]}, "2025-09-01", served=_served(metrics={"T20": {"n_holdout": 0}})
    )

    assert verdict.usable is True


def test_the_margin_widens_with_a_smaller_holdout() -> None:
    """Hanley & McNeil: the same fall is a regression on 1,600 rows and noise on 60."""
    summary_large = {"formats": [_scored("T20", 0.69, n_holdout=1600)]}
    summary_small = {"formats": [_scored("T20", 0.69, n_holdout=60)]}

    large = run_usability.decide(summary_large, "2025-09-01", served=_served())
    small = run_usability.decide(summary_small, "2025-09-01", served=_served())

    assert large.usable is False
    assert small.usable is True


def test_auc_standard_error_matches_hanley_mcneil_on_the_record() -> None:
    """The first scored run's T20 holdout: AUC 0.7206 over 1,635 rows at a positive rate
    of 0.4826 -- 789 positive, 846 negative -- gives 0.0126."""
    assert run_usability.auc_standard_error(0.7206, 789, 846) == pytest.approx(0.0126, abs=0.0002)


def test_auc_standard_error_refuses_a_single_class() -> None:
    with pytest.raises(ValueError, match="both classes"):
        run_usability.auc_standard_error(0.7, 0, 100)
