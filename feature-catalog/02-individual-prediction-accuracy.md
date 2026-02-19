# Plan 02: Individual Prediction Accuracy (Priority 2)

**Goal:** Predict individual player performance with maximum accuracy for an upcoming match. Inputs: match date, venue, opposition (player pool), and optionally weather forecast. **Note:** Weather data is not available yet; the plan documents where it will plug in later.

---

## 1. Objective

- Align **prediction-time features** with **training** so the model receives the same feature set (or a documented subset with consistent fill rules).
- Add **opposition strength** (or similar) features derived from `OppositionPlayerIDs` when provided.
- Document **weather**: training uses `weather_data` (currently sparse); prediction accepts optional `Weather` in the API; when a weather source is added later, document ingestion and ensure same feature names in training and prediction.

---

## 2. Current State

| Component | Location | Behavior |
|-----------|----------|----------|
| Future-match features | `go-app/internal/db/exportqueries/training_snapshot.go` | `ComputeFeaturesAtCutoffForFutureMatch`: sets form, form_short/long, momentum, consistency, venue, opposition, season, weather override. **Does not set sequence features** (bat_prev_sr, bat_window_sr_12_pp, etc.). |
| Feature vectors | `configs/feature_vectors.json` | Batting: 27 cols (consistency, form, form_short, form_long, momentum, temp, wind, rain, humidity, cloud, pressure, viscosity, inning, session, toss, venue, opposition, season, + 8 bat_* sequence). Bowling: similar + 8 bowl_* sequence. |
| ML training | `ml-service` | Uses feature_vectors.json or dataset_definitions; train_on_the_fly builds X from training-data API headers. |
| Opposition | `go-app/internal/services/predictteam/predict_team.go` | `Input.OppositionPlayerIDs` accepted but not used in feature computation. |

Gap: prediction feature map is missing the 8 batting and 8 bowling sequence features (so they default to 0 at prediction). Training may include them; we must either add them at prediction or train a model that does not depend on them for future-match.

---

## 3. Acceptance Criteria

- [ ] **Sequence features at prediction:** Either (A) add sequence features to `ComputeFeaturesAtCutoffForFutureMatch` (from precompute or 0 when no history), so the vector matches `feature_vectors.json` order and names; or (B) document that future-match prediction uses a subset and ML fills missing with 0 (current behavior), and ensure ML training uses the same fill for missing sequence cols. Prefer (A) for accuracy.
- [ ] **Opposition strength:** When `OppositionPlayerIDs` is non-empty, compute one or more features (e.g. opposition_bowling_strength, opposition_batting_strength as EWM or average of opposition players’ stats as of cutoff) and add to the feature map for batting and bowling. Training export must be extended to include these columns so train and predict stay aligned.
- [ ] **Weather:** Document in code and/or docs: training joins `weather_data`; prediction uses `WeatherOverride` when provided. No weather ingestion in this plan (user will add source later). Ensure feature names are consistent (batting_temp, bowling_temp, etc.).
- [ ] **Single feature list:** One canonical list (e.g. from feature_vectors.json) used for both Go “future match” feature build and ML; no silent mismatch in column order or names.

---

## 4. Implementation Details

### 4.1 Sequence features for future match

**File:** `go-app/internal/db/exportqueries/training_snapshot.go`

- In `ComputeFeaturesAtCutoffForFutureMatch`, after building `feats` with form, venue, opposition, season, weather:
  - Add keys for all batting sequence features from feature_vectors.json: `bat_prev_sr`, `bat_prev_out_rate`, `bat_window_sr_12_pp`, `bat_window_boundary_rate_12_pp`, `bat_entry_sr_1_6`, `bat_set_sr_13_30`, `bat_react_after_dot_sr`, `bat_after_k_dots_boundary_p_k2`.
  - Add keys for all bowling sequence features: `bowl_prev_wkt_rate`, `bowl_window_econ_24_death`, `bowl_window_wkt_rate_24_death`, `bowl_extras_wide_rate_pp`, `bowl_react_after_boundary_wkt_rate_next`, `bowl_spell_first_over_wkt_rate`, `bowl_over_ball1_wkt_rate`, `bowl_over_ball6_wkt_rate`.
  - Values: either from a new helper that computes “sequence features as of cutoff” per player/format (no match context), or 0.0 when not available. Prefer: add a function in exportqueries or features that, given playerID, formatID, cutoff, returns a map of these keys to float64 (0 if no history). That requires access to ball-by-ball or innings-level data; if that’s in seqcalc/precompute, call it. If not, use 0.0 for this plan and document “sequence features at future-match prediction are 0 until precompute exports them”.
