"""The walk-forward folds are the development surface, and every number summarised over
them says so (EVAL-11).

The same eleven quarterly folds have decided every gate this project has run -- the format
scoping, the thresholds, every feature family kept or dropped -- so a fold mean is a
development number under as many comparisons as there are gates that read it, not an
estimate on data no choice has seen. The estimate on unseen data is the holdout
(``evaluate.LOCKED_START`` on), scored once and printed bare. The two are told apart where
the number is: a fold summary is ``{mean, sd, n_folds, gates_consulted}`` and a holdout
figure is a bare value under the ``locked`` node, so a number copied out of the report
still carries which surface it came from and how many gates have read that surface.
"""

from __future__ import annotations

from typing import Dict, Optional, Sequence

import numpy as np

from ml.xi import gates


def summarise_over_folds(values: Sequence[Optional[float]]) -> Optional[Dict[str, float]]:
    """Mean and spread over folds, ignoring folds that could not produce the number, with
    how many gates consulted the folds beside them; None when no fold produced it."""
    present = [float(v) for v in values if v is not None]
    if not present:
        return None
    return {
        "mean": float(np.mean(present)),
        "sd": float(np.std(present)),
        "n_folds": len(present),
        "gates_consulted": gates.folds_consulted_count(),
    }
