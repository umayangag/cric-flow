# Audit: Object-oriented design (OOP) alignment

**Date:** 2025-03-11  
**Scope:** Alignment with the OOP section in [.cursor/rules/coding-principles.mdc](../.cursor/rules/coding-principles.mdc) (encapsulation, interfaces for dependencies, composition over inheritance, SRP for types, language-appropriate OOP).

---

## 1. Executive summary

- **Overall:** The codebase is largely aligned with the OOP guidelines. Go uses small interfaces for dependencies and composition; Python uses dataclasses and composition with limited inheritance; the frontend uses TypeScript interfaces and compositional React patterns.
- **Strengths:** Go-app consistently depends on interfaces (Logger, Client, MLClient, Predictor, DB, Tx, BattingExporter, etc.) for testability and swapping. Python uses dataclasses for data (PoolPlayer, ModelSpec, PipelineConfig, GenerateMatchSettings) and module-level functions or thin classes. Frontend uses interfaces for props and API shapes.
- **Gaps:** Some modules expose mutable state or broad structs; a few Python modules use classes where a Protocol or explicit interface would make contracts clearer; frontend has a few components that could better separate data-fetching from presentation.

---

## 2. Go-app (Go)

### 2.1 Interfaces for dependencies

| Area | Interface | Purpose |
|------|-----------|---------|
| logger | `Logger` | Test-friendly logging; mockery mocks in tests |
| server | `Client` | ML prediction client for API handlers |
| teampredictor | `MLClient` | PredictTeam, Reload; local to service for mocking |
| predictor | `Predictor` | PredictWin for commands |
| db | `DB`, `Tx`, `PoolIface`, `Rows`, `Row` | DB abstraction for pgxmock and tests |
| seqcalc | `Calculator` | Feature calculators (Name, Compute); multiple implementations |
| exportdataset | `BattingExporter`, `BowlingExporter`, etc. | Per-format export; mocks in tests |
| cricsheet | Seams (CricsheetDB, WeatherClient, etc.) | Test seams for ingest |
| teamselect | `Selector` | Selection algorithm pluggable |
| repo_features_cutoff | `FeatureProvider` | Feature data abstraction |

**Finding:** Strong use of small, focused interfaces. “Accept interfaces, return structs” is followed in most services.

### 2.2 Encapsulation

- **Good:** Response types (e.g. `playerResponse`, `matchDetailsResponse`) are package-private where appropriate. Repos expose methods, not raw DB handles.
- **Gap:** Some server packages use package-level seam variables (e.g. `getBacktestMatchDateFunc`, `mlBacktestPredictFunc`) that are set in tests. This is a form of dependency injection but is global; consider constructor or options struct injection where it improves clarity.

### 2.3 Composition vs inheritance

- **Good:** Go avoids inheritance; composition is used (e.g. App embeds or uses handlers, services use repos and clients). No deep type hierarchies.

### 2.4 SRP for types

- **Good:** Interfaces are small (one to a few methods). Structs are used for data (params, config, response DTOs). Calculator, Exporter, and Client interfaces each have a clear role.

---

## 3. ML-service (Python)

### 3.1 Data and behavior

- **Dataclasses:** Used consistently for data: `GenerateMatchSettings`, `ModelSpec`, `PipelineConfig`, `PoolPlayer`, `SelectionConstraints`, `ScoreWeights`, `OptimizationResult`, `BattingLine`, `BowlingLine`, `Variable`, `LinearConstraint`, `ReconciledPlayerStats`, `InningsTargets`, etc. Clear separation of “data bundle” vs “behavior.”
- **Classes with behavior:** `TrainingPipeline`, `CricketGeneralizedPipeline`, `AutogluonPredictorWrapper`, `Tracker`, `ProblemBuilder` — these encapsulate state and algorithms. No deep inheritance trees; composition (e.g. pipeline uses config, model specs) is common.

### 3.2 Interfaces (Protocol / ABC)

- **Finding:** Python code rarely defines explicit `Protocol` or ABC for “pluggable” behavior; dependencies are usually concrete modules or callables. Tests use small dummy classes (e.g. `DummyBatModel`, `DummyScaler`) that match the expected shape. This is acceptable but adding a `Protocol` for key contracts (e.g. “model with predict”) would make the contract explicit and type-checkable.
- **Recommendation:** For new code, consider `typing.Protocol` for ML model or repository-style dependencies where you want a clear, testable contract.

### 3.3 Encapsulation

- **Good:** Module-level functions (e.g. `make_base_estimator`, `compute_time_decay_weights`) keep pure logic outside classes. Classes like `TrainingPipeline` and `CricketGeneralizedPipeline` take config and expose a small API (`run`, `fit`, etc.).
- **Gap:** Some app modules (e.g. `prediction_service`) are large and mix orchestration with feature-building; further splitting would improve encapsulation (see audit-coding-principles-remediation.md).

### 3.4 Composition over inheritance

- **Good:** No deep class hierarchies. Pipelines and services compose dataclasses and functions. Enums used for closed sets (`VariableKind`, `ConstraintKind`, `ExtrasKind`, `WicketKind`).

---

## 4. Frontend (TypeScript / React)

### 4.1 Interfaces

- **Good:** Props and context are typed with interfaces (`AuthContextType`, `OpsStatusDataGridProps`, API response types in `types.ts`). API client and hooks expose clear shapes.
- **Good:** Components receive data and callbacks via props rather than reaching into global state arbitrarily.

### 4.2 Composition

- **Good:** Functional components and hooks (e.g. `useEvaluateDb`, `useBacktestFormOptions`, `useApiCall`, `usePolling`) compose behavior. No class-component inheritance. Presentational components (e.g. `MLModelStatsSection`, `OpsStatusSection`) receive `data`, `loading`, `error`, `onRefresh`.

### 4.3 Encapsulation and separation

- **Good:** API layer is centralized (`api.ts`); hooks encapsulate loading/error state and refetch logic. Storage helpers (`evaluateDbStorage`) hide localStorage details.
- **Gap:** A few components still call `api.*` directly (e.g. `HealthTab`, `PipelineStepDialog`, `OpsSuggestions`, `OpsMigrationsTable`, `PipelineProgressPanel`). Moving these to hooks (as with `MLModelStatsTab` and `OpsStatusTab`) would align with “push logic into hooks” and improve testability.

---

## 5. Remediation summary

| Priority | Area | Action |
|----------|------|--------|
| Low | Go server | Consider constructor or options-struct injection for backtest seams instead of package-level func vars where it simplifies tests. |
| Low | Python | Introduce `Protocol` for key “model” or “repository” contracts where you want an explicit, type-checked interface. |
| Low | Frontend | Gradually move remaining direct `api.*` calls in components into hooks and pass data/callbacks as props. |

No blocking OOP violations were found. The codebase already follows small interfaces, composition, and encapsulation in line with the new guidelines. The table above lists optional improvements.

---

## 6. Quick reference

- **OOP guidelines:** [.cursor/rules/coding-principles.mdc](../.cursor/rules/coding-principles.mdc) § Object-oriented design (OOP).
- **General remediation:** [audit-coding-principles-remediation.md](audit-coding-principles-remediation.md).
