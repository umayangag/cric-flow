# Plan 04: Data Processing and Feature Pipeline Consistency (Priority 4)

**Goal:** Single source of truth for feature names and order; consistent behavior between training export, training-data API, and prediction-time feature build. Weather is documented for future use; no weather ingestion in this plan.

> **Superseded in part (P-5).** Every file-level instruction below that names
> `ComputeFeaturesAtCutoffForFutureMatch` describes a prediction-time feature build that no
> longer exists: it and its opposition-strength helper were deleted with the rest of go-app's
> windowed-form path. Prediction-time features are built by the XI layer in ml-service; go-app
> supplies the pool and the fixture. The objective and the feature-contract reasoning still
> hold — the Go call sites do not.

---

## 1. Objective

- Treat **configs/feature_vectors.json** (or one canonical list in Go) as the single source of truth for “what we send to the model.”
- Ensure Go training-data API and CSV export emit columns in that order with consistent names.
- Ensure `ComputeFeaturesAtCutoffForFutureMatch` and backtest feature build produce exactly that set (or a documented subset with same fill rules).
- ML (`dataset_definitions.py`, `train_on_the_fly.py`, `feature_config.py`) reads or mirrors that contract so training and prediction never diverge.
- Document weather: how `weather_data` will be populated later; for now training may have all 0s for weather; prediction accepts override.

---

## 2. Current State

| Component | Location | Behavior |
|-----------|----------|----------|
| Feature list | `configs/feature_vectors.json` | Batting 27 cols, bowling 27 cols, fielding 14 cols. Names like batting_consistency, batting_temp, bat_prev_sr, etc. |
| Training data API | `go-app/internal/server/backtest_handlers.go` | Returns batting/bowling/fielding headers + rows from exportqueries (BattingTrainingRowsWithFormat, etc.). |
| Export | `go-app/internal/db/exportqueries/batting.go`, bowling.go | Build rows with columns; order may follow SQL or a manual list. |
| Future-match features | `go-app/internal/db/exportqueries/training_snapshot.go` | ComputeFeaturesAtCutoffForFutureMatch builds a map; keys may not match feature_vectors.json order or full set. |
| ML | `ml-service/ml/dataset_definitions.py`, `train_on_the_fly.py` | train_on_the_fly uses hardcoded BATTING_FEATURE_COLS with names like "temp", "wind"; feature_vectors uses "batting_temp", "batting_wind". Potential name mismatch. |

Gaps: (1) Export/API header order might not exactly match feature_vectors.json. (2) Go may use "venue" while JSON has "venue" under batting; need to confirm batting_venue vs venue. (3) ML Python may use "temp" vs "batting_temp". We need one canonical list and align everyone to it.

---

## 3. Acceptance Criteria

- [ ] **Canonical feature list:** One place defines the ordered list of feature names for batting and bowling (and fielding). Recommendation: keep `configs/feature_vectors.json` as that place.
- [ ] **Go export and API:** Batting and bowling training rows (headers + row values) use the exact same names and order as in feature_vectors.json. Any column that doesn’t exist in DB/export is filled with 0 or a documented default.
- [ ] **Go prediction:** ComputeFeaturesAtCutoffForFutureMatch and any backtest feature builder output a map (or slice) whose keys/order match feature_vectors.json. ML service, when it receives a map, should iterate in feature_vectors order when building the prediction vector.
- [ ] **ML training:** train_on_the_fly and batch training scripts use the same column names as feature_vectors.json (e.g. batting_temp not temp for batting). If the API returns headers from Go, ML uses those headers as-is for column order.
- [ ] **Documentation:** Add a short “Feature catalog” or “Feature contract” section: feature_vectors.json is the source of truth; Go and ML must align; weather columns are present but may be 0 until weather data is available.

---

## 4. Implementation Details

### 4.1 Load feature_vectors.json in Go

**File:** `go-app/internal/features/contract.go` or `go-app/internal/config/feature_vectors.go` (new)

