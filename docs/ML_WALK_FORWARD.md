# Walk-forward training and evaluation

This document describes **walk-forward** training: training on data before a cutoff, predicting the next X matches (holdout), evaluating (e.g. MAE), then absorbing that window into the training set and repeating. Each model and its parameters are recorded in a **registry** for feedback loops and optional integration with auto-tune.

---

## Pipeline integration: separate step (recommended)

**Walk-forward is a separate step, not part of auto_tune.**

| Step | Purpose | When to run |
|------|---------|-------------|
| **auto_tune** | Find best algorithm + hyperparameters for a given dataset (one cutoff or CSV). | When you want to improve `ml.training.<model>` before training; run once per format/model or periodically. |
| **walk-forward** | Evaluate temporal performance (train → predict next X → score → absorb) and build a **registry** of models, params, and metrics. | When you want to measure how well models would have performed over time, tune the window size X, or feed a feedback loop. |

**Recommended pipeline:**

1. **Core:** Precompute (go-app) → export-dataset → train models → run ML service.
2. **Optional tuning:** Run `make ml-auto-tune MODEL=batting FORMAT=T20` (or from API with cutoff); copy `config_snippet` into `ml.training.batting`; then run `make train-batting` (and same for bowling/fielding).
3. **Optional evaluation:** Run `make walk-forward` (e.g. after training, or on a schedule). Inspect `walk_forward_registry.json`; use it to compare different X or to trigger retraining/auto_tune for underperforming (format, model) windows.

Do **not** merge walk-forward into the auto_tune binary: they have different goals (tuning vs. temporal evaluation + registry), and keeping them separate keeps the code and usage clear. Use the **workflow** (run auto_tune, then walk-forward; or run walk-forward, then auto_tune for bad windows) to integrate them.

---

## 1. Overview

- **Goal:** Evaluate and tune models on sequential cricket data without future leakage, and keep a record of every trained model (cutoff, window size X, params, metrics) for later retraining or auto-tune.
- **Flow per window:**
  1. **Train** on all data with `match_date < cutoff` (via go-app training-data API).
  2. **Predict** the next X matches (holdout; features computed at cutoff).
  3. **Evaluate** (e.g. MAE per target, overall).
  4. **Record** in registry: model type, format, cutoff, window_x, training_params, metrics, optional artifact paths.
  5. **Advance** cutoff to the end of the holdout window (so the next iteration includes those matches in training).

- **Registry:** A JSON file (e.g. `walk_forward_registry.json`) with one entry per (window, model). You can use it to:
  - Compare accuracy when varying X (e.g. X=20 vs 50 vs 100).
  - Identify underperforming windows and retrain or run auto_tune for those (format, model).
  - Feed best params back into `ml.training.<model>` or into auto_tune.

---

## 2. Prerequisites

- **go-app** running with imported match data and (optionally) precomputed features.
- **Endpoints used:**
  - `GET /api/backtest/matches?after=...&format=...&limit=...` — list match_id and match_date after a cutoff.
  - `GET /api/backtest/training-data?format=...&cutoff=...` — training rows (match_date &lt; cutoff).
  - `GET /api/backtest/holdout-data?format=...&cutoff=...&limit=...` — holdout rows (matches after cutoff, features at cutoff).

---

## 3. Config (optional)

In `ml-service/config.json`, optional block **`ml.walk_forward`**:

| Key               | Meaning                          | Default        |
|-------------------|----------------------------------|----------------|
| `initial_cutoff`  | First cutoff (train on data before this). | — (CLI only) |
| `window_x`        | Number of matches per holdout window.     | `50`           |
| `registry_path`   | Path to write registry JSON.     | `{artifacts_dir}/walk_forward_registry.json` |

CLI flags override config. Example:

```json
"ml": {
  "walk_forward": {
    "initial_cutoff": "2020-01-01T00:00:00Z",
    "window_x": 50,
    "registry_path": "../output/ml-service/walk_forward_registry.json"
  }
}
```

---

## 4. How to run