- **Pragmatic choice for this plan:** Add the keys to the feature map with value 0.0 so that the **order and set** of keys match feature_vectors.json. This avoids ML seeing missing keys. Later, a separate task can backfill sequence feature computation for future-match.

### 4.2 Opposition strength features

**File:** `go-app/internal/db/exportqueries/training_snapshot.go` or new `go-app/internal/db/exportqueries/opposition_strength.go`

- Add function `ComputeOppositionStrength(ctx, cutoff, formatID, oppositionPlayerIDs []int64) (bowlingStrength, battingStrength float64, err error)`:
  - For each opposition player, get their bowling form (e.g. EWM of economy or wickets) and batting form (e.g. EWM of runs) as of cutoff from precomputed or DB.
  - Aggregate: e.g. average or weighted average across opposition players. Return two scalars.
- In `ComputeFeaturesAtCutoffForFutureMatch`, accept optional `oppositionPlayerIDs []int64`. When non-nil and non-empty, call `ComputeOppositionStrength` and add to `feats`:
  - e.g. `opposition_bowling_strength`, `opposition_batting_strength` (or one combined). Use names that will be added to feature_vectors.json and training export.
- **Training alignment:** The training-data API and CSV export currently do not include opposition strength; they are match-based and opposition is an ID. So either:
  - (A) Add opposition strength to training rows (compute per row from opposition team’s player list at that match), or
  - (B) Add opposition strength only at prediction and train with 0 for that column (model will learn to use it when non-zero later). Prefer (B) for this plan to avoid large export changes: add `opposition_bowling_strength` and `opposition_batting_strength` to the **prediction** feature map only, with 0 in training for now. Document that when training is extended to include these, accuracy can improve.

**Simpler variant:** Add a single feature `opposition_strength` (e.g. average opposition bowling EWM) at prediction only; training keeps 0. No change to training export in this plan.

### 4.3 Predict team: pass OppositionPlayerIDs into feature computation

**File:** `go-app/internal/services/predictteam/predict_team.go`

- When calling `exportqueries.ComputeFeaturesAtCutoffForFutureMatch`, pass `input.OppositionPlayerIDs` (or the resolved list of opposition team’s pool IDs). So the signature of `ComputeFeaturesAtCutoffForFutureMatch` must accept an optional slice of opposition player IDs; if provided, compute opposition strength and add to feats.

### 4.4 Feature list and ML alignment

**File:** `configs/feature_vectors.json`

- Ensure batting and bowling lists include the exact keys used in `ComputeFeaturesAtCutoffForFutureMatch` (including sequence and, if added, opposition_strength). Order must match what the ML service expects (same order as training data headers).

**File:** `ml-service/app/feature_config.py` or equivalent

- Ensure prediction-time feature building (when building the vector from the Go-provided map) uses the same order as feature_vectors.json. If the ML service receives a map, it should serialize to the list in feature_vectors order.

### 4.5 Documentation

- Document: “Future-match prediction features include form, venue, opposition, season, optional weather override, and sequence features (0 when not precomputed). Opposition strength can be passed via OppositionPlayerIDs. Weather data is not yet ingested; when available, it will be used in training and prediction.”
- Document feature_vectors.json as the single source of truth for feature order and names for both Go and ML.

---

## 5. File Checklist

| File | Action |
|------|--------|
| `go-app/internal/db/exportqueries/training_snapshot.go` | Add sequence feature keys (0.0) and optional opposition strength to ComputeFeaturesAtCutoffForFutureMatch; extend signature for opposition player IDs. |
| `go-app/internal/db/exportqueries/opposition_strength.go` | New (optional): ComputeOppositionStrength; or inline a simple version in training_snapshot. |
| `go-app/internal/services/predictteam/predict_team.go` | Pass OppositionPlayerIDs into ComputeFeaturesAtCutoffForFutureMatch. |
| `configs/feature_vectors.json` | No change if keys already match; else add opposition_* if we add them. |
| Docs | Add short section on future-match features and weather. |

---

## 6. Weather

- No new weather ingestion. Training continues to join `weather_data` (may be empty). Prediction continues to accept optional `Weather` in the API. When a weather source is added later, document that the same feature names (batting_temp, etc.) must be used in training and prediction.
