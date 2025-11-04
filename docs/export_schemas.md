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
