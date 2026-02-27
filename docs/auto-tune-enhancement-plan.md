# Auto-Tune Enhancement Plan: PyCaret, Optuna, Auto-sklearn, AutoGluon & Neural Networks

A detailed plan to enhance the cric-flow auto-tune feature for maximum accuracy in minimal time by integrating PyCaret, Optuna, auto-sklearn, AutoGluon, and neural networks.

---

## 1. Executive Summary

**Current state:** Two-phase pipeline—(1) coarse algorithm screening (RF, GBM, ET, HGB, quantile, stacked) via RandomizedSearchCV, (2) Optuna TPE fine-tuning on top 2 winners. Saves best scaler+model in joblib format.

**Target state:** Four-stage pipeline for fastest path to best models:

| Stage | Tool | Role | Output |
|-------|------|------|--------|
| 1 | **PyCaret** | Fast algorithm comparison (15+ models in one call) | Top 2–3 algorithms per task |
| 2 | **Optuna / auto-sklearn** | Deep hyperparameter tuning on winners | Best-tuned sklearn/ensemble models |
| 3 | **AutoGluon** | Stacking/ensembling to push accuracy | Strong tabular models; optional as final predictor |
| 4 | **Neural networks** | Optional boost for regression/classification | MLP/TabNet as candidates in PyCaret + Optuna |

**Models in scope:** batting, bowling, fielding (multi-output regression), extras (single-output regression), win (binary classification), combination_meta (regression for team selection weights).

---

## 2. Current Pipeline (Baseline)

### 2.1 Models and Tasks

| Model | Task Type | Outputs | Data Source | Artifact Format |
|-------|-----------|---------|-------------|-----------------|
| batting | Multi-output regression | runs, balls, fours, sixes, batting_position, strike_rate | CSV per format | scaler + model (joblib) |
| bowling | Multi-output regression | runs_conceded, deliveries, wickets, economy | CSV per format | scaler + model |
| fielding | Multi-output regression | catches, run_outs, stumpings | CSV / API | scaler + model |
| extras | Single-output regression | total_extras | API | model only |
| win | Binary classification | team1_wins (0/1) | API | model only |
| combination_meta | Regression | bat/bowl/field weights | CSV from backtest | JSON coefficients |

### 2.2 Current Two-Phase Flow

1. **Phase 1 (screening):** RandomizedSearchCV with coarse param grids over RF, GB, ET, HGB, quantile, stacked (regression) or RF, GB, ET, HGB (classification/single regression). ~5 trials per algorithm.
2. **Phase 2 (fine-tuning):** Optuna TPE on top 2 algorithms, ~25 trials. Best pipeline saved.

### 2.3 Constraints (Must Preserve)

- **Feature order:** Must match `configs/feature_vectors.json` and prediction-time feature map.
- **Artifact layout:** `{kind}_scaler_{format}.joblib`, `{kind}_model_{format}.joblib`; extras/win: model only, no scaler.
- **go-app compatibility:** Predict endpoints expect specific artifact names and scaler/model structure.
- **Walk-forward validation:** Temporal splits (TimeSeriesSplit) for batting/bowling/fielding; optional for extras/win.
- **Progress JSON:** `auto_tune_progress.json` for frontend display (phase, algorithm, trial, etc.).

---

## 3. Proposed Four-Stage Pipeline

### 3.1 Stage 1: PyCaret Algorithm Screening

**Purpose:** Replace manual Phase 1 with PyCaret `compare_models()` to quickly evaluate 15+ algorithms.

**Per task type:**

- **Regression (batting, bowling, fielding, extras):** `pycaret.regression`
- **Classification (win):** `pycaret.classification`

**PyCaret setup requirements:**

- Use `setup(data=df, target=target, …)` with:
  - `fold` = config `cv_splits` (default 5)
  - `session_id` = config `random_state`
  - Custom fold generator for **walk_forward** when needed (PyCaret 3.x supports `fold_strategy`)
  - `preprocess=True` (handles missing values, scaling internally; we may need to disable and use our StandardScaler for consistency with prediction pipeline)
