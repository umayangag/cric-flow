# Evaluate DB: Data fillers and non-pipeline values — audit

This document lists every place in the Evaluate DB pipeline where we **fill or default** unavailable data with averages, fixed stats, zeros, or random values. The goal is to analyse prediction accuracy of the **full** process: data import → precompute (features + sequence) → training → prediction. Any filler that is not derived from the same pipeline logic distorts that analysis.

---

## Pipeline stages (in order)

1. **Data import** (Cricsheet/ETL)
2. **Precompute** (feature snapshots: form, consistency, venue, opposition; then sequence features)
3. **Training data export** (for train-on-the-fly or offline training)
4. **Feature computation at cutoff** (go-app: features sent to ML for a specific match)
5. **ML: build feature vectors** (from go-app feature map → model inputs)
6. **ML: train** (on exported rows or train-on-the-fly)
7. **ML: predict** (loaded artifacts or train-on-the-fly)

---

## 1. Data import

| Location | What is filled | How | Impact on Evaluate DB |
|----------|----------------|-----|------------------------|
| `go-app/internal/db/repo_match.go` — `EnsureMatchByID` | `match_date`, `format_id`, `original_match_type` | Placeholder: `'1970-01-01'`, format T20 or 1, `'unknown'` when ensuring match by ID only | Used for flows that ensure match exists without full match data; not on the main evaluate path. |
| `go-app/internal/cricsheet/ingest.go` | `balls_per_over` | Default `6` when `info.BallsPerOver <= 0` | Minor; only when Cricsheet omits it. |
| Importer options | Weather / fielding | `placeholders-weather` inserts empty weather rows; `placeholders-fielding` inserts **zeroed fielding rows** for all players | If enabled, fielding/weather are explicit placeholders (zeros) until backfilled. Affects training rows and any feature that uses weather/fielding. |

**Recommendation:** Document that placeholder options (weather/fielding) mean “no real data yet”; avoid using them for accuracy analysis until backfilled. `EnsureMatchByID` is for edge cases, not evaluate.

---

## 2. Precompute (feature and sequence)

| Location | What is filled | How | Impact on Evaluate DB |
|----------|----------------|-----|------------------------|
| `go-app/internal/features/features.go` — `Consistency` | When `mean == 0` | Uses `mean = 1` to avoid division by zero when computing coefficient of variation | Numerical safeguard only; not a data filler. |
| `go-app/internal/features/features_fmt.go` | Strike rate, runs, etc. in formulas | `COALESCE(bd.strike_rate,0)`, `COALESCE(bd.runs,0)` etc. in SQL | Converts NULL to 0 **inside** the formula input. Acceptable if NULL means “no value” in source. |
| Precompute **snapshot** writes | — | Precompute writes EWM/Consistency/venue/opposition into `feature_*_snapshots`; no fallback there | Training **export** was fixed to compute form/consistency/venue/opposition on the fly (EWM/Consistency) when snapshots are missing. Other exports below still use zero. |

**Recommendation:** Precompute itself is consistent. The only remaining issue is **who reads** those snapshots vs who computes on the fly (see “Feature at cutoff” and “Exports” below).

---

## 3. Training data export (go-app → ML)

| Location | What is filled | How | Impact on Evaluate DB |
|----------|----------------|-----|------------------------|
| **BattingTrainingRows / BowlingTrainingRows** | Form, consistency, venue, opposition | **Fixed:** Now computed on the fly with **EWM + Consistency** (same as precompute). No AVG/zero fallback. | Train-on-the-fly uses pipeline-consistent values. |
| **Other exports** (unified/format, e.g. BattingFormatRows, BowlingUnifiedRows, CSV exports that join snapshots) | Form, consistency, venue, opposition | `COALESCE(tf.batting_form, 0)`, `COALESCE(tc.batting_consistency, 0)`, `COALESCE(tvv.batting_venue, 0)`, `COALESCE(tvo.batting_opposition, 0)` (and bowling analogues) | When snapshot is missing, **0** is used. So offline CSV exports and any consumer of those exports still see zeros for missing snapshots. |
| Weather, fielding, inning, toss, season_id, venue_id, opposition_id | NULL → value | `COALESCE(w.temp, 0)`, `COALESCE(fd.catches,0)`, `COALESCE(mi.inning_number, 1)`, etc. | Weather: 0 when no weather row. Fielding: 0 when no fielding row. Inning/toss/season: sensible single-value defaults. For **accuracy analysis**, weather/fielding zeros are only correct if we never use real weather/fielding in the model. |
| `exportqueries` raw training query | venue_id, opposition_id | `COALESCE(m.venue_id, 0)`, `COALESCE(mi.bowling_team_opposition_id, 0)` (and batting opposition) | Converts NULL to 0 for use as scope ID; on-the-fly computation then uses “no scope” when 0. Not a statistical filler. |

