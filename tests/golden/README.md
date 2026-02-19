# Golden export validation

Purpose: validate that go-app export CSVs match ML service expectations (headers, dtypes, encodings). Used by the ML validator and by parity/feature comparison scripts.

**What we compare**
- Exported CSV headers and dtypes vs ML expectations (batting/bowling)
- Encoded values: session {1,2,3}, viscosity {0,1}, toss {0,1}
- Optional: per-player predictions and team selection outcomes (when running full parity)

**Files**
- `expected_headers_batting.json` / `expected_headers_bowling.json` — ordered expected header list (inference)
- `expected_headers_batting_training.json` / `expected_headers_bowling_training.json` — training CSV headers
- `compare_features.py` — load exports and assert header/type/nullability conformance
- `run_parity.py` — parity checks (if used)

**How to generate exports (from repo root)**
```bash
make output-dirs
cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -unified=1
# Or per-format: go run ./cmd/export-dataset -formats=ODI,T20I
```

**How to validate with ML service**
```bash
make -C ml-service validate-exports
```
This runs `validate_exports.py` with `--all-formats --schema training --use-golden` and checks header order and nullability. See **docs/config-and-data.md** (Export schemas).