- **Note:** PyCaret handles scaling internally. For artifact compatibility, we either:
  - **(A)** Use PyCaret only for algorithm ranking, then rebuild pipelines with our StandardScaler + chosen estimator, or
  - **(B)** Accept PyCaret’s internal preprocessing and ensure prediction uses the same (more invasive for go-app).

**Recommended: Option A** — PyCaret for ranking only; we rebuild pipelines with our scaler and chosen algorithms.

**Include models:**

- Regression: RF, GBM, ET, HGB, XGBoost, LightGBM, CatBoost (if available), ExtraTrees, MLPRegressor, Ridge, Lasso, ElasticNet, KNN, SVM (SVR), AdaBoost, etc.
- Classification: RF, GBM, ET, HGB, XGBoost, LightGBM, CatBoost, MLPClassifier, LogisticRegression, ExtraTrees, AdaBoost, etc.

**Output:** Top N algorithms (default 3) by CV metric (MAE, RMSE, R² for regression; accuracy, AUC, F1 for classification). Store in `pycaret_ranking_{model}_{format}.json`.

**Config addition:**

```json
"ml": {
  "tuning": {
    "pycaret": {
      "enabled": true,
      "n_select": 3,
      "include": [],
      "exclude": []
    }
  }
}
```

---

### 3.2 Stage 2: Optuna / Auto-sklearn Hyperparameter Tuning

**Purpose:** Deep hyperparameter search on PyCaret winners (or current Phase 1 winners if PyCaret disabled).

#### 2a. Optuna (Primary)

- Extend current Optuna Phase 2 to cover:
  - All winners from PyCaret (or existing Phase 1)
  - **Neural networks:** Add `MLPRegressor` / `MLPClassifier` to Optuna’s `suggest_*` search space:
    - `hidden_layer_sizes`: suggest_categorical([(64,64), (128,64), (128,128,64), (256,128,64)])
    - `activation`: relu, tanh
    - `alpha`: log-uniform
    - `learning_rate_init`: log-uniform
    - `max_iter`: 500–2000
- Increase `PHASE2_TRIALS` optionally (e.g., 50) when time permits.
- Add **Optuna pruning** (MedianPruner) for early stopping of bad trials.

#### 2b. Auto-sklearn (Alternative / Parallel Path)

- Use **auto-sklearn 2.x** when available; fallback to 0.15.x.
- **Regressor:** `AutoSklearnRegressor`
- **Classifier:** `AutoSklearnClassifier`
- Configure:
  - `time_left_for_this_task` (e.g., 600 seconds)
  - `per_run_time_limit` (e.g., 60)
  - `ensemble_size`, `ensemble_nbest`
  - Custom `metric` (e.g., mean_absolute_error, accuracy)
- **Output:** Best auto-sklearn model. We must **export to sklearn-compatible pipeline** for joblib. Auto-sklearn exposes `.fit()` / `.predict()` and can be wrapped; ensemble is more complex—may need to keep as separate artifact type if we want to use it directly.
- **Recommendation:** Use auto-sklearn as an optional “turbo” path: run in parallel with Optuna, compare final CV scores, pick best. Requires careful handling of multi-output regression (auto-sklearn supports it).

---

### 3.3 Stage 3: AutoGluon Accuracy Boost

**Purpose:** Use AutoGluon’s stacking and ensembling to push accuracy further, especially for win and meta-model.

**AutoGluon API:**

- `TabularPredictor` with `fit()`:
  - `time_limit` (e.g., 300–600 seconds)
  - `presets`: "best_quality" (slower) vs "medium_quality" (faster)
  - `eval_metric`: "mean_absolute_error" (regression), "accuracy" or "roc_auc" (classification)

**Integration options:**

1. **Option A (Recommended):** AutoGluon as a **candidate** in the pipeline. Run after Optuna; compare AutoGluon’s leaderboard CV score vs best Optuna model. If AutoGluon wins, use its predictor; otherwise keep Optuna model.
2. **Option B:** AutoGluon always runs; we ensemble Optuna best + AutoGluon via a simple average or stacking meta-learner.
3. **Option C:** AutoGluon only for **win** and **combination_meta** (smaller, high-value models).

