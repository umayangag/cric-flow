# Roadmap: improvements toward system goals

Planned improvements that move the system toward:

1. **Goal 1:** Select 11 players to **maximize team performance** (≥1 keeper, ≥5 bowlers).
2. **Goal 2:** **Predict individual player performance** with maximum accuracy (match date, venue, opposition, optional weather).
3. **Goal 3:** **Predict the scorecard summary** from predicted player performance and chosen XIs.

---

## Summary

| # | Improvement | Primary goal | Effort |
|---|-------------|--------------|--------|
| 1 | Combinatorial team selection (or meta-model by default) | Goal 1 | Medium–High |
| 2 | Feature alignment, weather, opposition features | Goal 2 | Medium |
| 3 | Single feature contract, weather ingestion, sequence at prediction | Goals 1–3 | Medium |
| 4 | Automate meta-model, walk-forward, one-command retrain+evaluate | Goals 1–3 | Low–Medium |

Implementing **4** gives immediate value with limited risk. **2** and **3** improve prediction accuracy. **1** makes selection truly optimize performance.

---

## 1. Team selection: optimization or meta-model

**Current:** Greedy selection by fixed weighted score (bat/bowl/field/keeper), then constraint swaps.

**Options:**  
- **A:** Constrained optimization over valid XIs (maximize expected outcome; constraints: size 11, ≥1 keeper, ≥5 bowlers).  
- **B:** Keep greedy + swap but use **learned weights** by default via `selection.meta_model_path` from `train_combination_meta` ([ml-and-training.md](ml-and-training.md)).

**Touch:** `go-app/internal/services/teamselect/select.go`, config (`EffectiveScoreWeightsForFormat`, `use_optimizer`).

---

## 2. Individual prediction accuracy

**Current:** Form, consistency, venue, opposition, season, weather. Future-match prediction does **not** include sequence features (e.g. `bat_prev_sr`, `bat_window_sr_12_pp`); training export does. Opposition player pool not yet used for features.

**Improvements:**  
- Align prediction-time features with training: add sequence features to `ComputeFeaturesAtCutoffForFutureMatch` or define a reduced feature set and train accordingly.  
- Weather: keep `weather_data` in export; document optional `Weather` in team-selection API.  
- Use `OppositionPlayerIDs` to compute opposition strength / matchup features.

**Touch:** `go-app/internal/db/exportqueries/training_snapshot.go`, `configs/feature_vectors.json`, ML `dataset_definitions.py` / `train_on_the_fly.py`.

---

## 3. Feature pipeline consistency

**Improvements:**  
- **Single contract:** `configs/feature_vectors.json` (or one canonical list) as source of truth; go-app export and training-data API emit same order/names; `ComputeFeaturesAtCutoffForFutureMatch` and backtest use same set (or documented subset + fill rules).  
- **Weather:** Document how `weather_data` is populated; consider placeholders or backfill for training variance.  
- **Sequence at prediction:** Precompute or compute on demand sequence features as-of cutoff and feed into future-match feature map; or maintain a reduced model for future-match only.

**Touch:** `configs/feature_vectors.json`, export queries, `training_snapshot.go`, `ml-service/ml/dataset_definitions.py`, `feature_config.py`.

---

## 4. Pipeline and meta-model automation

**Improvements:**  
- **Meta-model:** After backtest evaluate (or batch job), produce contributions CSV → run `train_combination_meta` → set `selection.meta_model_path`.  
- **Pipeline:** Optional walk-forward / evaluate step; optionally “promote” artifacts when metrics exceed threshold.  
- **One-command:** Script/Make target: import → precompute → export → train all → (optional) evaluate → report MAE/winner accuracy.

**Touch:** backtest export-contributions job, pipeline handlers, `train_combination_meta.py`, Makefile/docs.
