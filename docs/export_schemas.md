# Export Schemas → ML Input Mapping (Draft)

Goal: Ensure `go-app` dataset exports match the ML service expected inputs exactly, for modular, human-readable, and maintainable pipelines.

Sources:
- Exporter: `go-app/cmd/export-dataset/main.go`
- ML inputs: `ml-service/ml/dataset_definitions.py`

Outputs produced by exporter:
- Legacy (combined):
  - `batting_encoded.csv`
  - `bowling_encoded.csv`
- Per-format (preferred):
  - `batting_encoded_<FORMAT>.csv` (e.g., `batting_encoded_T20.csv`)
  - `bowling_encoded_<FORMAT>.csv`

Accepted format codes in exporter: `TEST`, `ODI`, `T20`, `T20I`

---

## Batting schema mapping

Expected by ML (`input_batting_columns`):
1. `batting_consistency` (float)
2. `batting_form` (float)
3. `batting_temp` (int)
4. `batting_wind` (int)
5. `batting_rain` (int)
6. `batting_humidity` (int)
7. `batting_cloud` (int)
8. `batting_pressure` (int)
9. `batting_viscosity` (int: 0=dry,1=humid)
10. `batting_inning` (int: 1|2)
11. `batting_session` (int: 1|2|3)
12. `toss` (int: 0|1)
13. `venue` (float or int code)
14. `opposition` (float or int code)
15. `season` (int)
16. `player_name` (str)

Present in exporter (by query in `exportBattingFormat`/legacy):
- Selects core outputs first: `runs, balls, fours, sixes, batting_position`
- Then features: `pcd.batting_consistency, pfd.batting_form`
- Weather: `w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, viscosity_encoded`
- Context: `inning, session_encoded, toss, venue_agg, opposition_agg, season_id, player_name`

Mapping notes:
- `viscosity_encoded`: exporter converts text viscosity to numeric (0/1) via CASE; aligns with ML `batting_viscosity`.
- `session_encoded`: exporter encodes session to 1..3; aligns with ML `batting_session`.
- `venue/opposition`: exporter uses AVG(...)::real aggregates per venue/opposition; ML expects numeric features; alignment OK.
- `season`: must be integer season id consistent with prototype semantics.
- `player_name`: should be exact string used in player table; casing consistency important.

Nullability and defaults:
- All numeric features should be non-null. Use COALESCE in queries (exporter already uses COALESCE for some fields).
- Enforce `viscosity` default to 0 when NULL (exporter CASE handles this).

---

## Bowling schema mapping

Expected by ML (`input_bowling_columns`):
1. `bowling_consistency` (float)
2. `bowling_form` (float)
3. `bowling_temp` (int)
4. `bowling_wind` (int)
5. `bowling_rain` (int)
6. `bowling_humidity` (int)
7. `bowling_cloud` (int)
8. `bowling_pressure` (int)
9. `bowling_viscosity` (int: 0=dry,1=humid)
10. `batting_inning` (int: 1|2)
11. `bowling_session` (int: 1|2|3)
12. `toss` (int: 0|1)
13. `bowling_venue` (float or int code)
14. `bowling_opposition` (float or int code)
15. `season` (int)
16. `player_name` (str)

Present in exporter (by query in `exportBowlingFormat`/legacy):
- Selects outputs first: `runs_conceded=runs, deliveries=balls, wickets, econ`
- Then features: `pcd.bowling_consistency, pfd.bowling_form`
- Weather: `w.temp, w.wind, w.rain, w.humidity, w.cloud, w.pressure, viscosity_encoded`
- Context: `inning (batting_inning), session_encoded (bowling_session), toss, venue_agg (bowling_venue), opposition_agg (bowling_opposition), season_id, player_name`

Mapping notes:
- Same encoding and aggregation patterns as batting.
- Ensure `session_encoded` maps to `bowling_session` in the final CSV header.

Nullability and defaults:
- Use COALESCE for wickets and other aggregates to avoid NULLs.

---

## File naming and headers

- Headers must match ML expectations exactly (order and spelling). The exporter should write the columns in the same order as defined above.
- For legacy exports (`*_encoded.csv`), ML can still ingest if headers match; prefer per-format files going forward.

---

## Validation procedure

1. Generate exports (example for all formats):
```
make -C go-app export-dataset ARGS="--all-formats --out ./output/exports"
```
2. Validate with ML validator (checks headers/types/nulls):
```
make -C ml-service validate-exports
```
3. If the validator fails, compare the actual CSV headers against the expected sets in this document and update the exporter queries (or ML definitions) accordingly.

---

## Open items
- Confirm any additional categorical encodings (e.g., format-specific adjustments) match prototype semantics.
- Confirm `venue/opposition` aggregates align with prototype’s feature engineering.
- Decide whether `season` should be shifted (e.g., form based on `season-1`) at export time or in ML precompute; keep responsibilities modular.


---

## Inference schema (explicit header orders)

These headers are used when emitting inputs-only CSVs for inference or when constructing inference payloads. They intentionally exclude training outputs.

- Batting inference headers (matches `tests/golden/expected_headers_batting.json`):
```
batting_consistency,
batting_form,
batting_temp,
batting_wind,
batting_rain,
batting_humidity,
batting_cloud,
batting_pressure,
batting_viscosity,
batting_inning,
batting_session,
toss,
venue,
opposition,
season,
player_name
```

- Bowling inference headers (matches `tests/golden/expected_headers_bowling.json`):
```
bowling_consistency,
bowling_form,
bowling_temp,
bowling_wind,
bowling_rain,
bowling_humidity,
bowling_cloud,
bowling_pressure,
bowling_viscosity,
batting_inning,
bowling_session,
toss,
bowling_venue,
bowling_opposition,
season,
player_name
```

Notes:
- Encodings must match exporter semantics: `session ∈ {1,2,3}`, `viscosity ∈ {0,1}` with NULL→0, and `toss ∈ {0,1}`.
- `venue`/`opposition` are numeric aggregates (AVG/normalized) and must be non-null. Use `COALESCE` in SQL.

## Training schema (explicit header orders)

These are the training CSV headers written by the exporter, with outputs leading, followed by features and identifiers. They match the golden training headers:
- Batting training headers (matches `tests/golden/expected_headers_batting_training.json`)
- Bowling training headers (matches `tests/golden/expected_headers_bowling_training.json`)

For convenience, the ordered lists are maintained in the golden JSON files and enforced by `tests/golden/compare_features.py`.

## Validation in CI

- The CI job invokes `make -C ml-service validate-exports`, which runs:
  - `ml-service/ml/validate_exports.py --all-formats --schema training --use-golden`
- This enforces exact header order for all present per-format exports and checks basic nullability.
- If you add an inference-only export mode, validate with:
```
(cd ml-service/ml && ../.venv/bin/python3 validate_exports.py --all-formats --schema inference --use-golden)
```

## Producer responsibilities (exporter)

- Maintain exact column order as documented above.
- Ensure all numeric feature columns are non-null: apply `COALESCE`/`CASE` where necessary.
- Keep categorical encodings stable and documented (session/toss/viscosity).