**Artifact compatibility:**

- AutoGluon saves models in its own format. We need a **wrapper** that:
  - Loads AutoGluon predictor from disk
  - Exposes `predict(X)` with same interface as our current models
  - Ensures feature order matches our config
- go-app would need to load this wrapper when `model_type=autogluon` in metadata.

**Multi-output regression (batting, bowling, fielding):**

- AutoGluon supports multi-output via multiple targets or a custom approach. Document and test per-target vs multi-output strategies.

**Config addition:**

```json
"ml": {
  "tuning": {
    "autogluon": {
      "enabled": true,
      "time_limit_seconds": 300,
      "presets": "medium_quality",
      "models": ["win", "extras", "combination_meta"]
    }
  }
}
```

---

### 3.4 Stage 4: Neural Networks

**Purpose:** Add MLPs and optionally TabNet as first-class candidates.

#### 4a. MLP (sklearn)

- Add `MLPRegressor` and `MLPClassifier` to:
  - PyCaret `include` list (if we use PyCaret)
  - Optuna search space (already described in 3.2)
- Use `StandardScaler` before MLP (required for convergence).
- Enable `early_stopping=True` when available.

#### 4b. TabNet (optional)

- `pytorch-tabnet` provides `TabNetRegressor` and `TabNetClassifier`.
- Add to PyCaret `include` (if PyCaret supports it) or as a standalone Optuna candidate.
- Hyperparameters: `n_d`, `n_a`, `n_steps`, `gamma`, `lambda_sparse`, etc.
- **Dependency:** PyTorch + pytorch-tabnet; heavier. Make optional via `extras-neural` or feature flag.

**Config addition:**

```json
"ml": {
  "tuning": {
    "neural": {
      "enabled": true,
      "mlp": true,
      "tabnet": false
    }
  }
}
```

---

## 4. Model-Specific Details

### 4.1 Batting, Bowling, Fielding (Multi-Output Regression)

| Stage | Action |
|-------|--------|
| PyCaret | Use `pycaret.regression`; for multi-output, either (a) loop over targets and rank algorithms per target, or (b) use a single target (e.g., runs) for ranking, then apply same algorithm set to all outputs. |
| Optuna | Current MultiOutputRegressor + estimator; extend to MLPRegressor. |
| AutoGluon | Fit per-target or multi-target; compare with Optuna best. |
| Neural | MLPRegressor in MultiOutputRegressor; TabNet if available. |

### 4.2 Extras (Single-Output Regression)

| Stage | Action |
|-------|--------|
| PyCaret | Standard regression compare_models. |
| Optuna | Current flow; add MLPRegressor. |
| AutoGluon | Full TabularPredictor; likely strong for small tabular. |
| Neural | MLPRegressor, TabNet. |

### 4.3 Win (Binary Classification)

| Stage | Action |
|-------|--------|
| PyCaret | Classification compare_models; rank by accuracy/AUC/F1. |
| Optuna | Add MLPClassifier; consider calibration (Platt/isotonic) post-hoc. |
| AutoGluon | High priority—classification is AutoGluon’s strength. |
| Neural | MLPClassifier, TabNet. |

### 4.4 Combination Meta (Regression)

| Stage | Action |
|-------|--------|
| Current | Ridge on (bat_score, bowl_score, field_score, is_keeper, format). |
| Enhancement | Add AutoGluon TabularPredictor; optionally Optuna-tuned Ridge/ElasticNet; MLP. Output remains JSON (coefficients or weights). For AutoGluon, we need a different export (e.g., bat/bowl/field weights extracted from feature importances or a surrogate). |

**Recommendation:** Keep Ridge as default; add optional AutoGluon path for combination_meta with a custom export that maps to `bat`, `bowl`, `field`, `keeper_bonus`, `intercept` for go-app compatibility.

---

## 5. Pipeline Orchestration

### 5.1 Flow Diagram

