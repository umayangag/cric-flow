"""CLI entry point for auto-tune: ``python -m ml.auto_tune --format T20``.

The logic lives in the ``ml.tuning`` package. Only the win model is tunable after P-5,
and P-6 removes the stack entirely.

AGENTS: When modifying auto-tune behavior, keep it uniform across all ML algorithms
and all match formats. Do not add logic that applies only to one algorithm (e.g. quantile)
or only to one format (e.g. T20). Prefer data- or config-driven logic so the same code path
handles every model and format. See ml/tuning/AGENTS_AUTO_TUNE.md for the full note.
"""

from __future__ import annotations

from ml.tuning.cli import main

if __name__ == "__main__":
    main()
