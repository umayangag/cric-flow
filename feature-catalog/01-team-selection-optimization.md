# Plan 01: Team Selection Optimization (Priority 1)

**Goal:** Select 11 players from a pool to **maximize overall team performance**, with ≥1 wicket keeper and ≥5 bowlers (including part-time).

---

## 1. Objective

Replace the current greedy selection (sort by weighted score, fill top 11, then swap to satisfy keeper/bowler constraints) with either:
- **Option A:** Constrained optimization (ILP or explicit search) that maximizes expected team outcome subject to constraints; or
- **Option B:** Keep greedy selection but ensure **learned weights** from the combination meta-model are used by default when available.

This plan implements **Option B** first (minimal code change, immediate gain) and adds **Option A** as an optional optimizer that can be enabled via config.

---

## 2. Current State

| Component | Location | Behavior |
|-----------|----------|----------|
| Selection | `go-app/internal/services/teamselect/select.go` | `Select(pool, weights, constraints)`: sort by `ScorePlayer(., w)` desc, greedy fill, then swap for keeper then bowlers. |
| Scoring | `go-app/internal/services/teamselect/score.go` | `ScorePlayer(p, w)`: `w.Bat*bat + w.Bowl*bowl + w.Field*field + keeperBonus`; `DefaultWeights()` = 0.45, 0.40, 0.10, 0.02. |
| Weights source | `go-app/internal/config/config.go` | `EffectiveScoreWeightsForFormat(cfg, format)`: 1) meta-model from `selection.meta_model_path`; 2) `score_weights_by_format`; 3) `score_weights`; 4) defaults. |
| Predict team | `go-app/internal/services/predictteam/predict_team.go` | Calls `config.EffectiveScoreWeightsForFormat(cfg, format)` and `teamselect.Select(tsPool, weights, constraints)`. |

Meta-model is already supported in config; if `selection.meta_model_path` is set and the JSON is valid, learned weights are used. The gap: (1) meta_model_path is often unset; (2) no true optimization over the XI (greedy is suboptimal when bat/bowl/field contributions are not additive in the same way as the scalar score).

---

## 3. Acceptance Criteria

- [ ] When `selection.meta_model_path` points to a valid JSON file, team selection uses those weights (already true; verify in predict_team and any other callers).
- [ ] New config option e.g. `selection.use_optimizer: true` enables an **optimizer** that selects the XI by maximizing total score over all valid XIs (or by ILP), subject to size=11, ≥1 keeper, ≥5 bowlers.
- [ ] When `use_optimizer` is false or unset, behavior is unchanged (greedy + swap).
- [ ] Optimizer uses the same `ScorePlayer` and `ScoreWeights` (so meta-model weights apply to the objective).
- [ ] Unit tests: (1) Select still satisfies constraints; (2) OptimizerSelect returns same constraints; (3) OptimizerSelect total score >= greedy Select when pool has ≥11 players (optimizer cannot do worse).

---

## 4. Implementation Details

### 4.1 Config

**File:** `go-app/internal/config/config.go`

- Add to `Selection` struct:
  - `UseOptimizer bool \`json:"use_optimizer"\`` (default false).
- No change to `EffectiveScoreWeightsForFormat` (already supports meta-model).

### 4.2 Teamselect package

**File:** `go-app/internal/services/teamselect/optimize.go` (new)

- Add function:
  - `SelectOptimized(pool []Player, w ScoreWeights, c Constraints) ([]Player, error)`
  - Enumerate all valid XIs: size 11, at least 1 keeper, at least MinBowlers bowlers.
  - For each valid XI compute total score = sum over players in XI of `ScorePlayer(p, w)`.
  - Return the XI with maximum total score; tie-break by lexicographic order of player names.
  - If pool is small (e.g. len(pool) <= 15), full enumeration is cheap (C(n,11) for n≤15 is manageable). If pool is large, use a simple branch-and-bound or greedy-with-optimizer: e.g. keep greedy as baseline, then try swapping one player at a time and accept if total score improves (hill-climb). For n>20 document that we use "greedy + hill-climb swaps" to avoid C(n,11) blow-up.
