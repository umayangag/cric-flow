# Documentation

Reference for the cricket prediction system: architecture, configuration, APIs, ML training, and operations.

**Start here:** [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md) at the repo root — data flow, ML model shapes, and aggregation in one place. Its model shapes and endpoint tables are **generated** (`make gen-architecture-map`), so they cannot drift from the code.

---

## Reference

Descriptions of how the system works now. These should always be true of `main`.

| Doc | Purpose |
|-----|--------|
| [overview.md](overview.md) | Components, data flow, pipeline, how to run |
| [config-and-data.md](config-and-data.md) | go-app and ml-service config, Cricsheet import, dataset acquisition |
| [apis-backtest-and-ops.md](apis-backtest-and-ops.md) | API contracts, backtest/evaluate, ops status |
| [ml-and-training.md](ml-and-training.md) | The XI layer: the rating pass, the win models, the performance model, the simulator, and the L4 harness |
| [observability.md](observability.md) | Logging, metrics, health and artifact endpoints, tracing |
| [quality-and-debugging.md](quality-and-debugging.md) | Go test standards, check-all/CI, container OOM diagnosis |
| [SUPPRESSIONS.md](SUPPRESSIONS.md) | Every lint/scanner suppression in the repo, with its justification |

## Active work

Plans with open items. Check these before starting related work.

| Doc | Purpose |
|-----|--------|
| [CONSUMER_SURFACES_PR_CHECKLIST.md](CONSUMER_SURFACES_PR_CHECKLIST.md) | Streamline Workbench, Evaluate and Prediction; remove dead model-selection paths (**19 of 21 done**; W3-4 and W4-2 are `partly`, each with its blocker recorded in the row) |
| [WIN_PROB_SELECTION_PR_CHECKLIST.md](WIN_PROB_SELECTION_PR_CHECKLIST.md) | Select the XI that maximises win probability — **complete**: S-6 shipped in P-5, and every other S-item is done, cancelled or superseded. Kept for the defect record |
| [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md) | **Re-architecture of the ML pipeline**: verdict on model health with numbers, target architecture (one as-of pass, XI win model, distributional performance model, derived simulator), keep/remove table, experiments E1–E7, migration P-0…P-7 (**P-0…P-5 done; P-6 and P-7 open**) |
| [ML_PLAN_optimal_xi.md](ML_PLAN_optimal_xi.md) | The original first-principles feature and modelling plan from the dataset; partly implemented by S-10, the rest carried by the re-architecture plan |
| [IDENTITY_PR_CHECKLIST.md](IDENTITY_PR_CHECKLIST.md) | Player and team identity: 163 names hold 348 people, 130 team names span both genders (**I-1…I-5 open**). Blocks S-7; run after S-3c |
| [feature-catalog/README.md](feature-catalog/README.md) | Five numbered feature plans driving the three product goals |
| [../ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md](../ml-service/docs/IMPROVEMENT_PR_CHECKLIST.md) | ML service architecture improvements, P0–P4 (**9 of 22 done**) |

## Decisions and completed work

Kept for the reasoning, not as instructions. Nothing here is a to-do list.

| Doc | Purpose |
|-----|--------|
| [CLEANUP_PR_CHECKLIST.md](CLEANUP_PR_CHECKLIST.md) | The repo-wide cleanup, one PR per item — **complete**. Records what was removed and, more usefully, what was deliberately kept and why |
| [OPS_CONSOLE_PR_CHECKLIST.md](OPS_CONSOLE_PR_CHECKLIST.md) | Running the pipeline from the frontend, one PR per item — **complete**. Records the acquisition defences, the progress channel, and the bugs each phase uncovered |
| [weather-not-implemented.md](weather-not-implemented.md) | Why weather is absent from the models, what remains in the schema, and what building it would take |
| [audit-oop.md](audit-oop.md) | One-off design audit (March 2026) |
| [audit-coding-principles-remediation.md](audit-coding-principles-remediation.md) | Remediation tracked from that audit |

---

## Cross-references

- **Feature contract:** `ml-service/ml/xi/contract.py` — the XI columns, the display columns and the target. The XI layer computes its own as-of features from the event store; there is no shared feature-vector file and no export contract (P-6). Described in [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).
- **Backtest / Evaluation report:** Endpoints and flow in [apis-backtest-and-ops.md](apis-backtest-and-ops.md).
- **Team selection and the match simulator:** [ml-and-training.md](ml-and-training.md) and [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

## Generated files — do not hand-edit

| File | Regenerate with | Guarded by |
|------|-----------------|-----------|
| The marked blocks of [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md) | `make gen-architecture-map` | `make gen-architecture-map-check` |

It runs in the `Docs consistency` workflow on every PR that touches its inputs.
