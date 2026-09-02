# Lint and type suppressions

This project treats warnings as failures (see [CLAUDE.md](../CLAUDE.md) § Quality bars). Suppressions are allowed only when **unavoidable** and must be **documented**.

## Categories

- **Go (`//nolint`):** Used in tests for unparam (test helpers with many params), staticcheck (intentional nil context in tests), gosec (fixed-size array indexed by range), unused (kept for API consistency). Do not add without a short comment explaining why.
- **Python (`# noqa`, `# type: ignore`):** Used for optional/conditional imports, broad except where required at import, sklearn/joblib type ignores in baselines, and in tests for intentional invalid-arg tests. Do not add without justification.
- **Frontend (`eslint-disable`):** Used sparingly (e.g. react-hooks/exhaustive-deps only when the dependency is intentionally omitted and documented). Prefer fixing the underlying issue.

## Audit

When adding a new suppression, add a one-line comment at the suppression site saying why it is unavoidable, and note it here. To remove suppressions: fix the underlying issue (types, test structure, or logic) then delete the suppression.
