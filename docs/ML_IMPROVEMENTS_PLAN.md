# ML Pipeline Improvements: Data Import → Prediction

This document presents a world-class ML engineer’s analysis of the cricket prediction pipeline and a step-by-step plan to improve it. It covers feature extraction, model training, and model combination toward a single final outcome.

---

## 1. Current Pipeline Summary

```
Data Import (Cricsheet/CSV)  →  Precompute (form, consistency, venue, opposition)
                              →  Export (CSV)  →  Train (batting, bowling, fielding)
                              →  Predict  →  Team Selection (weighted combination)
```

**Strengths:**
- Strict temporal cutoff (no future leakage)
- Same feature logic in export and prediction
- StandardScaler on features; raw targets
- Per-format models; auto-tune available
- Sequence features exist (optional) for bowl-by-bowl context

**Gaps Identified:**
1. Basic feature set (EWM form, CV consistency only)
2. Fixed score normalization (hardcoded 80, 5, 12)
3. Fixed combination weights (0.45 bat, 0.40 bowl, 0.10 field)
4. Single model type per skill (RandomForest only)
5. No format-specific scaling in team selection

---

## 2. Detailed Improvement Plan

### Phase 1: Feature Engineering (High Impact)

#### 2.1 Advanced Form Features
| Improvement | Description | Implementation |
|-------------|-------------|----------------|
| Multi-horizon form | EWM at different α (e.g. 0.2, 0.3, 0.5) | Add `form_short`, `form_long` columns in go-app export |
| Momentum indicator | Slope of recent performances (last 5 innings) | Compute in export snapshot |
| Form × venue interaction | `form * venue_agg` | Add in training transform |
| Log1p for skewed features | `log1p(form)` for right-skewed form | Optional in ml-service feature pipeline |

#### 2.2 Consistency Enhancements
| Improvement | Description | Implementation |
|-------------|-------------|----------------|
| Format-aware consistency | CV per format (already configurable) | Ensure `consistency_per_format` used |
| Consistency × form | Low consistency + high form = risky | Add `consistency * form` interaction |
| Rolling std/mean | Last N innings std as additional feature | Optional in go-app |

#### 2.3 Context Features
| Improvement | Description | Implementation |
|-------------|-------------|----------------|
| Session × venue | Different behaviour in day/night at venue | Add if session data available |
| Opposition strength | Opposition EWM across all batters/bowlers | Aggregate from DB |
| Sequence features | Bowl sequences, dot streaks (already computed) | Wire into training when `-enable-seq` |

**Action:** Add optional `feature_transforms` block in `ml-service/config.json`. Module `ml/feature_transforms.py` provides `get_interaction_specs`, `apply_interactions`, `extend_feature_map`. Full integration (training + prediction) requires storing interaction specs in metadata and applying at prediction—see Step 4.
```json
"feature_transforms": {
  "add_interactions": [["batting_form", "batting_venue"], ["batting_consistency", "batting_form"]]
}
```

---

### Phase 2: Model Training Improvements

#### 2.4 Model Architecture
| Improvement | Description | Implementation |
|-------------|-------------|----------------|
| Gradient Boosting option | GBM often outperforms RF for tabular | Already in auto_tune; add to main train |
| Stacked ensemble | Base: RF + GBM; meta: Ridge | New `train_batting_stacked.py` or flag in train |
| Quantile regression | Prediction intervals | Optional `QuantileRegressor` for uncertainty |
| Feature importance export | Persist and log | Add to `train_and_save` |

#### 2.5 Target Engineering
| Improvement | Description | Implementation |
|-------------|-------------|----------------|
| Per-format target scaling | T20 runs ~20–40; ODI ~25–50 | Use format-specific scaler for targets (optional) |
| Composite target | e.g. `runs + sr_weight * strike_rate` | Configurable in config |

---

### Phase 3: Model Combination (Integration)

