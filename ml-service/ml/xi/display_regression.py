"""display-regression: a display-AUC fall against the previous accepted harness run, read
only where the two runs scored the same population.

Batch 4's T20 display AUC fell 0.7294 -> 0.5934, 3.24 fold sd, and no gate fired: T20 is
scoped off optimised selection, so H-17's absolute line does not read it. The fall was then
shown to be composition, entirely -- IMPORT-09 moved 3,888 international matches out of T20
into T20I, and batch 3's own model already scored 0.5747 on the club rows it had been
reported at 0.6966 on. An absolute floor would have fired on a non-defect: club T20 display
had been 0.60 all along. So this gate is relative and guarded. It compares a format's
walk-forward mean display AUC with the previous accepted run's, and it fails only when the
two runs scored the same population -- the same fold windows and, per ``competition_level``,
the same decided rows within ``MAX_LEVEL_MOVE_SHARE`` of the format's total. A population
that moved is reported as a re-baseline, with the move printed beside it, never as a
failure. T20 is included: the display model is what T20 is served, and this is the only
gate that reads it.

**The previous accepted run** is the harness report ``make evaluate`` last wrote to the
same directory (``evaluate.REPORT_NAME``), when its ``gates.passed`` was true -- the
harness is the batch's acceptance test, and a report every gate passed is an accepted run,
which is the notion ``run_usability`` already uses for the served manifest on the retrain
side. A previous report that did not pass carries, per format, the ``baseline`` it was
itself judged against, and that is read instead; so one failure does not erase the
reference and a re-run cannot pass by having failed once. A report is not a run
directory: the harness refits every model per fold and is tied to no served run, which is
why the reference is the report and not ``current_run.json``.

**Materially unchanged row counts** means: the walk-forward windows (the cutoffs and the
locked start) are the same, the fold count is the same, and no level's decided
development-row count moved by more than 5 % of the format's rows in the previous run. The
5 % is sized to the effect the guard exists to absorb. The largest gap between two levels'
display AUC on record is 0.21 (T20 club 0.60 against international 0.81, batch 3's
taxonomy); moving 5 % of a format's rows between levels shifts the pooled number by about
0.01 to first order, a quarter of the smallest display fold sd on record (0.035, T20I), so
composition alone cannot carry a fall the gate would read as a regression. Above it the
comparison is not like-for-like and the run re-baselines. IMPORT-09's change moved 31.5 %
of T20's rows and 185 % of T20I's; the ordinary difference between two harness runs on the
same windows is a handful of archive corrections (IMPORT-08's fold changed two rows). A
rotation of the windows (A-4) adds a season, 5-15 % of a format's rows, and is caught by
the windows check before any count is read: after a rotation the folds score different
matches and no fold mean is comparable with the last one, whatever the counts say.

The tolerance is one fold sd -- this run's sample sd over the folds (EVAL-15, ``ddof=1``),
the "after" column every batch record reads a move in units of. Absent evidence is not a
failure: with no previous accepted report, no display number on either side, or a single
fold with no spread, the gate records ``undecided`` and this run becomes the baseline.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any, Dict, List, Optional, Tuple

import pandas as pd

#: The key under a format's node the gate writes, and the registry's report path prefix.
NODE = "display_regression"

VERDICT_PASS = "pass"
VERDICT_FAIL = "fail"
VERDICT_REBASELINED = "rebaselined"
VERDICT_UNDECIDED = "undecided"

#: A level's row-count change, as a share of the format's rows in the previous accepted
#: run, above which the two populations are not the same measurement (see the docstring).
MAX_LEVEL_MOVE_SHARE = 0.05

#: How far under the previous accepted run's mean, in this run's fold sd, a fall fails.
MAX_FALL_IN_FOLD_SD = 1.0

#: The level a row is counted under when its source recorded none (a database migrated but
#: not re-imported; a synthetic source). Named, so the wire never carries an empty key.
UNRECORDED_LEVEL = "unrecorded"


@dataclass(frozen=True)
class Reference:
    """What one harness run measured for one format, as far as this gate reads it."""

    #: The report's ``generated_at``: which run the numbers are from.
    generated_at: str
    #: The walk-forward display AUC summary (``{mean, sd, n_folds, ...}``), or None when no
    #: fold scored the format.
    display_auc: Optional[Dict[str, Any]]
    #: Decided development rows per competition level -- the rows the folds can read. None
    #: on a report written before this gate existed, which then cannot be shown like-for-like.
    development_rows_by_level: Optional[Dict[str, int]]
    #: The fold windows: ``{"cutoffs": [...], "locked_start": "..."}``.
    windows: Dict[str, Any]

    def as_dict(self) -> Dict[str, Any]:
        return asdict(self)

    @property
    def mean(self) -> Optional[float]:
        if self.display_auc is None or self.display_auc.get("mean") is None:
            return None
        return float(self.display_auc["mean"])

    @property
    def fold_sd(self) -> Optional[float]:
        if self.display_auc is None or self.display_auc.get("sd") is None:
            return None
        return float(self.display_auc["sd"])

    @property
    def n_folds(self) -> Optional[int]:
        if self.display_auc is None or self.display_auc.get("n_folds") is None:
            return None
        return int(self.display_auc["n_folds"])


def windows(cutoffs: List[str], locked_start: str) -> Dict[str, Any]:
    """The fold windows as the reference records them."""
    return {"cutoffs": list(cutoffs), "locked_start": locked_start}


def development_rows_by_level(format_frame: pd.DataFrame, locked_start: str) -> Dict[str, int]:
    """Decided development rows per competition level: every row a fold can read (before
    the locked start), keyed by the level the source recorded, ``UNRECORDED_LEVEL`` where
    it recorded none. Sorted by level so two reports render the same."""
    development = format_frame[format_frame.match_date < pd.Timestamp(locked_start)]
    if "competition_level" in development.columns:
        levels = development.competition_level.fillna("").astype(str).replace("", UNRECORDED_LEVEL)
    else:
        levels = pd.Series(UNRECORDED_LEVEL, index=development.index, dtype="object")
    counts = levels.value_counts()
    return {str(level): int(counts[level]) for level in sorted(counts.index)}


def baseline_for(previous_report: Optional[Dict[str, Any]], format_code: str) -> Optional[Reference]:
    """The previous accepted run's reference for a format, or None when there is none.

    An accepted report (``gates.passed``) is read for its own numbers; one that did not pass
    hands over the ``baseline`` it carried for the format, which is the accepted reference
    it was itself judged against -- or None, when it had none either.
    """
    if previous_report is None:
        return None
    node = (previous_report.get("formats") or {}).get(format_code)
    if node is None:
        return None
    if (previous_report.get("gates") or {}).get("passed"):
        counts = ((node.get(NODE) or {}).get("current") or {}).get("development_rows_by_level")
        return Reference(
            generated_at=str(previous_report.get("generated_at", "")),
            display_auc=(node.get("walk_forward") or {}).get("summary", {}).get("display_auc"),
            development_rows_by_level=None if counts is None else dict(counts),
            windows=windows(previous_report.get("cutoffs") or [], str(previous_report.get("locked_start", ""))),
        )
    carried = (node.get(NODE) or {}).get("baseline")
    if not carried:
        return None
    return Reference(
        generated_at=str(carried.get("generated_at", "")),
        display_auc=carried.get("display_auc"),
        development_rows_by_level=carried.get("development_rows_by_level"),
        windows=dict(carried.get("windows") or {}),
    )


def level_moves(before: Dict[str, int], after: Dict[str, int]) -> Dict[str, Dict[str, Any]]:
    """Each level's row count in the previous accepted run and in this one, and the change
    as a share of the previous run's rows for the format. A level present on one side only
    reads zero on the other; a previous run with no rows at all has no share to divide by,
    and a change against it reads ``None`` -- "nothing to compare with", which is material."""
    total_before = sum(before.values())
    moves: Dict[str, Dict[str, Any]] = {}
    for level in sorted(set(before) | set(after)):
        rows_before, rows_after = int(before.get(level, 0)), int(after.get(level, 0))
        difference = abs(rows_after - rows_before)
        share: Optional[float]
        if total_before > 0:
            share = difference / total_before
        else:
            share = None if difference else 0.0
        moves[level] = {"before": rows_before, "after": rows_after, "share_of_format": share}
    return moves


def _is_material(share: Optional[float]) -> bool:
    return share is None or share > MAX_LEVEL_MOVE_SHARE


def _like_for_like(current: Reference, baseline: Reference) -> Tuple[bool, str, Optional[Dict[str, Dict[str, Any]]]]:
    """Whether the two runs scored the same population, and why not when they did not."""
    if current.windows != baseline.windows:
        return (
            False,
            "the walk-forward windows moved (the cutoffs or the locked start), so the folds score different "
            "matches and no fold mean is comparable with the previous run's",
            None,
        )
    if baseline.development_rows_by_level is None:
        return (
            False,
            "the previous accepted run carries no per-level row counts (it was written before this gate), so "
            "the comparison cannot be shown like-for-like",
            None,
        )
    if current.n_folds != baseline.n_folds:
        return (
            False,
            f"the fold count differs ({baseline.n_folds} folds before, {current.n_folds} now), so the two means "
            "are not over the same windows",
            None,
        )
    moves = level_moves(baseline.development_rows_by_level, current.development_rows_by_level or {})
    material = [level for level, move in moves.items() if _is_material(move["share_of_format"])]
    if material:
        total_before = sum(baseline.development_rows_by_level.values())
        moved = "; ".join(
            f"{level} {moves[level]['before']:,} -> {moves[level]['after']:,} "
            f"({_share_text(moves[level]['share_of_format'])} of the previous run's {total_before:,} rows)"
            for level in material
        )
        return (
            False,
            f"the per-level row counts moved materially (more than {MAX_LEVEL_MOVE_SHARE:.0%} of the format's "
            f"rows): {moved}; the populations differ, so the move is reported and not judged",
            moves,
        )
    return True, "", moves


def _share_text(share: Optional[float]) -> str:
    return "no share to read, the previous run read no rows" if share is None else f"{share:.1%}"


def decide(current: Reference, baseline: Optional[Reference]) -> Dict[str, Any]:
    """The gate's node for one format: the verdict, why, the move in fold sd, both
    references, the like-for-like working, and the ``baseline`` a later run reads when this
    report is not accepted -- the reference this run was judged against on a pass or a
    fail, and this run's own numbers on a re-baseline or when nothing could be decided,
    because the old reference was then either declared incomparable or absent."""
    node: Dict[str, Any] = {
        "verdict": VERDICT_UNDECIDED,
        "reason": "",
        "display_auc_move_in_fold_sd": None,
        "level_moves": None,
        "current": current.as_dict(),
        "compared_against": None if baseline is None else baseline.as_dict(),
    }
    verdict, reason = _verdict(current, baseline, node)
    node["verdict"], node["reason"] = verdict, reason
    keeps_reference = verdict in (VERDICT_PASS, VERDICT_FAIL) and baseline is not None
    node["baseline"] = (baseline if keeps_reference else current).as_dict()
    return node


def _verdict(current: Reference, baseline: Optional[Reference], node: Dict[str, Any]) -> Tuple[str, str]:
    if baseline is None:
        return VERDICT_UNDECIDED, (
            "no previous accepted harness report to compare against (none written, none readable, or none "
            "carrying a baseline); this run is the baseline"
        )
    if current.mean is None or current.fold_sd is None:
        return VERDICT_UNDECIDED, "no walk-forward display AUC this run, so there is nothing to compare"
    if baseline.mean is None:
        return VERDICT_UNDECIDED, (
            f"the previous accepted run ({baseline.generated_at}) carries no walk-forward display AUC for this "
            "format; this run is the baseline"
        )
    if current.fold_sd <= 0.0:
        return VERDICT_UNDECIDED, (
            f"one fold ({current.n_folds}) has no spread to read a move against; this run is the baseline"
        )
    move = (current.mean - baseline.mean) / current.fold_sd
    node["display_auc_move_in_fold_sd"] = move
    comparable, why_not, moves = _like_for_like(current, baseline)
    node["level_moves"] = moves
    described = (
        f"display AUC {current.mean:.4f} against the previous accepted run's {baseline.mean:.4f} "
        f"({baseline.generated_at}): {move:+.2f} fold sd (sd {current.fold_sd:.4f})"
    )
    if not comparable:
        return VERDICT_REBASELINED, f"{described}, but {why_not}; this run is the baseline"
    if move < -MAX_FALL_IN_FOLD_SD:
        return VERDICT_FAIL, (
            f"{described}, a fall beyond one fold sd on the same windows and the same per-level rows: the model "
            "got worse, not the population"
        )
    return VERDICT_PASS, f"{described}, within one fold sd on the same windows and the same per-level rows"