- Implementation note: For n=15, C(15,11)=1365 combinations; for n=18, C(18,11)=31824. So up to n≈18 full enumeration is fine. For n>18, implement "greedy selection then hill-climb": start with greedy XI, then repeatedly try swap (one out, one in from rest) if it improves total score until no improving swap.

**File:** `go-app/internal/services/teamselect/select.go`

- Add a new exported function that respects config:
  - Either add `SelectWithMode(pool []Player, w ScoreWeights, c Constraints, useOptimizer bool) ([]Player, error)` that calls `Select` when useOptimizer is false and `SelectOptimized` when true, or keep Select as-is and have the caller (predict_team) call SelectOptimized when config says so. Prefer: **caller decides** (predict_team reads config and calls Select vs SelectOptimized). So no change to select.go; only add SelectOptimized in optimize.go.

### 4.3 Predict team

**File:** `go-app/internal/services/predictteam/predict_team.go`

- After building `tsPool1` and `tsPool2` and resolving `weights` and `constraints`:
  - If `cfg != nil && cfg.Selection.UseOptimizer`: call `teamselect.SelectOptimized(tsPool1, weights, constraints)` and `teamselect.SelectOptimized(tsPool2, weights, constraints)`.
  - Else: call `teamselect.Select(...)` as today.
- Ensure `EffectiveScoreWeightsForFormat` is still used for `weights` (already is).

### 4.4 Tests

**File:** `go-app/internal/services/teamselect/optimize_test.go` (new)

- TestSelectOptimized_Constraints: pool with exactly 1 keeper and 5 bowlers; check selected XI has 1 keeper and 5 bowlers.
- TestSelectOptimized_NoKeeperInPool: pool with 0 keepers, RequireKeeper true → error.
- TestSelectOptimized_NotEnoughBowlers: pool with 4 bowlers, MinBowlers 5 → error.
- TestSelectOptimized_ScoreNotWorseThanGreedy: for a fixed pool (e.g. 15 players) and weights, compare total score of SelectOptimized vs Select; optimized total >= greedy total.
- TestSelectOptimized_Deterministic: same pool and weights → same XI every time (e.g. tie-break by names).

**File:** `go-app/internal/services/teamselect/select_test.go`

- No change required; existing tests remain for Select.

### 4.5 Documentation

**File:** `docs/config-and-data.md` or `docs/ml-and-training.md`

- Document `selection.use_optimizer`: when true, team selection uses constrained optimization to maximize total score over valid XIs; when false, uses greedy + constraint swaps. Document that `selection.meta_model_path` (combination meta-model) provides the weights used in both modes.

---

## 5. File Checklist

| File | Action |
|------|--------|
| `go-app/internal/config/config.go` | Add `UseOptimizer bool` to Selection struct. |
| `go-app/internal/services/teamselect/optimize.go` | New: SelectOptimized, enumeration or greedy+hill-climb. |
| `go-app/internal/services/teamselect/optimize_test.go` | New: tests for SelectOptimized. |
| `go-app/internal/services/predictteam/predict_team.go` | Branch on config UseOptimizer to call SelectOptimized vs Select. |
| `docs/config-and-data.md` or `docs/ml-and-training.md` | Document use_optimizer and meta_model_path. |

---

## 6. Edge Cases

- Pool size < 11: return error (same as Select).
- Pool has no keeper but RequireKeeper true: return error.
- Pool has fewer than MinBowlers bowlers: return error.
- Ties: break by sorted names of the XI so result is deterministic.
- Very large pool (e.g. 30): use hill-climb from greedy to avoid C(30,11) enumeration.

---

## 7. Weather

No weather dependency in this plan.
