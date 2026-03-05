# Stage 3 – Joint / hierarchical modelling (design stub §4)

**Goal:** Move beyond independent models + reconciliation to **hierarchical models** where batting, bowling, and win predictions share latent structure (e.g. batting strength, bowling strength, pitch).

This document is a design placeholder. Implementation is optional and follows Stage 2 (consistency-aware training) and the current reconciliation pipeline.

## 4.1 Define latent structure

- **Candidate latent variables**
  - Team batting strength (per team, possibly per format/venue).
  - Team bowling strength (per team).
  - Venue / pitch factor (affects run rates, wicket propensity).
  - Match-level noise (e.g. toss, conditions).
- **Mapping from current features**
  - Map existing features (consistency, form, venue_id, opposition, etc.) to these latent factors where possible; identify which factors are shared across batting, bowling, and innings models.
- **Output:** A small design doc or schema that defines the latent variables and how they feed into batting, bowling, innings, and win outputs.

## 4.2 Prototype generative model

- Start with a **small scope** (e.g. T20 only).
- **Generative process (high level):**
  - Sample or condition on latent variables (batting strength, bowling strength, pitch).
  - Generate ball-by-ball or over-by-over outcomes (or aggregate to player/innings level).
  - Derive per-player lines and team totals from the same generative process so that they are **by construction** consistent (no separate reconciliation step).
- **Evaluation:** Compare realism (e.g. distribution of runs per innings, wickets) and predictive accuracy vs the current decoupled pipeline + reconciliation.

## 4.3 Evaluate vs decoupled + reconciliation

- **Offline comparison**
  - Predictive accuracy (e.g. MAE/RMSE on held-out matches).
  - Realism metrics (reconciled vs historical bands; see `ml.harmony_metrics`).
  - “Reconciliation effort”: in the current pipeline, how much adjustment is applied; in the joint model, this is zero by design. Compare also win coherence.
- **Decision:** Adopt joint modelling only if it matches or improves on decoupled + reconciliation on these axes without excessive complexity or training cost.

## Status

- No implementation yet. This doc serves as a backlog for when the team is ready to explore joint/hierarchical models.
