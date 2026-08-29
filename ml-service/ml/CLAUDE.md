# Auto-tune: uniform behaviour for all algorithms and formats

Applies when editing **auto-tune** — hyperparameter tuning, MLQA, stability, search space,
Phase 1/2 — in `ml/auto_tune*.py` or `ml/tuning/`.

1. **No algorithm-only changes.** Do not add or change behaviour for a single ML algorithm
   (e.g. only `quantile` or only `mlp`) unless the **same** logic applies to every other
   algorithm used in auto-tune. Prefer one data- or config-driven path for all algorithms.

2. **No format-only changes.** Do not add or change behaviour for a single match format (e.g.
   only `T20` or only `T20I`) unless the **same** logic applies to every format. Drive the logic
   from dataset or training parameters (`n_samples`, config) so all formats are handled
   uniformly.

3. **Before finishing**, ask: "does this treat any algorithm or format specially?" If yes, either
   generalize it to all, or document a minimal, justified exception.

Full note: [tuning/AGENTS_AUTO_TUNE.md](tuning/AGENTS_AUTO_TUNE.md).
