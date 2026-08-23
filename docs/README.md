# Documentation

Reference for the cricket prediction system: architecture, configuration, APIs, ML training, and operations.

**Quick reference:** For data flow, ML model shapes, and aggregation logic in one place, see [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md) at the repo root.

---

## Reading order

| Doc | Purpose |
|-----|--------|
| [overview.md](overview.md) | Components, data flow, pipeline, how to run |
| [config-and-data.md](config-and-data.md) | go-app and ml-service config, Cricsheet import, export schemas |
| [apis-backtest-and-ops.md](apis-backtest-and-ops.md) | API contracts, backtest/evaluate, ops status |
| [ml-and-training.md](ml-and-training.md) | ML models, training pipeline, auto-tune, walk-forward, meta-model |
| [quality-and-debugging.md](quality-and-debugging.md) | Go test standards, check-all/CI, container OOM diagnosis |

---

## Active work

| Doc | Purpose |
|-----|--------|
| [CLEANUP_PR_CHECKLIST.md](CLEANUP_PR_CHECKLIST.md) | Repo-wide cleanup tracked as one PR at a time: dead code, CLI paths superseded by the API, legacy compatibility layers |
| [../ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md](../ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md) | ML service architecture improvements (P0–P4) |

---

## Cross-references

- **Feature vectors:** `configs/feature_vectors.json` (shared by go-app and ml-service). Described in [config-and-data.md](config-and-data.md) and [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).
- **Backtest / Evaluate DB:** Endpoints and flow in [apis-backtest-and-ops.md](apis-backtest-and-ops.md).
- **Team selection / Monte Carlo:** [ml-and-training.md](ml-and-training.md) and [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).
