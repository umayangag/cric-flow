"""Unit tests for the metric glossary and its completeness gate (ml.xi.glossary, L-1)."""

from __future__ import annotations

import pytest

from ml.xi import glossary, retrain


def _report_with_two_metrics() -> dict:
    """The shapes a metric takes in the report: a bare number and a fold summary."""
    return {
        "formats": {
            "T20": {
                "walk_forward": {
                    "folds": [{"n_train": 900, "objective_auc": 0.71}],
                    "summary": {"objective_auc": {"mean": 0.72, "sd": 0.01, "n_folds": 2}},
                }
            }
        }
    }


def test_every_entry_declares_the_copy_a_reader_needs() -> None:
    for metric in glossary.METRICS:
        assert metric.name.strip(), metric.key
        assert metric.explanation.strip(), metric.key
        assert metric.band.strip(), metric.key
        assert metric.better.strip(), metric.key


def test_the_glossary_explains_no_spread_across_seeds() -> None:
    """EVAL-02: the display model is one fit, so there is no seed spread to explain and
    the entries that called one a noise floor are gone; what remains says the number is
    the one fit's score."""
    assert "display_auc_seed_sd" not in glossary.REGISTRY
    assert "display_auc_seed_sd_mean" not in glossary.REGISTRY
    assert "seed" not in glossary.REGISTRY["display_auc_mean"].name.lower()
    assert "one display model" in glossary.REGISTRY["display_auc_mean"].explanation


def test_no_key_is_both_a_metric_and_a_declared_non_metric() -> None:
    assert set(glossary.REGISTRY) & set(glossary.NON_METRIC_KEYS) == set()


def test_check_report_passes_a_report_whose_every_metric_is_explained() -> None:
    assert glossary.check_report(_report_with_two_metrics()) == []


def test_check_report_fails_on_a_metric_with_no_glossary_entry() -> None:
    """The gate that stops a new metric shipping unexplained."""
    report = _report_with_two_metrics()
    report["formats"]["T20"]["walk_forward"]["summary"]["brand_new_score"] = 0.42

    problems = glossary.check_report(report)

    assert problems == ["metric 'brand_new_score' is reported with no glossary entry"]


def test_check_report_fails_an_entry_that_declares_no_copy(monkeypatch) -> None:
    """An entry with an empty band explains nothing; the gate treats it as a gap, not as
    an entry, so a placeholder cannot pass for an explanation."""
    placeholder = glossary.Metric(key="objective_auc", name="Objective AUC", explanation="x", band="  ", better="")
    monkeypatch.setattr(glossary, "METRICS", (placeholder,))

    problems = glossary.check_report(_report_with_two_metrics())

    assert "metric objective_auc declares no 'band'" in problems
    assert "metric objective_auc declares no 'better'" in problems


def test_check_report_notices_an_embedded_glossary_that_drifted_from_the_code() -> None:
    report = _report_with_two_metrics()
    report["glossary"] = {"entries": {"objective_auc": {}}}

    problems = glossary.check_report(report)

    assert problems == ["the report's embedded glossary does not match the code's"]


@pytest.mark.parametrize(
    "node, expected",
    [
        ({"width_80": 12.0}, ["width_80"]),
        ({"pit_deciles": [0.1, 0.1]}, ["pit_deciles"]),
        ({"n_eval": 40, "fit": {"iterations": {"runs": 120}}}, []),
        ({"interval": {"coverage_80": 0.8, "q10": {"strict": 0.0, "inclusive": 0.11}}}, ["coverage_80", "q10"]),
    ],
    ids=["a number", "a list of numbers", "declared non-metrics", "an entry ends the walk at its node"],
)
def test_metric_keys_reads_the_shapes_a_metric_takes(node: dict, expected: list) -> None:
    assert glossary.metric_keys(node) == expected