**Recommendation:** Training rows for train-on-the-fly are now correct (EWM/Consistency on the fly). For **offline** training from CSV/unified exports, either (a) run precompute so snapshots exist, or (b) introduce the same on-the-fly computation for those export paths so they never emit 0 for form/consistency/venue/opposition when snapshot is missing.

---

## 4. Feature computation at cutoff (go-app → ML for evaluate)

This is the **critical path** for Evaluate DB: for each match we need features **as of the match date** to send to the ML service.

| Location | What is filled | How | Impact on Evaluate DB |
|----------|----------------|-----|------------------------|
| **Evaluate path (matchID &gt; 0)** — `getBacktestFeaturesAtCutoffFunc` → `exportqueries.ComputeFeaturesAtCutoffForMatch` | Form, consistency, venue, opposition | **Match context** from `db.GetMatchFeatureContext(matchID)` (format, venue, season, per-player batting/bowling opposition IDs). Then **EWM + Consistency** at cutoff via `computeBattingSnapshotAtCutoff` / `computeBowlingSnapshotAtCutoff` (same as training). **Missing history → 0.** Weather: **0** when not available (averages for missing weather are allowed elsewhere; here we use 0 for consistency with “no filler” for other features). | Evaluate uses the **same** semantics as training and precompute. No averages as stand-in for form; opposition and venue use **correct** values from the match. |
| **Legacy / tests (matchID == 0)** — `db.DefaultFeatureProviderInst.GetPlayerFeaturesAtCutoff` | Fallback | AVG(runs), AVG(wickets), AVG(econ) only. | Used when no match context (e.g. tests that don’t need venue/opposition). |

**Recommendation:** For accuracy analysis always use the evaluate path with a real match (matchID &gt; 0) so features come from `ComputeFeaturesAtCutoffForMatch`. No further change required for form/consistency/venue/opposition.

---

## 5. ML: building feature vectors from the feature map

| Location | What is filled | How | Impact on Evaluate DB |
|----------|----------------|-----|------------------------|
| **`ml-service/app/backtest_service.py`** — `build_batting_features_from_map` | Missing keys in `feature_map` | `_float(d, "batting_consistency", 0.5)`, `_float(d, "batting_form", _float(d, "avg_runs", 0.0))`, `_int(d, "batting_temp", 25)`, `venue=_float(d, "venue", 0.5)`, `opposition=_float(d, "opposition", 0.5)`, etc. | When go-app does **not** send a key, ML uses: form → avg_runs then 0; consistency → 0.5; weather → 25, 0, 50, 0, 0; venue/opposition → 0.5. So **hard-coded defaults** for missing features. |
| **`build_bowling_features_from_map`** | Same idea | `_float(d, "bowling_consistency", 0.5)`, `_float(d, "bowling_form", _float(d, "avg_wickets", 0.0))`, bowling_venue/opposition → venue/opposition then 0.5. | Same as batting: **defaults** when key is missing. |

**Recommendation:** Once go-app sends a **complete** feature map (form, consistency, venue, opposition, and any weather/context used in training), ML should not need to fill. Optionally, ML could **reject** or **warn** when required keys are missing instead of silently substituting 0.5 / 0 / 25 etc., so we don’t hide gaps.

---

## 6. ML: training (train-on-the-fly and offline)

| Location | What is filled | How | Impact on Evaluate DB |
|----------|----------------|-----|------------------------|
| **`ml-service/app/train_on_the_fly.py`** | `bowling_session` | `fillna(0)` for `bowling_session` column | Single column; keeps rows that would otherwise be dropped. Acceptable if “no session” is encoded as 0 in the pipeline. |
| **`train_on_the_fly`** | Rows with NaN in feature columns | `dropna(subset=BATTING_FEATURE_COLS)` / `BOWLING_FEATURE_COLS` | Rows with missing **required** features are **dropped**, not filled. So no hidden filler there. |
| **`ml-service/ml/train_batting_model.py`** / **`train_bowling_model.py`** | Any NaN in feature matrix | `X = X.fillna(0.0)` | **All** missing feature values are set to **0**. Used for offline training from CSV. So any column that is NaN (e.g. missing snapshot in an export that uses COALESCE(..., 0)) becomes 0 again; if the export already used 0 for missing snapshots, this is consistent but still “zero filler”. |
| **`ml-service/ml/fill_missing_attributes.py`** | Missing attributes in player pool | `fillna(column.mean())` for some columns, then `fillna(0)` | Used by `export_pool` (player pool), not by the backtest training data path. Document for completeness. |

