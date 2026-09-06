"""One step from raw data to a named, loadable run (§9.3).

    python -m ml.xi.retrain --postgres --cutoff 2025-09-01

It is the pipeline's only training step. What used to be precompute, export and six
training commands -- each of which could be run in an order that produced artifacts
nothing had measured -- is this:

    rating pass -> XI win models (with the grid) -> performance models -> report -> manifest

Everything it writes goes into ``runs/<run_id>/``, and the manifest is written last, so a
directory only becomes a run once every artifact it names is on disk (H-16). Nothing here
publishes: ``current`` is moved by ``reload``, which is a separate step because "build a
run" and "serve that run" are different decisions and a retrain that published itself
would leave no way back to the run before it.

The L4 harness (``ml.xi.evaluate``) is the ``evaluate`` step, not part of this one. Its
seven rolling origins refit every model per fold per format; folding that into every
retrain would put a full pipeline out of reach of any sensible cadence. What a retrain
records is the run's own holdout report -- the numbers the models it just fitted actually
produced -- and the manifest quotes their headline.
"""

from __future__ import annotations

import argparse
import logging
import os
import sys
from datetime import date, datetime, timezone
from typing import Dict, List, Optional, Sequence

import pandas as pd

from ml.xi import contract as C
from ml.xi import glossary, runs
from ml.xi.builder import BuildResult, build
from ml.xi.store import state_shape
from ml.xi.train import REPORT_NAME, train_all

logger = logging.getLogger(__name__)


def headline_metrics(summary: Dict) -> Dict[str, Dict[str, float]]:
    """The numbers the manifest carries per format, from the run's own report.

    One entry per format the run *trained*, whether or not it had a holdout to score on.
    That is B-3: a retrain at today's cutoff has no rows at or after it, so no format
    reported an ``objective`` block, and skipping those left ``formats: []`` and
    ``metrics: {}`` -- a manifest indistinguishable from one whose run trained nothing,
    for a run that had in fact written and served four formats' models.

    The scored numbers are held to the two that answer "is this run usable?": the
    objective's holdout AUC, which H-17 scopes selection by, and the display model's,
    which is what a user is shown. A format with no holdout carries its row counts alone,
    and ``format_notes`` says why the rest is missing. The rest of the report stays in the
    report -- a manifest that copies everything is a second copy to fall out of step.

    The names are glossary keys (``ml.xi.glossary``), because the Workbench renders these
    metrics by key and looks their explanation up: a headline metric named anything else
    would reach a surface with nothing to say about itself.
    """
    out: Dict[str, Dict[str, float]] = {}
    for report in summary.get("formats", []):
        if "skipped_reason" in report:
            continue  # no model was fitted, so there is nothing for this format to be usable as
        entry: Dict[str, float] = {"n_train": report["n_train"], "n_holdout": report["n_holdout"]}
        if "objective" in report:
            entry["objective_auc"] = report["objective"]["auc"]
            entry["display_auc_mean"] = report["display"]["auc_mean"]
        out[report["format_code"]] = entry
    return out


def format_notes(summary: Dict) -> Dict[str, str]:
    """Why a format carries no holdout metrics, per format, in the run's own words (B-3).

    An absence in ``metrics`` has two very different causes -- nothing was trained, or
    something was trained and there was nothing to score it on -- and a manifest that
    renders the same either way cannot answer "is this run usable?". §8.7: the reason is
    recorded where the answer is read, not only in the training log.

    The wording is the report's own (``skipped_reason`` / ``holdout_note``) plus the row
    counts, so there is one place that decides why a format was skipped and this only
    quotes it. A format that trained and scored gets no note: there is nothing missing.
    """
    notes: Dict[str, str] = {}
    for report in summary.get("formats", []):
        if "skipped_reason" in report:
            notes[report["format_code"]] = (
                f"not trained: {report['skipped_reason']} ({report['n_train']} rows before the cutoff)"
            )
        elif "objective" not in report:
            notes[report["format_code"]] = (
                f"trained on {report['n_train']} rows but not scored: "
                f"{report.get('holdout_note', 'no holdout metrics were reported')} "
                f"({report['n_holdout']} rows at or after the cutoff)"
            )
    return notes


def chosen_hyperparameters(summary: Dict) -> Dict[str, Dict]:
    """What the grid picked per format, and why (§9.3)."""
    return {
        report["format_code"]: report["hyperparameters"]
        for report in summary.get("formats", [])
        if report.get("hyperparameters")
    }


