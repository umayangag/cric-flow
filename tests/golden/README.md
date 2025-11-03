# Golden Dataset Harness (Scaffold)

Purpose: Side-by-side parity checks between the prototype (src/) and implementations (go-app + ml-service) on a small, representative dataset. Starts with T20 + ODI; can expand to TEST + T20I.

What we compare
- Exported CSV headers and dtypes vs ML expectations (batting/bowling)
- Encoded values: session {1,2,3}, viscosity {0,1}, toss {0,1}
- Player form usage: season S uses S-1 rows
- Per-player predictions (batting/bowling)
- Team selection outcome under constraints (keeper >=1, bowlers >=4 by default)
- Team win probability

Files
- expected_headers_batting.json — ordered expected header list
- expected_headers_bowling.json — ordered expected header list
- compare_features.py — outline to load exports and assert header/type/nullability conformance

How to generate exports
```
# From repo root
GO_APP_OUT=./output/exports
mkdir -p "$GO_APP_OUT"
# Per-format export (preferred)
go run ./go-app/cmd/export-dataset --all-formats --out "$GO_APP_OUT"
# Legacy (single combined) is also supported by the exporter when no format is selected
```

How to validate with ML service validator
```
make -C ml-service validate-exports
```

Next steps
- Add small CSV slices covering diverse matches/players and commit under tests/golden/data if needed.
- Implement compare_features.py to run full checks and diff against prototype once prototype-run path is decided (read-only).
