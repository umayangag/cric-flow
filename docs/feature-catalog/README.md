# Feature Catalog: Implementation Plans

This directory contains **detailed implementation plans** for the five improvements that move the system toward the three ultimate goals:

1. **Goal 1:** Select 11 players to maximize team performance (≥1 keeper, ≥5 bowlers).
2. **Goal 2:** Predict individual player performance with maximum accuracy (match date, venue, opposition, optional weather).
3. **Goal 3:** Predict the scorecard summary from predicted performance and chosen team.

Plans are **numbered by priority** (1 = highest). Weather data is not yet available; plans document where it will plug in when a source is added.

| # | Plan | Primary goal |
|---|------|--------------|
| 01 | Team selection optimization — **superseded**: the greedy scorer and its weights were deleted in P-5; selection is the XI objective in `ml/xi/optimizer.py` |
| 02 | [Individual prediction accuracy](02-individual-prediction-accuracy.md) | Goal 2 |
| 03 | [Scorecard summary for upcoming match](03-scorecard-summary-upcoming-match.md) | Goal 3 |
| 04 | [Feature pipeline consistency](04-feature-pipeline-consistency.md) | Goals 1–3 |
| 05 | [Automation and meta-model pipeline](05-automation-meta-model-pipeline.md) | Goals 1–3 |

Each plan includes: objective, current state, acceptance criteria, file-by-file implementation details, tests, edge cases, and a file checklist. Implementation proceeds one plan at a time in order.

---

## Feature contract (single source of truth)

**`configs/feature_vectors.json`** is the canonical list of feature names and order for batting, bowling, and fielding. Go and the ML service both use this contract:

- **Go:** `internal/features/contract.go` loads the file (or uses embedded defaults) and exposes `BattingFeatureNames()`, `BowlingFeatureNames()`, `FieldingFeatureNames()`. The future-match feature map is filled with any missing contract keys (value 0) so prediction always sends a consistent vector.
- **ML:** `app/feature_config.py` reads the same file for `get_feature_names("batting")` etc. `train_on_the_fly` uses these lists for column order; legacy export column names (e.g. `temp` vs `batting_temp`) are mapped via alias so training works with both.
- **Weather:** Feature names like `batting_temp`, `bowling_temp` are in the contract. Until weather data is ingested, training may see 0; prediction accepts optional `Weather` override. When a weather source is added, use the same names in training and prediction.
