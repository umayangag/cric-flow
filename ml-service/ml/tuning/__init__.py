"""Tuning package — hyperparameter search, model selection, and auto-tune orchestration.

Public API:
    run_auto_tune          — two-phase search for batting/bowling/fielding/innings
    run_auto_tune_extras   — single-output regression for extras
    run_auto_tune_win      — classification search for win prediction
    main                   — CLI entrypoint
"""

from ml.tuning.cli import main
from ml.tuning.runners import run_auto_tune, run_auto_tune_extras, run_auto_tune_win
from ml.tuning.types import (
    AVAILABLE_ALGORITHMS,
    BAT_SEQ_COLS,
    BATTING_FEATURE_COLS,
    BATTING_TARGET_COLS,
    BOWL_SEQ_COLS,
    BOWLING_FEATURE_COLS,
    BOWLING_TARGET_COLS,
    PHASE1_TRIALS_PER_ALGORITHM,
    PHASE2_TRIALS,
)

__all__ = [
    "run_auto_tune",
    "run_auto_tune_extras",
    "run_auto_tune_win",
    "main",
    "AVAILABLE_ALGORITHMS",
    "BAT_SEQ_COLS",
    "BATTING_FEATURE_COLS",
    "BATTING_TARGET_COLS",
    "BOWL_SEQ_COLS",
    "BOWLING_FEATURE_COLS",
    "BOWLING_TARGET_COLS",
    "PHASE1_TRIALS_PER_ALGORITHM",
    "PHASE2_TRIALS",
]