### From repo root

```bash
# Default: initial cutoff 2020-01-01, window 50, format T20, model batting
make walk-forward

# Custom cutoff and window
make walk-forward INITIAL_CUTOFF=2019-06-01T00:00:00Z WINDOW_X=30

# All models (batting + bowling), format ODI
make walk-forward WALK_FORMAT=ODI WALK_MODEL=all

# Cap windows (for testing)
make walk-forward MAX_WINDOWS=3
```

Ensure **GO_APP_URL** is set if go-app is not on `http://localhost:8080`:

```bash
GO_APP_URL=http://localhost:8080 make walk-forward INITIAL_CUTOFF=2020-01-01 WINDOW_X=50
```

### From ml-service

```bash
cd ml-service
GO_APP_URL=http://localhost:8080 python -m ml.walk_forward \
  --initial-cutoff 2020-01-01T00:00:00Z --window-x 50 --format T20 --model batting

# With registry path and save artifacts
python -m ml.walk_forward --initial-cutoff 2020-01-01 --window-x 50 --format T20 \
  --model all --registry ../output/ml-service/walk_forward_registry.json

# Export per-format mean MAE/RMSE for config updates
python -m ml.walk_forward --initial-cutoff 2020-01-01 --window-x 50 --format T20 \
  --model all --export-metrics ../output/ml-service/walk_forward_metrics.json
```

---

## 5. Registry format

The registry JSON has:

- **run_id:** Timestamp of the run.
- **config:** `initial_cutoff`, `window_x`, `format`.
- **summary:** Per-format/model aggregates: `mean_mae_overall`, `std_mae_overall`, `mean_rmse_overall`, `std_rmse_overall`, `n_windows`. Use for feedback loops (e.g. compare T20/batting vs ODI/bowling).
- **windows:** List of entries, one per (window index, model type).

Each entry:

- **model_type:** `batting` | `bowling`
- **format:** e.g. T20
- **cutoff_trained_before:** Cutoff used for training (data before this).
- **window_x:** Number of matches in the holdout window.
- **window_start_date**, **window_end_date:** First and last match date in the window.
- **training_params:** e.g. `n_estimators`, `max_depth`, `random_state` (from `ml.training.<model>`).
- **auto_tune_used:** `true` if this window used auto_tune (e.g. with `--auto-tune-initial`).
- **metrics:** e.g. `mae_runs`, `mae_wickets`, `mae_overall`, `rmse_overall`.
- **n_training_samples**, **n_holdout_samples**
- **artifact_paths:** `{"scaler": "...", "model": "..."}` if artifacts were saved; else `null`.
- **created_at:** ISO timestamp.

Use this to build a feedback loop: e.g. select windows where `mae_overall` is above a threshold and re-run auto_tune for that format/model, then update `ml.training` and re-run walk-forward.

---

## 6. Integration with auto_tune

- **Option A — Tune once, then walk-forward:** Run auto_tune for a format (e.g. T20 batting) with a cutoff, copy `config_snippet` into `ml.training.batting`, then run walk-forward with the same initial cutoff. All windows use the same tuned params.
- **Option B — Tune on first window:** Use `--auto-tune-initial` so the first window’s training data is used to run auto_tune; the best params can be written to the registry (and optionally to config) for that run. (Implementation can run `ml.auto_tune` on the first training fetch and record `auto_tune_used: true` and the chosen params.)
- **Option C — Feedback loop:** After walk-forward, parse the registry, find (format, model) with high MAE, run auto_tune for that (format, model) with a cutoff covering that period, update config, then re-run walk-forward or production training.

---

## 7. References

- Feature computation at cutoff: **docs/ML_DATA_AND_NORMALIZATION.md**
- Auto-tune: **docs/ML_AUTO_TUNE.md**
- Config: **docs/CONFIG.md** (`ml.training`, `ml.walk_forward`)
- Go-app APIs: `GET /api/backtest/matches`, `GET /api/backtest/holdout-data`, `GET /api/backtest/training-data`
