# Meta-model for score combination

This document describes how to **train a learned meta-model** for combining batting, bowling, and fielding scores in team selection, and how go-app uses it.

---

## 1. Overview

**Current:** Team selection uses fixed weights (e.g. 0.45 bat, 0.40 bowl, 0.10 field) to combine normalized signals into a single player score.

**Improvement:** A Ridge meta-model learns optimal weights from historical backtest outcomes. Inputs: `(bat_score, bowl_score, field_score, is_keeper, format)`. Target: actual contribution metric from evaluate-db or backtest.

**Output:** JSON with coefficients that go-app loads via `selection.meta_model_path`.

---

## 2. Expected CSV format

Train from a CSV with columns:

| Column       | Type   | Description                                      |
|-------------|--------|--------------------------------------------------|
| bat_score   | float  | Normalized batting signal (0–1)                  |
| bowl_score  | float  | Normalized bowling signal (0–1)                  |
| field_score | float  | Normalized fielding signal (0–1)                 |
| is_keeper   | 0 or 1 | 1 if player can keep wickets                    |
| format      | string | T20, ODI, TEST, T20I                             |
| target      | float  | Actual contribution (e.g. runs/80, wickets/5, or composite) |

Scores should be normalized the same way as in go-app predict_team (see `selection.score_normalization`). The target can come from evaluate-db backtest or any outcome metric.

---

## 3. How to run

### From ml-service

```bash
cd ml-service

# Single global model
python -m ml.train_combination_meta --csv path/to/backtest_contributions.csv --out ../output/ml-service/combination_meta.json

# Per-format models (separate Ridge per format)
python -m ml.train_combination_meta --csv data.csv --out combo.json --per-format

# Adjust Ridge alpha (regularization)
python -m ml.train_combination_meta --csv data.csv --out combo.json --alpha 0.5
```

### Make target (from repo root)

```bash
make train-combination-meta CSV=path/to/backtest.csv OUT=output/ml-service/combination_meta.json
```

---

## 4. Output JSON format

### Global model

```json
{
  "bat": 0.42,
  "bowl": 0.44,
  "field": 0.10,
  "keeper_bonus": 0.02,
  "intercept": 0.01,
  "feature_names": ["bat_score", "bowl_score", "field_score", "is_keeper", "format_T20", "format_ODI", ...],
  "n_samples": 1500
}
```

Coefficients are normalized so |bat|+|bowl|+|field|+|keeper_bonus| ≈ 1.

### Per-format model (`--per-format`)

```json
{
  "per_format": {
    "T20": { "bat": 0.42, "bowl": 0.44, "field": 0.10, "keeper_bonus": 0.02, "intercept": 0.01 },
    "ODI": { "bat": 0.45, "bowl": 0.42, "field": 0.09, "keeper_bonus": 0.02, "intercept": 0.0 }
  }
}
```

---

## 5. Configuring go-app

In `go-app/config.json` under `selection`:

```json
{
  "selection": {
    "meta_model_path": "output/ml-service/combination_meta.json"
  }
}
```

Path can be absolute or relative to the working directory of the go-app process.

When `meta_model_path` is set:

- **Per-format:** If the JSON has `per_format` and the format (e.g. T20) exists, those weights are used.
- **Global:** Otherwise, the top-level `bat`, `bowl`, `field`, `keeper_bonus` are used.
- **Fallback:** If loading fails or no matching format, go-app falls back to `score_weights` or `score_weights_by_format`.

---

## 6. References

- Config: **docs/CONFIG.md** (`selection.meta_model_path`)
- Score normalization: **docs/CONFIG.md** (`selection.score_normalization`)
- Training script: `ml-service/ml/train_combination_meta.py`
