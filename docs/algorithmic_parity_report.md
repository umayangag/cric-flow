# Algorithmic Parity Report — Prototype vs Implementations (In‑Progress)

Status: scaffold created. This report will capture side‑by‑side results for the golden dataset across features, per‑player predictions, team selection, and team win probability. It will be updated as we execute runs and close gaps.

## Scope
- Formats (first pass): ODI, T20 (expand to TEST, T20I after initial pass)
- Comparisons:
  - Feature engineering parity (exact or ≤ 1e‑6 float tolerance)
  - Per‑player predictions (batting/bowling): identical if same model artifacts; else ±1–3% relative tolerance with rationale
  - Team selection (constraints: ≥1 keeper, ≥4 bowlers by default) and tie‑breakers
  - Team win probability (overall)

## How to Reproduce
1. Generate exports (training + inference):
```bash
# Training
go run ./go-app/cmd/export-dataset --all-formats --out ./output/exports
# Inference (inputs‑only)
go run ./go-app/cmd/export-dataset --all-formats --inference-only --out ./output/exports
```
2. Validate schemas:
```bash
make -C ml-service validate-exports
make -C ml-service validate-exports-infer
```
3. Build parity payloads and (optionally) call ML service:
```bash
python tests/golden/run_parity.py --exports ./output/exports --formats ODI,T20 --limit 50
ML_BASE_URL=http://localhost:8000 \
python tests/golden/run_parity.py --exports ./output/exports --formats ODI,T20 --limit 50 --call
```
Artifacts are written to `tests/golden/out/`.

## Datasets
- Source exports: `output/exports/`
- Inference payloads: `tests/golden/out/batting_payload_<FMT>.json`, `tests/golden/out/bowling_payload_<FMT>.json`
- (If called) predictions: `tests/golden/out/batting_preds_<FMT>.json`, `tests/golden/out/bowling_preds_<FMT>.json`

## Parity Checks and Results

### 1) Feature Engineering Parity
- Method: compare exporter inputs vs prototype expected inputs (column by column). Tolerance: ≤ 1e‑6.
- ODI: [pending]
- T20: [pending]

Issues/Notes:
- Toss encoding parity confirmed? [pending]
- Viscosity encoding for inference is now {0,1} (NULL→0, humid→1). [done]

### 2) Per‑player Predictions (Batting)
- ODI: [pending]
- T20: [pending]

### 3) Per‑player Predictions (Bowling)
- ODI: [pending]
- T20: [pending]

### 4) Team Selection & Tie‑breakers
- Constraint checks (≥1 keeper, ≥4 bowlers): [pending]
- Selected XI parity vs prototype: [pending]

### 5) Team Win Probability
- ODI: [pending]
- T20: [pending]

## Deviations & Improvements
- Any intentional improvements vs prototype (better null handling, stricter constraints, robust encodings) will be tracked here with rationale and impact.

## Action Items
- [ ] Run ODI, T20 golden passes and populate sections above.
- [ ] Confirm toss mapping parity with prototype semantics.
- [ ] Document any differences beyond tolerance with proposed fixes.
- [ ] Expand to TEST and T20I after first pass.

## Appendix
- Export schemas: see `docs/export_schemas.md` (Training and Inference headers)
- Algorithms baseline: `docs/algorithms_baseline.md`
- Implementation map: `docs/algorithms_implementation_map.md`
