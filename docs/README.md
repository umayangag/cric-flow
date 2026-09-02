# Documentation

Reference for the cricket prediction system: architecture, configuration, APIs, ML training, and operations.

**Start here:** [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md) at the repo root — data flow, ML model shapes, and aggregation in one place. Its model shapes and endpoint tables are **generated** (`make gen-architecture-map`), so they cannot drift from the code.

Everything in this directory describes the system as it is on `main`. The plans, checklists and
audits that described the pipeline deleted during the P-0…P-7 re-architecture were removed once
that migration completed; git history is the archive.

---

## Reference

How the system works now. These should always be true of `main`.

| Doc | Purpose |
|-----|--------|
| [overview.md](overview.md) | Components, data flow, the three pipeline steps, how to run them |
| [config-and-data.md](config-and-data.md) | go-app and ml-service config keys, migrations, Cricsheet import, dataset acquisition |
| [apis-backtest-and-ops.md](apis-backtest-and-ops.md) | Go and ML API contracts, the prediction and evaluation surfaces, the ops status dashboard |
| [ml-and-training.md](ml-and-training.md) | The XI layer: the rating pass, the win models, the performance model, the simulator, and the L4 harness |
| [observability.md](observability.md) | Where to look for health and behaviour: endpoints, run identity, progress, logs, frontend tabs |
| [quality-and-debugging.md](quality-and-debugging.md) | Test standards, coverage gates, dead-code guardrails, container OOM and blank-page triage |
| [SUPPRESSIONS.md](SUPPRESSIONS.md) | Every lint/scanner suppression in the repo, with its justification |

## The measurement record

| Doc | Purpose |
|-----|--------|
| [ML_PIPELINE_REARCHITECTURE_PLAN.md](ML_PIPELINE_REARCHITECTURE_PLAN.md) | **The evidence for every number the system claims.** Model-health verdict, target architecture, keep/remove table, experiments E1–E7, the P-0…P-7 migration (complete, 2026-09-02) and its acceptance numbers, the schema and step changes, and the leakage / ML-standards audit. Every figure in it is one `make evaluate` reproduces; the migration footer in §6 is the short statement of what the system now is |

It is a record, not a to-do list: the plan has no further PRs.

## The open plan

| Doc | Purpose |
|-----|--------|
| [FOLLOW_UP_PLAN.md](FOLLOW_UP_PLAN.md) | **The to-do list.** Post-migration follow-ups: two ops defects (D-8 orphaned walk-forward upload, D-9 the console retrain cutoff mismatch) with the H-24 boundary-contract rule, the metric-glossary/legibility PR, and the accuracy roadmap (venue & competition context, chase tails, the T20 lineup signal, locked-window rotation, data cadence) |

---

## Component docs

| Doc | Purpose |
|-----|--------|
| [../README.md](../README.md) | Repo root: quick start, workflows, troubleshooting |
| [../go-app/README.md](../go-app/README.md) | Go services, make targets, mocks, coverage |
| [../go-app/docs/testing-guidelines.md](../go-app/docs/testing-guidelines.md) | The Go unit-test conventions, in full |
| [../ml-service/README.md](../ml-service/README.md) | FastAPI service, its endpoints, artifact loading |
| [../scripts/experiments/xi/README.md](../scripts/experiments/xi/README.md) | The standalone experiment scripts the plan's numbers come from |

## Cross-references

- **Feature contract:** `ml-service/ml/xi/contract.py` — the XI columns, the display columns and the target. The XI layer computes its own as-of features from the event store; there is no shared feature-vector file and no export contract (P-6). Described in [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).
- **Evaluation report:** endpoints and flow in [apis-backtest-and-ops.md](apis-backtest-and-ops.md); what it measures in [ml-and-training.md](ml-and-training.md) § Evaluation harness.
- **Team selection and the match simulator:** [ml-and-training.md](ml-and-training.md) and [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md).

## Generated files — do not hand-edit

| File | Regenerate with | Guarded by |
|------|-----------------|-----------|
| The marked blocks of [ARCHITECTURE_MAP.md](../ARCHITECTURE_MAP.md) | `make gen-architecture-map` | `make gen-architecture-map-check` |

It runs in the `Docs consistency` workflow on every PR that touches its inputs.
