# Documentation

Reference for the cricket prediction system: architecture, configuration, APIs, ML training, and operations.

**Start here:** [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md) at the repo root — data flow, ML model shapes, and aggregation in one place. Its model shapes and endpoint tables are **generated** (`make gen-architecture-map`), so they cannot drift from the code.

---

## Reference

Descriptions of how the system works now. These should always be true of `main`.

| Doc | Purpose |
|-----|--------|
| [overview.md](overview.md) | Components, data flow, pipeline, how to run |
| [config-and-data.md](config-and-data.md) | go-app and ml-service config, Cricsheet import, export schemas |
| [apis-backtest-and-ops.md](apis-backtest-and-ops.md) | API contracts, backtest/evaluate, ops status |
| [ml-and-training.md](ml-and-training.md) | ML models, training pipeline, auto-tune, walk-forward, meta-model |
| [match-schema.md](match-schema.md) | Database schema: matches, innings, deliveries, and the derived tables |
| [observability.md](observability.md) | Logging, metrics, health and artifact endpoints, tracing |
| [quality-and-debugging.md](quality-and-debugging.md) | Go test standards, check-all/CI, container OOM diagnosis |
| [SUPPRESSIONS.md](SUPPRESSIONS.md) | Every lint/scanner suppression in the repo, with its justification |

## Active work

Plans with open items. Check these before starting related work.

| Doc | Purpose |
|-----|--------|
| [../model-harmony-plan.md](../model-harmony-plan.md) | Cross-model consistency: the strategic plan (**65 items open**) |
| [../model-harmony-implementation-plan.md](../model-harmony-implementation-plan.md) | Its implementation companion (**6 open**). Note the correction banner: `[x]` there means *designed and prototyped*, not in use |
| [OPS_CONSOLE_PR_CHECKLIST.md](OPS_CONSOLE_PR_CHECKLIST.md) | Run the whole pipeline from the frontend: acquisition, extraction, structured progress, chaining (**6 items open**) |
| [CONSUMER_SURFACES_PR_CHECKLIST.md](CONSUMER_SURFACES_PR_CHECKLIST.md) | Streamline Workbench, Evaluate and Prediction; remove dead model-selection paths (**17 items open**) |
| [../feature-catalog/README.md](../feature-catalog/README.md) | Five numbered feature plans driving the three product goals |
| [../ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md](../ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md) | ML service architecture improvements (P0–P4) |

## Decisions and completed work

Kept for the reasoning, not as instructions. Nothing here is a to-do list.

| Doc | Purpose |
|-----|--------|
| [CLEANUP_PR_CHECKLIST.md](CLEANUP_PR_CHECKLIST.md) | The repo-wide cleanup, one PR per item — **complete**. Records what was removed and, more usefully, what was deliberately kept and why |
| [weather-not-implemented.md](weather-not-implemented.md) | Why weather is absent from the models, what remains in the schema, and what building it would take |
| [rollout-reconciliation.md](rollout-reconciliation.md) | How the innings reconciliation path was rolled out |
| [stage3-joint-modelling.md](stage3-joint-modelling.md) | Joint-modelling investigation and its outcome |
| [audit-oop.md](audit-oop.md) | One-off design audit (March 2026) |
| [audit-coding-principles-remediation.md](audit-coding-principles-remediation.md) | Remediation tracked from that audit |

---

## Cross-references

- **Feature vectors:** `configs/feature_vectors.json` (shared by go-app and ml-service). Described in [config-and-data.md](config-and-data.md) and [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).
- **Backtest / Evaluate DB:** Endpoints and flow in [apis-backtest-and-ops.md](apis-backtest-and-ops.md).
- **Team selection / Monte Carlo:** [ml-and-training.md](ml-and-training.md) and [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

## Generated files — do not hand-edit

| File | Regenerate with | Guarded by |
|------|-----------------|-----------|
| The marked blocks of [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md) | `make gen-architecture-map` | `make gen-architecture-map-check` |
| `.junie/skills/` (from `.cursor/skills/`) | `make sync-skills` | `make sync-skills-check` |

Both run in the `Docs consistency` workflow on every PR that touches their inputs.