```
                    ┌─────────────────────────────────────────────────────────────┐
                    │                     AUTO-TUNE PIPELINE                        │
                    └─────────────────────────────────────────────────────────────┘
                                              │
                    ┌─────────────────────────▼─────────────────────────┐
                    │  STAGE 1: PyCaret (optional, fast)                 │
                    │  - compare_models() for regression/classification  │
                    │  - Output: top 3 algorithms                        │
                    └─────────────────────────┬─────────────────────────┘
                                              │
                    ┌─────────────────────────▼─────────────────────────┐
                    │  STAGE 2: Optuna / Auto-sklearn                    │
                    │  - TPE on PyCaret winners + MLP + (TabNet)         │
                    │  - (Optional) Auto-sklearn in parallel             │
                    │  - Output: best sklearn/auto-sklearn pipeline      │
                    └─────────────────────────┬─────────────────────────┘
                                              │
                    ┌─────────────────────────▼─────────────────────────┐
                    │  STAGE 3: AutoGluon (optional)                     │
                    │  - TabularPredictor.fit()                          │
                    │  - Compare leaderboard vs Optuna best              │
                    │  - Output: best of Optuna vs AutoGluon             │
                    └─────────────────────────┬─────────────────────────┘
                                              │
                    ┌─────────────────────────▼─────────────────────────┐
                    │  SAVE: Best pipeline + report + progress JSON      │
                    └───────────────────────────────────────────────────┘
```

### 5.2 Config-Driven Stages

All stages configurable via `ml.tuning`:

```json
"ml": {
  "tuning": {
    "stages": {
      "pycaret": { "enabled": true, "n_select": 3 },
      "optuna": { "enabled": true, "n_trials": 50, "pruner": "median" },
      "autosklearn": { "enabled": false, "time_limit": 600 },
      "autogluon": { "enabled": true, "time_limit_seconds": 300, "models": ["win", "extras"] },
      "neural": { "enabled": true, "mlp": true, "tabnet": false }
    },
    "cv_splits": 5,
    "validation_method": "walk_forward",
    "scoring": "neg_mean_absolute_error",
    "random_state": 42
  }
}
```

### 5.3 Time Budget (Per Model/Format)

| Stage | Approx. time (single format) |
|-------|-----------------------------|
| PyCaret | 2–5 min |
| Optuna (50 trials) | 5–15 min |
| Auto-sklearn (600s) | 10 min |
| AutoGluon (300s) | 5 min |
| **Total (all stages)** | ~25–35 min per (model, format) |

Use `--fast` flag to disable AutoGluon and auto-sklearn, reduce Optuna trials to 25.

---

## 6. Implementation Plan

### Phase A: Dependencies & Structure (Week 1)

1. **requirements.in**
   - Add: `pycaret`, `autogluon.tabular`
   - Optional: `auto-sklearn`, `pytorch`, `pytorch-tabnet`
   - Run `pip-compile` and verify no conflicts.

2. **New modules**
   - `ml/auto_tune_pycaret.py` — PyCaret ranking
   - `ml/auto_tune_optuna_extended.py` — Optuna + MLP/TabNet (or extend `auto_tune.py`)
   - `ml/auto_tune_autogluon.py` — AutoGluon integration
   - `ml/auto_tune_autosklearn.py` — Optional auto-sklearn path
   - `ml/autogluon_wrapper.py` — Wrapper for prediction compatibility

3. **Config schema**
   - Extend `config.default.json` with `ml.tuning.stages`, `ml.tuning.pycaret`, etc.

### Phase B: PyCaret Integration (Week 2)

1. Implement `run_pycaret_ranking(X, y, task_type, cv_splits, ...)`.
2. Support walk_forward via custom `fold_generator` if PyCaret allows.
3. Integrate into `auto_tune.main()` as optional Stage 1; pass winners to Optuna.
4. Unit tests: mock PyCaret, assert ranking output format.

### Phase C: Optuna + Neural (Week 2–3)

