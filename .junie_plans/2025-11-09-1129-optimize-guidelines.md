# Plan X: Optimize and Reorganize Junie Guidelines

Parent: —
Active path: X

## Goal
Rearrange and optimize `.junie/guidelines.md` for this project. Remove redundancies and non‑applicable instructions, standardize paths/wording, and keep the Plan Hierarchy & Anti‑Drift Rule prominent. No behavior change to the codebase beyond the guideline document.

## Scope
- File: `.junie/guidelines.md`
- Create plan file under `.junie_plans/` (this file)

## High‑Level Changes
1. Standardize plan storage path references to `.junie_plans/{timestamp}-{slug}.md` (single canonical form).
2. Keep SOP sections but streamline content and fix typos; keep Anti‑Drift Rule (X / X.y / X.y.z) prominent under Step 2.
3. Remove redundant/duplicated sections:
   - Remove the separate "Environment Constraint" Terraform note (already covered under Defaults previously; consolidated now removed).
   - Remove duplicated `src/` Directory Rule from Defaults (retain a single reference under Project‑Specific Guidelines).
4. Fix testing standards wording for Go and Python (grammar, table‑driven phrasing, assert wording, file naming).
5. Minor grammar/punctuation cleanups in Project‑Specific Guidelines.

## Files to Modify
- `.junie/guidelines.md` (reorder/cleanup, standardize path, remove redundancy, fix wording)

## Tests
- Not applicable (documentation only). Verification is done via text searches.

## Acceptance Criteria
- A1: The document references only `.junie_plans/{timestamp}-{slug}.md` for plan files.
- A2: There is no stand‑alone "Environment Constraint" section; Terraform/IaC guidance not duplicated elsewhere unless explicitly needed.
- A3: There is only one `src/` rule location under Project‑Specific Guidelines; no duplicate in Defaults.
- A4: Testing standards read:
  - Go: "tests should not have if statements; use assert functions instead" and "tests should be written in table‑driven format and use the {packagename}_test.go naming."
  - Python: "Tests should not have if statements; use assert functions instead." and "Tests should be written in table‑driven format."
- A5: The Anti‑Drift Plan Hierarchy section remains intact under Step 2 and unchanged in spirit.
- A6: Grammar/typos fixed in the closing Project‑Specific bullet: "Ensure all code is tested and documented, e.g., Makefile and README.md".

## Verification Commands
Run from repo root:

```bash
# A1: Only the canonical plan path remains
rg -n "\.junie_plans/" .junie/guidelines.md | wc -l
rg -n "\.junie/\{title\}\{timestamp\}" .junie/guidelines.md || true
rg -n "plan-\{date:timestamp\}-\{title\}" .junie/guidelines.md || true

# A2: No Environment Constraint section; no Terraform duplication
rg -n "Environment Constraint" .junie/guidelines.md || true
rg -n "Terraform|IaC" .junie/guidelines.md

# A3: No duplicate src rule in Defaults
rg -n "`src/` Directory Rule" .junie/guidelines.md || true
rg -n "`src/` directory contains the prototype" .junie/guidelines.md

# A4: Testing phrasing
rg -n "tests should not have if statements; use assert functions instead" .junie/guidelines.md
rg -n "table-driven" .junie/guidelines.md
rg -n "{packagename}_test.go" .junie/guidelines.md

# A5: Anti‑Drift section exists under Step 2
rg -n "Plan Hierarchy & Anti‑Drift Rule" .junie/guidelines.md

# A6: Grammar fix present
rg -n "Ensure all code is tested and documented, e.g., Makefile and README.md" .junie/guidelines.md
```

## Execution Notes
- Keep the doc concise and tailored for this repo.
- Do not change anything under `src/`.

## Status Tracking
- X.1 Review and normalize path references — Done ✓
- X.2 Remove redundant sections (Env Constraint, duplicate src rule) — Done ✓
- X.3 Fix testing standards wording — Done ✓
- X.4 Create this plan with acceptance criteria and verification commands — Done ✓
- X.5 Final verify and submit — Pending *

## Risks
- Over‑removal of guidance; mitigated by keeping concise Project‑Specific section and Defaults.

## Rollback
- Revert `.junie/guidelines.md` to previous commit if any acceptance criteria regress.
