# Auto-tune ML models

This document describes how to **automatically search for the best algorithm and hyperparameters** for each model (batting, bowling, fielding) and optionally apply the results to config and artifacts.

---

## 1. Overview

The auto-tune pipeline:

- Loads training data from **CSV** (go-app export) or from the **go-app training-data API**.
- For each model type (batting, bowling, fielding), runs **RandomizedSearchCV** over:
  - **Algorithms:** `RandomForestRegressor`, `GradientBoostingRegressor`
  - **Hyperparameters:** e.g. `n_estimators`, `max_depth`, `min_samples_split`, `min_samples_leaf` (and for GBM: `learning_rate`).
- Uses **cross-validation** (K-fold; no future leakage within the same dataset) and a configurable **scoring** metric (default: `neg_mean_absolute_error`).
- Saves the **best scaler + model** in the same format as the regular training scripts (`batting_scaler_<FMT>.joblib`, `batting_model_<FMT>.joblib`, etc.) so existing serving and artifact loading work unchanged.
- Writes a **tuning report** JSON per (model, format) with `best_params`, `best_cv_score`, and a **config_snippet** you can copy into `ml-service/config.json` under `ml.training.<model>`.

---

## 2. Config: `ml.tuning`

In `ml-service/config.json`, optional block **`ml.tuning`**:

| Key       | Meaning                    | Default                     |
|----------|----------------------------|-----------------------------|
| `cv_splits` | Number of CV folds         | `5`                         |
| `n_iter`   | RandomizedSearchCV iterations per estimator | `25` |
| `scoring`  | sklearn scoring (e.g. `neg_mean_absolute_error`, `neg_mean_squared_error`) | `neg_mean_absolute_error` |

Example:

```json
"ml": {
  "tuning": {
    "cv_splits": 5,
    "n_iter": 25,
    "scoring": "neg_mean_absolute_error"
  },
  ...
}
```

---

## 3. How to run

### From CSV (per-format)

Assume go-app has exported CSVs to e.g. `../output/go-app/`:

```bash
cd ml-service

# Single model, single format
python -m ml.auto_tune --model batting --format T20
python -m ml.auto_tune --model bowling --format T20
python -m ml.auto_tune --model fielding --format T20

# Explicit CSV path
python -m ml.auto_tune --model batting --csv ../output/go-app/batting_encoded_T20.csv --format T20

# All models, all formats (from ml.formats)
python -m ml.auto_tune --model all --all-formats
```

### From go-app API

Requires **GO_APP_URL** and **--cutoff** (RFC3339). Optionally **--format** to restrict to one format; otherwise combined or per-format depending on model.

```bash
export GO_APP_URL=http://localhost:8080
python -m ml.auto_tune --model batting --from-api --cutoff 2024-12-01T00:00:00Z --format T20
python -m ml.auto_tune --model bowling --from-api --cutoff 2024-12-01T00:00:00Z --format T20
python -m ml.auto_tune --model fielding --from-api --cutoff 2024-12-01T00:00:00Z

# All models, one format
python -m ml.auto_tune --model all --from-api --cutoff 2024-12-01T00:00:00Z --format T20
```

### Make targets

```bash
make auto-tune MODEL=batting FORMAT=T20
make auto-tune MODEL=all ALL_FORMATS=1
```

Output directory for artifacts and reports: **ML_SERVICE_OUTPUT_DIR** or `outputs.artifacts_dir` from config (e.g. `../output/ml-service`).

---

## 4. Outputs

For each (model, format) tuned:

- **Artifacts** (same names as normal training):
  - `{model}_scaler_{format}.joblib`, `{model}_model_{format}.joblib`
  - Without format: `{model}_scaler.joblib`, `{model}_model.joblib`
- **Report:** `tuning_report_{model}_{format}.json` (or `tuning_report_{model}.json`)

Report contents:

- `best_cv_score`: best CV score (e.g. negative MAE)
- `best_params`: full RandomizedSearchCV params (e.g. `est__estimator__n_estimators`)
- **`config_snippet`**: params suitable for **`ml.training.<model>`** (e.g. `n_estimators`, `max_depth`, `random_state`; no prefix). You can copy these into `config.json` for reproducible training with the same hyperparameters.
- `candidates`: list of estimator name, best score, and best params for each algorithm tried.

---

## 5. Tuned params per (model, format) and retraining

Auto-tune saves the best params to the **go-app** (when **GO_APP_URL** is set) so you know **which model and which format** they belong to, and so **retraining** can use them without editing config.

- **Storage:** Each run POSTs to `POST /api/ml/tuned-params` with `{ "model": "batting", "format": "T20", "params": { ... } }`. The go-app stores one row per (model, format) in `ml_tuned_params` (latest per pair).
- **Listing:** `GET /api/ml/tuned-params/list` returns all stored (model, format) entries with `created_at`, so you can see which combinations have saved params.
- **Fetching:** `GET /api/ml/tuned-params?model=batting&format=T20` returns the latest params for that pair; the response includes `model`, `format`, `params`, and `created_at` so it is self-describing.
- **Retraining:** When you run training (e.g. `make train-batting` or pipeline Train Batting) with **GO_APP_URL** set, each per-format training step calls `get_training_params(model, format_code)`, which fetches the latest tuned params for that model+format from the go-app and overlays them on config. So retraining uses the auto-tuned parameters per format without copying them into `config.json`.

---

## 6. Applying best params to config (manual)

After tuning, you can **update `ml-service/config.json`** so that normal training (and train-on-the-fly) use the best-found hyperparameters:

1. Open `tuning_report_batting_T20.json` (or the relevant report).
2. Copy the **`config_snippet`** object (only keys that match `ml.training`; add `joblib_compress` from current config if missing).
3. Paste into `ml.training.batting` (or `bowling`, `fielding`), keeping required keys: `n_estimators`, `max_depth`, `random_state`, `joblib_compress`.

Example: if `config_snippet` is  
`{"n_estimators": 200, "max_depth": 12, "random_state": 42, "min_samples_split": 5}`  
then set `ml.training.batting` to those values and add `joblib_compress: 3` (or your preferred value). Note: `min_samples_split` (and similar) are optional in config; training scripts currently use only `n_estimators`, `max_depth`, `random_state`, `joblib_compress`. Future training script changes could read more keys from config if needed.

---

## 7. Fine-tune all models (workflow)

1. **Export training data** (go-app): ensure CSVs exist for the formats you care about, or have go-app running and a chosen cutoff.
2. **Run auto-tune** for each model (and optionally each format):
   - `python -m ml.auto_tune --model all --all-formats` (CSV), or
   - `python -m ml.auto_tune --model all --from-api --cutoff <CUTOFF> --format <FMT>` per format.
3. **Inspect** `tuning_report_*.json` in the artifacts dir; note best algorithm and `config_snippet`.
4. **Update** `ml.training.batting`, `ml.training.bowling`, `ml.training.fielding` in `config.json` with the chosen params (and re-run normal training if you want to re-train with fixed params without running auto-tune again).
5. **Restart** the ML service so it loads the new artifacts.

---

## 8. References

- Training scripts: `ml-service/ml/train_batting.py`, `train_bowling.py`, `train_fielding.py`
- Train-on-the-fly: `ml-service/app/train_on_the_fly.py`
- Config: `ml-service/config.json` (`ml.training`, `ml.tuning`)
- Pipeline and normalization: **docs/ML_DATA_AND_NORMALIZATION.md**
