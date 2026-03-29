# Note for AI agents: Auto-tune behavior must be uniform

**When changing auto-tune (hyperparameter tuning, MLQA, stability, search space, or Phase 1/2 logic), keep behavior the same across all ML models and all match formats.**

## Rules

1. **Algorithms**  
   Do not add or change behavior for **one** ML algorithm (e.g. only `quantile` or only `mlp`) unless the **same** change is applied to every other algorithm used in auto-tune (e.g. `rf`, `gb`, `quantile`, `et`, `hgb`, `mlp`, `stacked` as applicable).  
   Prefer **data-driven or config-driven** logic (e.g. by sample size, config keys) so one code path handles all algorithms.

2. **Formats**  
   Do not add or change behavior for **one** match format (e.g. only `T20` or only `T20I`) unless the **same** change is applied to every format (e.g. `TEST`, `ODI`, `T20`, `T20I`).  
   Prefer logic that depends on **dataset/training parameters** (e.g. `n_samples`, config) so one code path handles all formats.

3. **Check before merging**  
   Before finishing a change, ask: “Does this treat any algorithm or format specially?” If yes, either generalize the logic so it applies to all, or document why a single-case exception is required and ensure it is minimal and justified.

## Why

Inconsistent behavior across algorithms or formats leads to audit failures, maintenance burden, and bugs that only appear for some models/formats. Uniform, data-driven behavior is easier to reason about and to tune via config.