- Add a loader that reads `configs/feature_vectors.json` (path from env or relative to repo root) and parses:
  - `Batting []string`
  - `Bowling []string`
  - `Fielding []string`
- Export: `BattingFeatureNames() []string`, `BowlingFeatureNames() []string`, `FieldingFeatureNames() []string`. Cache after first load. If file missing, return a built-in default list that matches current feature_vectors.json so the app doesn’t depend on file at runtime if needed.

### 4.2 Export queries: use canonical order

**File:** `go-app/internal/db/exportqueries/batting.go` (and bowling, fielding)

- Ensure the **headers** returned for training rows (for API and CSV export) are exactly the list from the canonical source (feature_vectors.json). So when building the row, iterate over `features.BattingFeatureNames()` and for each name, fill the value from the computed row (or 0 if not available). This may require refactoring the current row build to be a map or struct that is then serialized in canonical order.
- Same for bowling and fielding.

### 4.3 Training snapshot and future-match

**File:** `go-app/internal/db/exportqueries/training_snapshot.go`

- ComputeFeaturesAtCutoffForFutureMatch already builds a map. Ensure every key in feature_vectors.json (batting and bowling) is present in that map (add missing keys with 0). So when ML or the backtest client builds the vector, they can iterate BattingFeatureNames() and take feats[name] for each.
- No change to backtest feature build if it already produces the same keys; otherwise align.

### 4.4 ML service

**File:** `ml-service/ml/dataset_definitions.py` or `ml-service/app/feature_config.py`

- Ensure the list of input columns for batting and bowling matches feature_vectors.json. If the training-data API returns headers, use those; otherwise load feature_vectors.json in Python and use that order. Prefer: ML uses the **headers from the API response** as the source of truth for that request, so Go is the single source; then Go must emit headers from feature_vectors.json.
- **File:** `ml-service/app/train_on_the_fly.py`: Replace hardcoded BATTING_FEATURE_COLS / BOWLING_FEATURE_COLS with the list from the training-data response headers, or load from feature_vectors.json. This way column order is never out of sync.

### 4.5 configs/feature_vectors.json

- Keep as-is. Optionally add a short comment in the repo (e.g. README in configs or in docs) that this file is the feature contract for both Go and ML.
- Ensure keys use the same naming as in Go (e.g. batting_venue, batting_opposition, season; and batting_temp, etc.). Current JSON uses "venue", "opposition", "season" under batting — confirm Go uses "venue" or "batting_venue" in the map. If Go uses "batting_venue", feature_vectors should have "batting_venue". Align so one convention is used everywhere.

### 4.6 Documentation

**File:** `docs/feature-catalog/README.md`

- State: feature_vectors.json is the single source of truth for feature names and order. Go export, training-data API, and prediction-time feature build all use this order and these names. ML training and prediction use the same. Weather features (batting_temp, etc.) are included; until weather data is ingested, they may be 0 in training.

---

## 5. File Checklist

| File | Action |
|------|--------|
| `go-app/internal/features/contract.go` or `config/feature_vectors.go` | New: load feature_vectors.json, export BattingFeatureNames(), BowlingFeatureNames(), FieldingFeatureNames(). |
| `go-app/internal/db/exportqueries/batting.go` (bowling, fielding) | Emit headers and row values in canonical order from contract. |
| `go-app/internal/db/exportqueries/training_snapshot.go` | Ensure future-match map has all keys from contract; fill missing with 0. |
| `ml-service/app/feature_config.py` or dataset_definitions | Use API headers or feature_vectors.json for column order; align names. |
| `ml-service/app/train_on_the_fly.py` | Use headers from API or feature_vectors for column order. |
| `docs/feature-catalog/README.md` | Document feature contract and weather. |

---

## 6. Weather

- No weather ingestion. Document that weather_data is joined in export; when a source is added, same feature names (batting_temp, etc.) must be used. Prediction already supports WeatherOverride.