def retrain(
    result: BuildResult,
    artifacts_dir: str,
    cutoff: pd.Timestamp,
    formats: Sequence[str] = C.FORMAT_CODES,
    run_id: Optional[str] = None,
) -> Dict:
    """Write one run from a completed rating pass, and return its manifest as a dict."""
    run_id = run_id or runs.new_run_id()
    directory = runs.run_dir(artifacts_dir, run_id)
    os.makedirs(directory, exist_ok=True)
    logger.info("retrain: run %s -> %s", run_id, directory)

    summary = train_all(result, directory, cutoff, formats, baseline_dir=artifacts_dir)
    metrics = headline_metrics(summary)
    notes = format_notes(summary)
    # L-1: the Workbench renders these by key and looks each one up, so a headline metric
    # the glossary does not carry would reach a surface with nothing to say about itself.
    unexplained = glossary.check_metric_names(
        sorted({key for per_format in metrics.values() for key in per_format}), "the run manifest"
    )
    if unexplained:
        logger.error("retrain: %s", "; ".join(unexplained))

    manifest = runs.RunManifest(
        run_id=run_id,
        created_at=datetime.now(timezone.utc).isoformat(),
        cutoff=cutoff.date().isoformat(),
        dataset_sha=runs.dataset_sha(_match_keys(result)),
        git_sha=runs.git_sha(),
        rating_params={
            "decay_per_match": C.DECAY_PER_MATCH,
            "prior_balls": C.PRIOR_BALLS,
            "k_team_elo": C.K_TEAM_ELO,
            "k_player_elo": C.K_PLAYER_ELO,
            "gender_split_context": result.state.gender_split_context,
            "age_aware_cold_start": result.state.age_aware_cold_start,
        },
        hyperparameters=chosen_hyperparameters(summary),
        metrics=metrics,
        state_shape=state_shape(result.state),
        # The formats this run trained -- not the formats it managed to score. A run whose
        # cutoff leaves no holdout still wrote and can still serve these models (B-3).
        formats=sorted(metrics),
        format_notes=notes,
        report=REPORT_NAME,
    )
    runs.write_manifest(directory, manifest)
    logger.info(
        "retrain: run %s written -- %d players, formats %s, dataset %s",
        run_id,
        manifest.state_shape["players"],
        manifest.formats,
        manifest.dataset_sha[:12],
    )
    for format_code, note in sorted(notes.items()):
        logger.warning("retrain: run %s has no headline metrics for %s -- %s", run_id, format_code, note)
    return {"run_id": run_id, "run_dir": directory, "manifest": manifest.as_dict(), "summary": summary}


def _match_keys(result: BuildResult) -> List[str]:
    """The identity of every match the pass consumed, for the dataset digest.

    Read off the training frame rather than counted, so two runs agree exactly when they
    walked the same cricket -- a re-import that changes one match's date changes the sha.
    """
    frame = result.frame
    return [f"{row.match_id}|{row.match_date}" for row in frame.itertuples(index=False)]


def parse_cutoff(value: str) -> pd.Timestamp:
    """Parse a training cutoff off the wire, tolerating a timestamp.

    A cutoff is a *date*: rows before it train, rows at or after it are the holdout, and
    the time of day names no different set of rows. So an RFC3339 value is truncated to
    its date rather than refused -- go-app formatted its default cutoff that way until
    F-1 (D-9), the endpoint's own hint has always promised it, and an operator typing a
    timestamp into the console's cutoff box means the day they typed.

    The wire format is declared in ``contracts/ops-console.contract.json`` and asserted
    from both sides (H-24); this is ml-service's half of it.
    """
    text = (value or "").strip()
    # Both separators RFC3339 allows between the date and the time, so "2026-09-02T18:33:11Z"
    # and "2026-09-02 18:33:11" reduce to the same day.
    head = text.split("T", 1)[0].split(" ", 1)[0]
    try:
        return pd.Timestamp(date.fromisoformat(head))
    except ValueError as exc:
        raise ValueError(f"--cutoff must be a date (YYYY-MM-DD) or an RFC3339 timestamp; got {value!r}") from exc


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
    p.add_argument(
        "--cutoff",
        required=True,
        help="YYYY-MM-DD (an RFC3339 timestamp is accepted and read as its date); "
        "rows before it train, rows at/after it are the holdout",
    )
    p.add_argument("--out", default=None, help="artifacts root (default: ml.config.default_artifacts_dir())")
    p.add_argument("--formats", nargs="+", default=list(C.FORMAT_CODES))
    p.add_argument(
        "--gender-split-context",
        action="store_true",
        help="E7 (H-7): split the context baselines (runs/wickets per format x over) by gender",
    )
    p.add_argument(
        "--accept-data-quality",
        action="store_true",
        help="record this run's data-quality counts as the baseline even if the gate failed (H-15)",
    )
    return p.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    args = _parse_args(argv)
    cutoff = parse_cutoff(args.cutoff)

    if args.cricsheet_dir:
        from ml.xi.sources import CricsheetJsonSource
        from ml.xi.train import _international_teams_from_config

        source = CricsheetJsonSource(
            args.cricsheet_dir, _international_teams_from_config(), args.formats, birth_dates_path=args.birth_dates
        )
    else:
        from ml.db import get_db_connection
        from ml.xi.sources import PostgresSource

        source = PostgresSource(get_db_connection(), args.formats)

    out_dir = args.out
    if out_dir is None:
        from ml.config import default_artifacts_dir

        out_dir = default_artifacts_dir()

    result = build(
        source,
        progress=lambda i: logger.info("rating pass: %d matches", i),
        gender_split_context=args.gender_split_context,
    )
    written = retrain(result, out_dir, cutoff, args.formats)

    # The data-quality gate (H-15) decides the exit code, not whether the run exists: an
    # operator has to be able to see what the run produced in order to judge whether the
    # new counts are right. What a failure withholds is the baseline, so re-running
    # cannot clear the gate on its own.
    from ml.xi import quality

    failures = written["summary"]["data_quality_failures"]
    if failures and not args.accept_data_quality:
        for failure in failures:
            logger.error("data-quality gate: %s", failure)
        logger.error(
            "data-quality gate failed (%d %s). Run %s is written but the baseline is unchanged, "
            "so a re-run will fail the same way. Review the counts; if they are right, re-run "
            "with --accept-data-quality.",
            len(failures),
            "check" if len(failures) == 1 else "checks",
            written["run_id"],
        )
        return 1
    if failures:
        logger.warning("data-quality gate failed but --accept-data-quality was given; recording the new baseline")
    quality.save_baseline(out_dir, result.quality)
    logger.info("retrain complete: run %s. Point `current` at it with POST /admin/reload.", written["run_id"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
