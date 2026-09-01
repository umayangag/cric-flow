"""Tuning package — hyperparameter search for the win model.

Public API:
    run_auto_tune_win      — two-phase classification search for win prediction
    main                   — CLI entrypoint

The regression searches (batting, bowling, fielding, extras, innings) went with their
trainers in P-5; this whole stack goes in P-6.
"""

from ml.tuning.cli import main
from ml.tuning.runners import run_auto_tune_win
from ml.tuning.types import (
    AVAILABLE_ALGORITHMS,
    PHASE1_TRIALS_PER_ALGORITHM,
    PHASE2_TRIALS,
)

__all__ = [
    "run_auto_tune_win",
    "main",
    "AVAILABLE_ALGORITHMS",
    "PHASE1_TRIALS_PER_ALGORITHM",
    "PHASE2_TRIALS",
]
