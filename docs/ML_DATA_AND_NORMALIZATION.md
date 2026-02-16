# ML pipeline: data normalization and best practices

This document describes how we apply proper machine learning techniques from **data import → feature computation → training → prediction** so that training and serving stay consistent.

---

## 1. Pipeline overview

```
Data import (go-app)     →  Feature computation (go-app)  →  Training (ml-service)  →  Prediction (ml-service)
Cricsheet / DB               EWM form, Consistency,            StandardScaler on X       Same scaler.transform(X)
                             venue/opposition at cutoff        Targets Y in raw units    Model outputs raw Y
                             Strict cutoff (no future data)    Temporal holdout          Same feature order
```

- **No future leakage:** All training data uses only matches with `match_date < cutoff`. Evaluation uses matches at or after cutoff (temporal holdout).
- **Same feature computation:** Go-app uses identical logic for (1) export rows (training data) and (2) feature map at prediction time (`ComputeFeaturesAtCutoffForMatch`). Form = EWM, consistency = coefficient of variation, venue/opposition = EWM at scope.
- **Feature order:** Training and prediction use the same feature order defined in `configs/feature_vectors.json` (batting, bowling, fielding). The ML service builds the vector from this config at prediction; training scripts must use the same order when building X.

---

## 2. Data import and feature computation (go-app)

- **Raw data:** Runs, balls, wickets, etc. are stored in raw units. No normalization at import.
- **Feature computation (form, consistency, venue, opposition):**
  - **Form:** Exponentially weighted mean (EWM) of performance (e.g. runs for batting, wickets for bowling) over history before cutoff. Same formula in export and at prediction.
  - **Consistency:** Coefficient of variation (std/mean) over the last N innings. Bounded by avoiding division by zero (mean = 1 when 0).
  - **Venue / opposition:** EWM of performance in that venue or vs that opposition before cutoff.
- **Units:** Features are in **raw or derived units** (e.g. form in runs scale, consistency dimensionless). They are **not** z-score or min-max normalized in go-app. Normalization is applied only in the ML service so that the same scaler fitted on training data is used at prediction.

---

## 3. Training (ml-service)

### 3.1 Input normalization (features X)

- **Method:** `sklearn.preprocessing.StandardScaler` (zero mean, unit variance).
- **Fit:** Scaler is fitted **only on the training set** (all rows with `match_date < cutoff` from the training-data API or CSV). No test/future data is used.
- **Transform:** Training inputs are transformed with `scaler.fit_transform(X)` before fitting the model. The same scaler is saved and loaded at prediction.

### 3.2 Targets (Y)

- **No scaling:** Targets (runs, wickets, economy, catches, run_outs, etc.) are kept in **raw units** during training and prediction. Tree-based models (RandomForest) do not require target scaling; keeping raw units keeps API responses interpretable and avoids loading a second scaler for inverse transform.

### 3.3 Missing values

- **Training:**
  - **CSV / train_batting / train_bowling:** Rows with missing required feature columns are dropped (`dropna(subset=feature_cols)`). Optional: fill remaining NaNs with 0 before scaling (e.g. `X.fillna(0.0)`) where the export may have sparse columns.
  - **Train-on-the-fly:** Rows with any missing value in the required feature columns are dropped (`dropna(subset=feature_cols)`). No fill.
- **Prediction:** If the go-app feature map omits a key, the ML service uses documented defaults (e.g. 0.5 for consistency, 0 for form) in `build_*_features_from_map`. These defaults should match the semantics of “no history” so that training (where we drop or fill) and prediction stay aligned.

### 3.4 Feature order

- Feature columns must appear in the **same order** as in `configs/feature_vectors.json` for the model kind (batting, bowling, fielding). Training scripts that build X from CSV or API must use the same order (e.g. by selecting columns in the order defined in that config or in a shared constant that mirrors it).

### 3.5 Temporal validity

- Training data is restricted by **cutoff** (e.g. `GET /api/backtest/training-data?cutoff=...`). Only matches strictly before the cutoff are used. No train/validation split is applied inside the training script; evaluation is done via backtest on held-out matches (temporal holdout).

---

## 4. Prediction (ml-service)

- **Features:** Built from the go-app feature map using `build_batting_features_from_map`, `build_bowling_features_from_map`, `build_fielding_features_from_map`. The feature vector is constructed in the order given by `get_feature_names("batting"|"bowling"|"fielding")` from `configs/feature_vectors.json`.
- **Scaling:** The **same** scaler that was fitted at training is loaded and applied: `scaler.transform(X)`. No refit at prediction.
- **Outputs:** Model predictions are in **raw units** (runs, wickets, economy, catches, run_outs). No inverse transform.

---

## 5. Summary table

| Stage              | Practice |
|--------------------|----------|
| Import             | Raw units; no normalization. |
| Feature computation| Same formulas for export and prediction; strict cutoff; no future data. |
| Training X         | StandardScaler fitted on training data only; transform X before fit. |
| Training Y         | No scaling; raw units. |
| Missing (train)    | Drop rows with missing required features (or fill with 0 where defined). |
| Missing (predict)  | Documented defaults per feature so behavior matches “no history”. |
| Feature order      | One source of truth: `configs/feature_vectors.json`; training and prediction use same order. |
| Evaluation         | Temporal holdout (evaluate on matches after cutoff). |

---

## 6. Go-app: feature value ranges

- **Form:** EWM of runs (batting) or wickets (bowling) or fielding involvements — typically 0 to ~100+ for runs, 0 to ~5 for wickets, 0 to ~3 for fielding.
- **Consistency:** Coefficient of variation — typically 0 to ~2.
- **Venue / opposition:** EWM at scope — same scale as form.
- **Weather:** Raw or categorical (e.g. temp 0–50, viscosity 0/1/2). Same in export and at prediction.
- No min-max or z-score is applied in go-app; the ML service’s StandardScaler handles input normalization so training and prediction see the same transformation.

---

## 7. References

- Feature order: `configs/feature_vectors.json`
- Build features at prediction: `app/backtest_service.py` (`build_*_features_from_map`), `app/features.py` (`*_feature_vector`)
- Training: `ml/train_batting.py`, `ml/train_bowling.py`, `ml/train_fielding.py`, `app/train_on_the_fly.py`
- Scaler save/load: `app/artifacts.py` (scaler and model per format)