1. Add MLPRegressor/MLPClassifier to Optuna search space.
2. Add MedianPruner for early stopping.
3. Optional: add TabNet to Optuna (behind feature flag).
4. Tests: ensure best pipeline is sklearn-compatible and saves correctly.

### Phase D: AutoGluon Integration (Week 3–4)

1. Implement `run_autogluon_fit(X, y, task_type, time_limit, ...)`.
2. Implement `AutogluonPredictorWrapper` for go-app compatibility.
3. Add AutoGluon to pipeline after Optuna; compare and pick best.
4. Document artifact layout when AutoGluon wins (new metadata key `model_type`).
5. Tests: train, save, load, predict; compare with sklearn path.

### Phase E: Auto-sklearn (Optional) (Week 4)

1. Implement optional auto-sklearn path; run in parallel with Optuna.
2. Handle multi-output regression.
3. Export best model to joblib-compatible format (or document limitations).

### Phase F: Combination Meta & Win (Week 4–5)

1. Extend `train_combination_meta` to support AutoGluon and Optuna-tuned Ridge/ElasticNet.
2. Ensure win model can be AutoGluon or MLPClassifier.
3. End-to-end tests: full pipeline for batting, win, combination_meta.

### Phase G: Documentation & Rollout (Week 5)

1. Update `docs/ml-and-training.md` with new pipeline and config.
2. Add `--fast`, `--no-autogluon`, `--no-pycaret` CLI flags.
3. Migration guide for existing users.

---

## 7. Dependencies Summary

| Package | Purpose | Required |
|---------|---------|----------|
| pycaret | Algorithm comparison | Optional (stage 1) |
| optuna | Hyperparameter tuning | Yes (already) |
| autogluon | Tabular stacking/ensembling | Optional (stage 3) |
| auto-sklearn | Full AutoML alternative | Optional |
| scikit-learn | MLPRegressor, MLPClassifier | Yes (already) |
| pytorch | TabNet backend | Optional |
| pytorch-tabnet | TabNet models | Optional |

**Suggested extras in setup/pyproject:**

- `pip install cric-flow[ml-autotune]` → pycaret, autogluon
- `pip install cric-flow[ml-autotune-full]` → + auto-sklearn, pytorch, pytorch-tabnet

---

## 8. Risk Mitigation

| Risk | Mitigation |
|------|------------|
| PyCaret preprocessing differs from ours | Use PyCaret for ranking only; rebuild pipelines with our StandardScaler |
| AutoGluon artifact incompatible with go-app | Implement wrapper; add `model_type` to metadata; update artifacts loader |
| auto-sklearn heavy / slow | Make optional; use time limit; run only when explicitly enabled |
| TabNet dependency issues | Make optional; skip if import fails |
| Multi-output regression in AutoGluon | Test per-target vs multi-output; document chosen approach |

---

## 9. Success Criteria

1. **Accuracy:** Win model AUC improves by ≥2% vs current best (RF/GB). Batting/bowling MAE improves by ≥1%.
2. **Speed:** Full pipeline (all stages) completes in &lt;35 min per (model, format) on a typical dev machine.
3. **Compatibility:** All artifacts load and predict correctly in go-app; no regression in existing flows.
4. **Configurability:** All stages can be toggled via config; `--fast` mode completes in &lt;15 min per model.

---

## 10. Appendix: File Structure After Implementation

```
ml-service/
├── ml/
│   ├── auto_tune.py              # Main orchestrator (refactored)
│   ├── auto_tune_pycaret.py      # Stage 1: PyCaret ranking
│   ├── auto_tune_optuna.py       # Stage 2: Optuna + MLP (extracted/refactored)
│   ├── auto_tune_autogluon.py    # Stage 3: AutoGluon
│   ├── auto_tune_autosklearn.py  # Optional: auto-sklearn
│   ├── autogluon_wrapper.py      # Prediction wrapper for go-app
│   ├── auto_tune_progress.py     # (unchanged)
│   └── ...
├── config.default.json           # Extended tuning config
└── requirements.in               # + pycaret, autogluon, etc.
```

---

*Document version: 1.0. Last updated: 2025-02-27.*