#### 2.6 Format-Aware Score Normalization
**Current (predict_team.go):**
```go
normalizeBatScore(runs)   = min(1, runs/80)           // T20/ODI/Test same
normalizeBowlScore(w,e)   = (min(1,w/5) + max(0,1-e/12)) / 2
normalizeFieldScore(c,r)  = min(1, (c + r*1.5)/5)
```

**Improvement:** Format-specific divisors:
| Format | Bat divisor | Wickets divisor | Econ base | Field divisor |
|--------|-------------|-----------------|-----------|---------------|
| T20    | 80          | 5               | 12        | 4             |
| T20I   | 75          | 4               | 11        | 3             |
| ODI    | 100         | 5               | 6         | 5             |
| TEST   | 150         | 6               | 4         | 6             |

**Action:** Add `selection.score_normalization` in go-app config:
```json
"score_normalization": {
  "T20":  { "bat_divisor": 80, "wicket_divisor": 5, "econ_base": 12, "field_divisor": 5 },
  "ODI":  { "bat_divisor": 100, "wicket_divisor": 5, "econ_base": 6, "field_divisor": 5 },
  "TEST": { "bat_divisor": 150, "wicket_divisor": 6, "econ_base": 4, "field_divisor": 6 }
}
```

#### 2.7 Learned Combination (Meta-Model)
**Current:** Fixed weighted sum `0.45*Bat + 0.40*Bowl + 0.10*Field + keeper_bonus`.

**Improvement:** Optional meta-model that learns optimal combination from historical backtest:
- Input: `(bat_score, bowl_score, field_score, is_keeper, format)`
- Target: Actual contribution to team win (e.g. runs added, wickets impact)
- Model: Small MLP or Ridge; trained on evaluate-db outcomes

**Action:** Add `ml.combination.meta_model` (optional) – train from backtest results.

#### 2.8 Configurable Weights Per Format
**Current:** Single `score_weights` for all formats.

**Improvement:** Allow per-format overrides:
```json
"score_weights_by_format": {
  "T20":  { "bat": 0.42, "bowl": 0.44, "field": 0.10, "keeper_bonus": 0.02 },
  "ODI":  { "bat": 0.45, "bowl": 0.42, "field": 0.09, "keeper_bonus": 0.02 },
  "TEST": { "bat": 0.48, "bowl": 0.38, "field": 0.10, "keeper_bonus": 0.02 }
}
```

---

### Phase 4: Evaluation & Feedback

#### 2.9 Walk-Forward Enhancement
- Persist per-window MAE/RMSE by format
- Use for automatic weight tuning
- Feed best params back into config

#### 2.10 Calibration
- Calibrate predicted probabilities (if win model used)
- Reliability diagrams for categorical outputs

---

## 3. Implementation Order

| Step | Item | Effort | Impact | Status |
|------|------|--------|--------|--------|
| 1 | docs/ML_IMPROVEMENTS_PLAN.md | Done | — | ✅ |
| 2 | Format-aware score normalization (go-app config + predict_team) | Medium | High | ✅ |
| 3 | Feature importance export in training | Low | Medium | ✅ |
| 4 | Optional feature transforms (interactions, log1p) - full integration | Medium | Medium | ✅ |
| 5 | Multi-horizon form (form_short, form_long) in go-app export | Medium | High | ✅ |
| 6 | Momentum indicator in go-app | Low | Medium | ✅ |
| 7 | Gradient Boosting option in main train scripts | Medium | High | ✅ |
| 8 | Per-format score_weights config | Low | Medium | ✅ |
| 9 | Stacked ensemble option in auto_tune and train | High | High | ✅ |
| 10 | Meta-model for combination | High | High | ✅ |
| 11 | Calibration (ml.calibrate, reliability diagrams) | Medium | Medium | ✅ |

---

## 4. References

- Feature order: `configs/feature_vectors.json`
- Data flow: `docs/ARCHITECTURE.md`
- Normalization: `docs/ML_DATA_AND_NORMALIZATION.md`
- Config: `docs/CONFIG.md`
- Models combined: `docs/ML_MODELS_COMBINED.md`
