# Lint and type suppressions

This project treats warnings as failures (see [.cursor/rules/coding-principles.mdc](../.cursor/rules/coding-principles.mdc) and [run-check-all-incremental](../.cursor/skills/run-check-all-incremental/SKILL.md)). Suppressions are allowed only when **unavoidable** and must be **documented**.

## Categories

- **Go (`//nolint`):** Used in tests for unparam (test helpers with many params), staticcheck (intentional nil context in tests), gosec (fixed-size array indexed by range), unused (kept for API consistency). Do not add without a short comment explaining why.
- **Python (`# noqa`, `# type: ignore`):** Used for optional/conditional imports, broad except where required at import, sklearn/joblib type ignores in baselines, and in tests for intentional invalid-arg tests. Do not add without justification.
- **Frontend (`eslint-disable`):** Used sparingly (e.g. react-hooks/exhaustive-deps only when the dependency is intentionally omitted and documented). Prefer fixing the underlying issue.

## Audit

When adding a new suppression, add a one-line comment at the suppression site and consider noting it here or in [audit-coding-principles-remediation.md](audit-coding-principles-remediation.md). To remove suppressions: fix the underlying issue (types, test structure, or logic) then delete the suppression.