**Recommendation:** Train-on-the-fly already avoids fill for core features (dropna). Offline training’s `X.fillna(0.0)` is consistent with exports that emit 0 for missing snapshot columns; to remove zero-filler entirely, exports and training should either have no missing values (precompute + on-the-fly form/consistency/venue/opposition) or use a strict “required columns present” check and fail instead of filling.

---

## 7. ML: prediction and match aggregates

| Location | What is filled | How | Impact on Evaluate DB |
|----------|----------------|-----|------------------------|
| **`ml-service/app/main.py`** — backtest predict | Player predictions when format/features missing | **Rejected:** Returns 400 `FORMAT_AND_FEATURES_REQUIRED`. No baseline for **player** predictions. | Evaluate DB always sends format + features; **no random baseline** on the player path. |
| **Match aggregates (evaluate path)** — `populateMatchAggregatesAndMetrics` in go-app | Predicted runs, wickets, extras, winner | **Derived from player predictions:** sum of predicted runs and wickets across `resp.Players`; winner from team run totals (via `db.GetMatchPlayerTeams`). **Extras** = 0 (not predicted per player). **No call** to `mlBacktestPredictMatchAggregatesFunc` on the evaluate path. | Evaluate uses **no baseline/RNG** for match aggregates; predicted totals come from the same player predictions used for the scorecard. |
| **`ml-service/app/backtest_service.py`** — `predict_match_baseline` | Team-level match aggregate | **Deterministic RNG** when only teams provided. | **Not** used for Evaluate DB “evaluate selected match”; only for other backtest flows that request match-level prediction without player_ids. |

**Recommendation:** No change needed; evaluate path uses real data only (player preds → aggregates).

---

## 8. Other COALESCE / defaults (not on evaluate path)

- **Actuals in backtest:** `COALESCE(SUM(bd.runs), 0)` etc. in `getBacktestPlayerActualsForMatchFunc`: correct handling of LEFT JOIN (no row → 0 runs).
- **Upserts (batting_data, bowling_data, fielding_data):** `COALESCE(EXCLUDED.x, table.x)` on conflict: keep existing value when new value is NULL. Not a statistical filler.
- **Scorecard / display:** `COALESCE(m.match_date::text, '')`, `COALESCE(p.player_name, '')`: display only.

---

## Summary: pipeline consistency (actual behaviour)

1. **Feature computation at cutoff (go-app)**  
   **Done.** When `matchID > 0`, the evaluate path uses **`exportqueries.ComputeFeaturesAtCutoffForMatch`**: match context from `db.GetMatchFeatureContext` (format, venue, season, per-player batting/bowling opposition), then **EWM form**, **Consistency**, and **venue/opposition** at cutoff (same logic as `training_snapshot.go`). Missing history → **0**. Weather → 0 when not available (averages allowed elsewhere). No averages as stand-in for form.

2. **Match aggregates (evaluate path)**  
   **Done.** Predicted runs, wickets, and winner are **derived from player predictions** (sum of runs by team, sum of wickets; winner from team run comparison). No call to the ML match-aggregates baseline; no RNG.

3. **ML feature builders**  
   **Current:** Missing keys are filled with 0.5, 0, 25, etc.  
   **Target:** Go-app now sends a full feature map on the evaluate path; ML defaults only apply if a key is omitted. Optionally document or warn when required keys are missing.

4. **Other exports (unified/format CSV)**  
   **Current:** `COALESCE(tf.batting_form, 0)` etc. when snapshot is missing.  
   **Target:** For accuracy analysis, either run precompute so snapshots exist, or use on-the-fly EWM/Consistency for those exports.

5. **Placeholder options at import**  
   Document that `placeholders-fielding` / `placeholders-weather` are for “structure only” and not for accuracy analysis until backfilled.

---

## Next steps (optional)

- **Priority 1:** ML: fail or warn when required feature keys are missing instead of silently substituting.
- **Priority 2:** Optionally extend **unified/format exports** to use on-the-fly snapshot computation.
- **Priority 3:** Document placeholder import options when running evaluate accuracy analysis.
