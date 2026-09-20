"""What a run's manifest says about itself when the run had nothing to score (B-3).

A retrain at today's cutoff has no rows at or after it, so no format reports a holdout
AUC. The manifest used to answer that by omitting every format it had just trained --
`formats: []`, `metrics: {}` -- which is exactly the answer a run that trained *nothing*
gives, and the two are very different states to be in. These tests hold the manifest to
telling them apart, and to saying why.
"""

from __future__ import annotations

import json
import os
from typing import List

import pandas as pd
import pytest

from ml.xi import evaluate as evaluate_module
from ml.xi import retrain as retrain_module
from ml.xi import runs
from ml.xi import train as train_module
from ml.xi.builder import build
from ml.xi.retrain import format_notes, headline_metrics, retrain
from ml.xi.train import train_format
from tests.test_xi_train import synthetic_win_rows
from tests.xi_fixtures import ListSource, make_deliveries, make_match, xi

TEAM_ONE, TEAM_TWO = xi("a"), xi("b")


#: The one batter who decides the synthetic history below: he scores at three times the
#: rate, and the side he opens for wins.
STAR = "star"
STAR_RUNS_PER_BALL = 6


def _innings(batting: List[str], bowling: List[str], runs_per_ball: int, n_balls: int = 36) -> tuple:
    """One side's innings: the top six bat in turn and five bowlers share the overs, so
    enough players are involved for the performance model to have rows to fit on."""
    batters = [batting[i % 6] for i in range(n_balls)]
    bowlers = [bowling[5 + (i // 6) % 5] for i in range(n_balls)]
    runs = [STAR_RUNS_PER_BALL if batter == STAR else runs_per_ball for batter in batters]
    wickets = [1 if i % 12 == 11 else 0 for i in range(n_balls)]
    return batters, bowlers, runs, wickets


def _history(n_matches: int = 80) -> List:
    """A synthetic T20 history the objective can learn under the contract's signs: the
    star opens for one side or the other and the side he plays for wins, so the winner is
    the side with the higher batting impact and -- since his side always wins -- the
    higher Elo. Two wins in three go to A, so the label has both classes.

    It used to make the winner score at twice the rate on a fixed A, A, B cycle, which
    left the rating gap widest right before the weaker side won: only a fit reading Elo
    and batting impact *negatively* could rank that (the unconstrained objective did, at
    0.825 on the holdout), and one bound to the contract's signs correctly cannot (FEAT-14)."""
    matches = []
    for k in range(n_matches):
        winner = "A" if k % 3 else "B"
        team_one, team_two = list(TEAM_ONE), list(TEAM_TWO)
        (team_one if winner == "A" else team_two)[0] = STAR
        first = _innings(team_one, team_two, runs_per_ball=2)
        second = _innings(team_two, team_one, runs_per_ball=2)
        deliveries = make_deliveries(
            batters=first[0] + second[0],
            bowlers=first[1] + second[1],
            runs=first[2] + second[2],
            wickets=first[3] + second[3],
            innings=[0] * len(first[0]) + [1] * len(second[0]),
        )
        matches.append(make_match(f"m{k}", k, winner, team_one, team_two, deliveries))
    return matches


@pytest.fixture(scope="module")
def built():
    return build(ListSource(_history()))


def _written_manifest(built, tmp_path, cutoff: str) -> dict:
    written = retrain(built, str(tmp_path), pd.Timestamp(cutoff), formats=["T20"])
    return runs.read_manifest(written["run_dir"]).as_dict()


def test_the_run_report_scores_the_display_model_once(built, tmp_path) -> None:
    """EVAL-02: the run's own report carries one display fit's AUC and Brier -- the
    served model's -- and no seed list or spread across seeds; the manifest's headline
    is that number under its wire key."""
    written = retrain(built, str(tmp_path), pd.Timestamp("2024-02-20"), formats=["T20"])

    with open(os.path.join(written["run_dir"], "xi_win_report.json")) as fh:
        format_report = json.load(fh)["formats"][0]
    manifest = runs.read_manifest(written["run_dir"]).as_dict()
    assert "seeds" not in format_report
    assert set(format_report["display_marginalised"]) == {"auc", "brier"}
    assert manifest["metrics"]["T20"]["display_auc_mean"] == format_report["display_marginalised"]["auc"]


def test_the_manifest_records_the_iterations_the_display_model_ran(built, tmp_path) -> None:
    """EVAL-01: the manifest carries ``n_iter`` beside the grid's choice, and it is the
    ``max_iter`` the grid chose -- the record that would have shown an early-stopped fit."""
    manifest = _written_manifest(built, tmp_path, "2024-02-20")

    chosen = manifest["hyperparameters"]["T20"]
    assert chosen["n_iter"] == chosen["params"]["max_iter"]


def test_a_run_with_no_holdout_still_names_the_formats_it_trained(built, tmp_path) -> None:
    """B-3, the measured case: a cutoff after the last match trains everything and scores
    nothing. The manifest names the format, carries its row counts, and says why the
    discrimination numbers are missing."""
    manifest = _written_manifest(built, tmp_path, "2024-06-01")

    assert manifest["formats"] == ["T20"], "the run trained a format and the manifest must say so"
    assert manifest["metrics"]["T20"]["n_holdout"] == 0
    assert manifest["metrics"]["T20"]["n_train"] > 50
    assert "objective_auc" not in manifest["metrics"]["T20"], "there was no holdout to score on"
    assert "0 rows at or after the cutoff" in manifest["format_notes"]["T20"]
    assert manifest["format_notes"]["T20"].startswith("trained on ")


def test_a_run_that_trained_nothing_is_not_the_same_manifest(built, tmp_path) -> None:
    """The other half of the distinction: too little history to fit anything leaves the
    formats list empty, and the note says *not trained* rather than not scored."""
    manifest = _written_manifest(built, tmp_path, "2024-01-10")

    assert manifest["formats"] == [], "nothing was trained, so there is nothing to serve"
    assert manifest["metrics"] == {}
    assert manifest["format_notes"]["T20"].startswith("not trained: insufficient training rows")


def test_a_scored_run_carries_the_headline_numbers_and_no_note(built, tmp_path) -> None:
    """The ordinary case is unchanged: a cutoff that leaves a holdout reports both AUCs,
    and a format with nothing missing gets no note."""
    manifest = _written_manifest(built, tmp_path, "2024-02-20")

    assert manifest["formats"] == ["T20"]
    assert 0.0 <= manifest["metrics"]["T20"]["objective_auc"] <= 1.0
    assert 0.0 <= manifest["metrics"]["T20"]["display_auc_mean"] <= 1.0
    assert manifest["metrics"]["T20"]["n_holdout"] >= 20
    assert manifest["format_notes"] == {}


def test_headline_metrics_and_notes_read_the_report_not_the_models() -> None:
    """The unit form of the same three cases, so the shape is pinned without a fit."""
    summary = {
        "formats": [
            {
                "format_code": "T20",
                "n_train": 900,
                "n_holdout": 100,
                "objective_marginalised": {"auc": 0.72},
                "objective_toss_aware": {"auc": 0.75},
                "display_marginalised": {"auc": 0.71, "brier": 0.22},
                "display_toss_aware": {"auc": 0.74, "brier": 0.21},
            },
            {
                "format_code": "ODI",
                "n_train": 800,
                "n_holdout": 0,
                "holdout_note": "holdout too small or single-class; no discrimination numbers",
            },
            {"format_code": "TEST", "n_train": 12, "n_holdout": 0, "skipped_reason": "insufficient training rows"},
        ]
    }

    metrics, notes = headline_metrics(summary), format_notes(summary)

    assert sorted(metrics) == ["ODI", "T20"], "a trained format belongs in the manifest, scored or not"
    assert sorted(metrics["T20"]) == ["display_auc_mean", "n_holdout", "n_train", "objective_auc"]
    assert (metrics["T20"]["objective_auc"], metrics["T20"]["display_auc_mean"]) == (0.72, 0.71), "the served reading"
    assert sorted(metrics["ODI"]) == ["n_holdout", "n_train"]
    assert "T20" not in notes
    assert notes["ODI"] == (
        "trained on 800 rows but not scored: holdout too small or single-class; "
        "no discrimination numbers (0 rows at or after the cutoff)"
    )
    assert notes["TEST"] == "not trained: insufficient training rows (12 rows before the cutoff)"


# --- EVAL-06: the harness measures the display model the grid would ship ---------------

#: The grid's settings a fitted display model carries, so two fits can be compared by the
#: configuration they were made under rather than by their predictions.
GRID_SETTINGS = ("max_depth", "learning_rate", "max_iter")


def _grid_settings(model) -> dict:
    params = model.get_params()
    return {key: params[key] for key in GRID_SETTINGS}


def test_the_harness_fits_the_display_model_the_grid_would_ship(monkeypatch) -> None:
    """EVAL-06: the harness used to fit the display model at grid point 0 whatever the grid
    would have chosen, so a format whose retrain picked another point had walk-forward
    numbers for a model it never served. Under a grid whose incumbent is a one-stump model
    the grid must move, and the harness window must be fitted at the pick, record it under
    the manifest's key, and carry the same configuration ``train_format`` ships."""
    one_stump, incumbent = {"max_depth": 1, "learning_rate": 0.001, "max_iter": 1}, dict(train_module.DISPLAY_GRID[0])
    monkeypatch.setattr(train_module, "DISPLAY_GRID", (one_stump, incumbent))
    rows = synthetic_win_rows(400)
    cutoff, end = pd.Timestamp("2024-01-01"), rows.match_date.max() + pd.Timedelta(days=1)

    models, report = train_format(rows, "T20", cutoff)
    fold, _, harness_display = evaluate_module._evaluate_win_window(rows, cutoff, end)

    assert report["hyperparameters"]["params"] == incumbent, "the grid moved off its (crippled) incumbent"
    assert fold["hyperparameters"] == report["hyperparameters"], "the window records the pick the manifest records"
    assert _grid_settings(harness_display) == _grid_settings(models.display) == incumbent


# --- EVAL-05: one key, one quantity, whichever writer wrote it -------------------------


def test_the_manifest_headline_is_the_number_the_harness_reports_under_the_same_key() -> None:
    """EVAL-05: ``objective_auc`` and ``display_auc_mean`` reach the run manifest from
    ``train_format``'s report and the harness report from ``_evaluate_win_window``. On the
    same rows, the same cutoff and the same fits the two must be one number: the manifest
    used to score the actual batting order while the harness scored the served,
    toss-marginalised probability, so the same glossary key named two quantities. Both
    writers run the grid on the same training rows (EVAL-06), so nothing is pinned."""
    rows = synthetic_win_rows(400)
    cutoff, end = pd.Timestamp("2024-01-01"), rows.match_date.max() + pd.Timedelta(days=1)

    _, report = train_format(rows, "T20", cutoff)
    fold, _, _ = evaluate_module._evaluate_win_window(rows, cutoff, end)
    manifest_metrics = headline_metrics({"formats": [report]})["T20"]

    assert manifest_metrics["objective_auc"] == fold["objective_auc"]
    assert manifest_metrics["display_auc_mean"] == fold["display_auc_mean"]


def test_the_run_report_names_both_readings_of_each_model() -> None:
    """The toss-aware score stays in the report, under a name that says so, beside the
    served one the manifest quotes; nothing in the report is called plain ``objective``
    any more, because that name was the one that meant two things."""
    rows = synthetic_win_rows(400)

    _, report = train_format(rows, "T20", pd.Timestamp("2024-01-01"))

    assert "objective" not in report and "display" not in report
    for block in ("objective_marginalised", "objective_toss_aware", "display_marginalised", "display_toss_aware"):
        assert set(report[block]) == {"auc", "brier"}, block


# --- EVAL-04: a retrain refuses to publish a run whose objective does not rank ---------


def _summary_scoring(auc: float) -> dict:
    """What ``train_all`` reports for one scored format, at the AUC the test needs; the
    models it would have written are not needed to pin what the manifest says."""
    return {
        "data_quality_failures": [],
        "formats": [
            {
                "format_code": "T20",
                "n_train": 900,
                "n_holdout": 100,
                "holdout_positive_rate": 0.5,
                "objective_marginalised": {"auc": auc, "brier": 0.25},
                "objective_toss_aware": {"auc": auc, "brier": 0.25},
                "display_marginalised": {"auc": auc, "brier": 0.25},
                "display_toss_aware": {"auc": auc, "brier": 0.25},
                "hyperparameters": {"params": {"max_iter": 100}, "reason": "baseline"},
            }
        ],
    }


def test_a_scored_run_above_the_base_rate_is_usable(built, tmp_path) -> None:
    manifest = _written_manifest(built, tmp_path, "2024-02-20")

    assert manifest["usable"] is True
    assert manifest["unusable_reasons"] == {}


def test_a_run_whose_objective_does_not_rank_is_written_unusable_and_cannot_be_published(
    built, tmp_path, monkeypatch
) -> None:
    """The finding's retrain half: the run is on disk with its reasons, so the operator
    can read what it produced, and ``current`` refuses to point at it."""
    monkeypatch.setattr(retrain_module, "train_all", lambda *args, **kwargs: _summary_scoring(0.48))

    written = retrain(built, str(tmp_path), pd.Timestamp("2024-02-20"), formats=["T20"])
    manifest = runs.read_manifest(written["run_dir"])

    assert manifest.usable is False
    assert manifest.unusable_reasons == {
        "T20": "objective holdout AUC 0.4800 is not above the base rate's 0.5 on 100 holdout rows: "
        "the objective does not rank"
    }
    with pytest.raises(runs.RunArtifactsInvalid, match="not usable and cannot be published"):
        runs.set_current(str(tmp_path), written["run_id"])
    assert runs.newest_run_id(str(tmp_path)) is None, "a reload with no run named must not find it either"


def test_a_run_that_regressed_against_the_served_run_on_the_same_holdout_is_unusable(
    built, tmp_path, monkeypatch
) -> None:
    served = runs.RunManifest(
        run_id="20260902T102135Z-2818a6b7",
        created_at="2026-09-02T10:21:35+00:00",
        cutoff="2024-02-20",
        ratings_through="2024-02-19",
        dataset_sha="abc",
        git_sha="def",
        metrics={"T20": {"objective_auc": 0.80, "n_holdout": 100}},
    )
    monkeypatch.setattr(retrain_module, "train_all", lambda *args, **kwargs: _summary_scoring(0.65))

    written = retrain(built, str(tmp_path), pd.Timestamp("2024-02-20"), formats=["T20"], served=served)

    assert written["manifest"]["usable"] is False
    assert written["manifest"]["unusable_reasons"]["T20"].startswith(
        "objective holdout AUC 0.6500 is under the served run 20260902T102135Z-2818a6b7's 0.8000 on the same holdout"
    )


def test_retrain_main_exits_non_zero_on_an_unusable_run_and_still_records_the_quality_baseline(
    tmp_path, monkeypatch
) -> None:
    """The exit code says what the manifest says. The data-quality baseline is about the
    import and is recorded regardless: the two gates are independent."""
    from ml.xi import quality
    from tests.test_xi_data_quality import _tiny_cricsheet_dir

    monkeypatch.setattr(retrain_module, "train_all", lambda *args, **kwargs: _summary_scoring(0.48))
    src = _tiny_cricsheet_dir(tmp_path)
    out = tmp_path / "artifacts"

    rc = retrain_module.main(["--cricsheet-dir", str(src), "--cutoff", "2024-03-02", "--out", str(out)])

    assert rc == 1
    assert (out / quality.BASELINE_NAME).exists()
    assert runs.newest_run_id(str(out)) is None


def test_the_served_run_manifest_is_read_from_current_and_absent_when_nothing_is_published(tmp_path) -> None:
    assert retrain_module.served_run_manifest(str(tmp_path)) is None


def test_the_manifest_records_the_date_the_pass_consumed_through_beside_the_cutoff(built, tmp_path) -> None:
    """P2-2: ``ratings_through`` is the last match the pass consumed -- the state's own
    ``last_date`` -- and ``cutoff`` is the boundary the operator asked for. They are two
    dates, and a manifest that records only the second cannot say what its data is."""
    manifest = _written_manifest(built, tmp_path, "2024-06-01")

    assert manifest["ratings_through"] == built.state.last_date.isoformat()
    assert manifest["cutoff"] == "2024-06-01"
    assert manifest["ratings_through"] != manifest["cutoff"]


# --- EVAL-12: what it takes to build the run again ----------------------------------


def test_the_manifest_records_what_it_takes_to_build_the_run_again(built, tmp_path) -> None:
    """The provenance fields the audit found missing, on a run that actually wrote them:
    which source the pass read, what the dataset sha is a digest of, the versions the fit
    ran under, the win-model constants and the performance model's spec."""
    manifest = _written_manifest(built, tmp_path, "2024-02-20")

    assert manifest["source"] == "ListSource"
    assert manifest["dataset_digest"]["scheme"] == runs.DATASET_DIGEST_SCHEME
    assert manifest["dataset_digest"]["player_rows"] > 0
    assert manifest["library_versions"]["scikit-learn"]
    assert manifest["library_versions"]["python"]
    assert manifest["model_params"]["objective_C"] == train_module.OBJECTIVE_C
    assert manifest["model_params"]["display_fixed"] == train_module.DISPLAY_FIXED_PARAMS
    assert manifest["performance_spec"]["T20"]["n_features"] > 0


def test_the_win_model_constants_do_not_restate_the_grids_choice(built, tmp_path) -> None:
    """One enumeration each: ``hyperparameters`` is the only record of what the grid
    picked (EVAL-06), and ``model_params`` carries only the levers it never varies."""
    manifest = _written_manifest(built, tmp_path, "2024-02-20")

    chosen = manifest["hyperparameters"]["T20"]["params"]
    assert set(chosen) == {"max_depth", "learning_rate", "max_iter"}
    assert not set(manifest["model_params"]["display_fixed"]).intersection(chosen)


def test_the_manifest_quotes_the_performance_spec_the_report_recorded(built, tmp_path) -> None:
    """One computation quoted twice, not two that can disagree: the manifest's spec is
    the object the run's own report carries."""
    written = retrain(built, str(tmp_path), pd.Timestamp("2024-02-20"), formats=["T20"])

    with open(os.path.join(written["run_dir"], "xi_win_report.json")) as fh:
        reported = json.load(fh)["formats"][0]["performance"]["fit"]["spec"]
    assert written["manifest"]["performance_spec"]["T20"] == reported