def test_metric_keys_names_each_key_once_however_often_it_is_reported() -> None:
    report = {"folds": [{"objective_auc": 0.7}, {"objective_auc": 0.8}]}

    assert glossary.metric_keys(report) == ["objective_auc"]


def test_run_manifest_headline_metrics_are_glossaried() -> None:
    """The manifest's headline metrics are rendered by key on the Workbench, so they are
    held to the same gate as the report's."""
    summary = {
        "formats": [
            {
                "format_code": "T20",
                "n_train": 900,
                "n_holdout": 100,
                "objective_marginalised": {"auc": 0.72},
                "display_marginalised": {"auc": 0.71, "brier": 0.22},
            }
        ]
    }
    keys = sorted(retrain.headline_metrics(summary)["T20"])

    assert glossary.check_metric_names(keys, "the run manifest") == []


def test_check_metric_names_names_the_source_of_an_unexplained_key() -> None:
    problems = glossary.check_metric_names(["objective_auc", "mystery_score"], "the run manifest")

    assert problems == ["the run manifest reports 'mystery_score' with no glossary entry"]


def test_every_entry_carries_the_direction_in_one_word_beside_the_sentence() -> None:
    """A surface that colours a change reads `direction`; one that prints a line reads
    `better`. Deriving the first from the second is what stops them disagreeing."""
    served = glossary.as_dict()

    assert served["objective_auc"]["direction"] == "higher"
    assert served["pinball"]["direction"] == "lower"
    assert served["coverage_80"]["direction"] == "nominal"
    assert served["max_abs_difference"]["direction"] == "exact"
    assert served["spread_share"]["direction"] == "none"
    assert all(entry["direction"] in {"higher", "lower", "nominal", "exact", "none"} for entry in served.values())


def test_describe_states_the_band_and_the_direction() -> None:
    line = glossary.describe("swap_violation_share")

    assert line.startswith("swap_violation_share (Swap violations)")
    assert "Under 2 % passes (H-4)" in line
    assert line.endswith("(lower is better)")


def test_the_display_surfaces_swap_share_states_the_zero_and_what_it_cost() -> None:
    """B-7: the surface reads 0.0000 by construction now, and the entry has to say both
    that H-4's line still is not what binds it and that display AUC paid for the zero --
    or a reader takes a free win from a trade that cost 0.005 in two formats."""
    line = glossary.describe("display_swap_violation_share")

    assert line.startswith("display_swap_violation_share (Swap violations, display surface)")
    assert "0.0000 in every format" in line
    assert "not a gate" in line
    assert "-0.0055 ODI" in line, "the cost is stated where the number is read"


def test_a_scale_never_points_the_opposite_way_from_the_direction_it_is_read_with() -> None:
    """The colour and the sentence come from one entry, so an anchor pair that ran the
    wrong way would paint a bad number green while the popover said the opposite."""
    monotone = {"higher": lambda scale: scale.good > scale.bad, "lower": lambda scale: scale.good < scale.bad}

    wrong = [
        metric.key
        for metric in glossary.METRICS
        if metric.scale is not None and metric.direction in monotone and not monotone[metric.direction](metric.scale)
    ]

    assert wrong == []


def test_a_metric_with_no_good_direction_carries_no_scale() -> None:
    """`width_80` is the case the rule exists for: narrower is progress only when
    coverage holds, so painting it any colour on its own states something untrue."""
    unreadable = [metric.key for metric in glossary.METRICS if metric.direction == "none" and metric.scale is not None]

    assert unreadable == []


def test_the_served_scale_is_two_plain_numbers_a_surface_can_interpolate() -> None:
    served = glossary.as_dict()

    assert served["objective_auc"]["scale"] == {"bad": 0.50, "good": 0.75}
    assert served["coverage_80"]["scale"] == {"bad": 0.65, "good": 0.80}
    assert served["max_abs_difference"]["scale"] == {"bad": 0.0, "good": 0.0}
    assert served["width_80"]["scale"] is None
