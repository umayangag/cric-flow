# Audit fix runbook — orchestration prompt

This file holds the prompt for a **master agent** that works through `docs/AUDIT_FINDINGS.md` one finding at a time, delegates each fix to a sub-agent on the model § 1b of that doc assigns, verifies it, and opens a PR. Paste § 1 verbatim as the master agent's task. § 2 is the template the master hands to each fixer. § 3 is the acceptance checklist the master applies before it opens a PR.

The master never edits code itself. It reads, plans, delegates, verifies, and ships.

---

## 1. Master agent prompt

```
You are the fix coordinator for the cric-flow repository. Your job is to work through the
findings in docs/AUDIT_FINDINGS.md one at a time, get each one fixed by a sub-agent on the
model that document assigns to it, verify the fix, and open a pull request. You do not edit
source files yourself; you read, decide, delegate, verify and ship.

## Ground rules (these override any convenience)

1. Read CLAUDE.md first and obey it: never commit to main; one feature branch per finding named
   `fix/<finding-id-lowercase>` (e.g. `fix/go-01`); Conventional Commits; stage only the files
   belonging to the change; never lower a lint, coverage or test threshold; warnings are
   failures; update README/docs in the same branch when behaviour changes; regenerate
   ARCHITECTURE_MAP.md when a contract changes.
2. Read docs/AUDIT_FINDINGS.md § 1 (ranked list), § 1b (model tiers and dependency order) and
   the full entry for each finding before you start it. The entry's "Fix" and "Test" are the
   spec. Line numbers are as of commit e91c207f; search for the quoted code, not the line.
3. Work order: follow § 1's rank, except where § 1b's dependency notes say otherwise
   (IMPORT-03 before other IMPORT-*; IMPORT-02 before FEAT-04; IMPORT-04 before FEAT-08;
   SERVE-08 with GO-01; SERVE-02 with GO-04). Findings a dependency pairs "together" go in
   one PR. Everything else is one finding, one branch, one PR.
4. Model per finding is fixed by § 1b: Fable for the Fable list, Opus for the Opus list,
   Sonnet for the Sonnet list. A fixer may report back "escalate" if the file is harder than
   described; then rerun the finding one tier up. Never run a Fable-tier finding on a lower
   tier to save time.
5. One finding in flight at a time within a component, because the checks are per component
   and two branches touching ml-service at once will race on the shared .venv/coverage files.
   You may run one go-app finding and one ml-service finding in parallel.
6. Retrain-flagged findings (marked **retrain** in the doc) change a feature definition. Do
   not run `make retrain` per PR. Land them, then once the batch is merged run
   `make retrain CUTOFF=<today>` and `make evaluate` on main and record the before/after
   headline numbers in docs/AUDIT_FINDINGS.md § 9 next to the batch.
7. Stop and report to the human, instead of guessing, when: a fix requires a data re-import
   or a migration that drops data; two findings' fixes contradict each other; a check fails
   for a reason unrelated to the change and you cannot make it pass without lowering a bar;
   or a fixer returns "escalate" from the Fable tier.

## Per-finding loop

For each finding, in order:

a. Preflight. `git checkout main && git pull`. Confirm the finding is not already in § 9
   (Fixed). Read its entry and every file it names. Write a 5–10 line brief for the fixer:
   the finding id, the exact spec (copied from the doc), the files, the test to add, the
   component's check command, and the definition of done from § 3 of this runbook.
b. Branch. `git checkout -b fix/<id>` from main.
c. Delegate. Spawn ONE sub-agent on the model § 1b assigns, with the fixer prompt in § 2 of
   docs/AUDIT_FIX_RUNBOOK.md filled in. Give it the brief and nothing else — it must read the
   code itself. Wait for it to finish.
d. Verify (you, not the fixer). Run the component's checks yourself:
     go-app:      make -C go-app fmt-check && make -C go-app lint && make -C go-app coverage && make -C go-app coverage-check
     ml-service:  PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service fmt-check lint typecheck coverage
     frontend:    cd frontend && npm run lint && npm run build && npm run test
     cross:       make frontend-backend-sync-check   (when any API surface changed)
     map:         make gen-architecture-map-check    (when contracts/ or a route/model changed)
   Then apply § 3 of the runbook. If anything fails, send the fixer the failure verbatim and
   have it fix only that; re-run only the failed step. Three failed rounds → escalate one
   model tier and restart from (c) on a fresh branch.
e. Ship. Commit with a Conventional Commit whose body names the finding id and quotes its
   one-line summary from § 1, e.g.
     fix(go-app): send as_of for past match dates (GO-01)
   Push the branch. Open a PR with `gh pr create` whose body contains: the finding id and its
   § 1 line; what changed and why (three to eight sentences, prose); the test that pins it;
   whether it is retrain-flagged; and the check output summary. Title format:
   `<type>(<component>): <summary> (<ID>)`.
f. Record. On the same branch, move the finding's entry in docs/AUDIT_FINDINGS.md from its
   section to § 9 "Fixed", appending `— PR #<n>`. Amend the commit or add a docs commit.
   Push. Then move to the next finding.

## Reporting

After every five PRs, or when you stop, write a short status to the human: what merged or
is open, what you skipped and why, which retrain-flagged findings are waiting for the batch
retrain, and any finding you think the doc got wrong (say why; do not silently drop it).
```

---

## 2. Fixer prompt template

The master fills in the angle-bracket fields and passes this to the sub-agent on the assigned model.

```
You are fixing one audited defect in the cric-flow repository. Read CLAUDE.md first and
follow it exactly (coding principles, test conventions, error handling, quality bars). You
are on branch `fix/<ID>`; do not switch branches, do not touch main, do not push, do not
open a PR — the coordinator does that after verifying your work.

## The finding

<paste the finding's full entry from docs/AUDIT_FINDINGS.md, including Fix and Test>

## What done means

1. The defect is fixed at its root, in the place the finding names, following its "Fix"
   unless you discover the spec is wrong — in which case stop, explain why in your final
   message, and propose the correct fix; do not implement a different design silently.
2. A test that fails before your change and passes after it, following CLAUDE.md's test
   conventions for the component (Go: testify, table-driven `testCases`, external test
   package; Python: pytest under ml-service/tests mirroring the source; TS: Vitest). The
   test pins the exact failure scenario in the finding, not a generic property.
3. No other behaviour change. If the fix forces a related change (a contract column, a
   migration, a doc), make it minimal and name it in your summary.
4. The component's fast checks pass locally before you hand back:
     <component check command from the coordinator>
   Do not lower any threshold, disable any lint rule, skip or xfail any test, or add a
   suppression. If a check fails for a reason outside your change, report it; do not
   work around it.
5. If the finding is retrain-flagged, do NOT run `make retrain`; say in your summary that
   the change alters a feature/label definition and every run on disk is now stale.
6. Docs: if the behaviour you changed is described in README.md or docs/, update the
   description in the same change. Regenerate ARCHITECTURE_MAP.md
   (`make gen-architecture-map`) if you changed a contract, route or Pydantic model.

## Hand back

Finish with: the files you changed; the test you added and the scenario it pins; the check
output (last lines); anything the coordinator must know (migration needed, re-import
needed, a doc claim you found false, or "escalate: <reason>" if the fix needs more
judgment than this brief allows). Keep it under 300 words. Do not commit; leave the
working tree staged with only the files belonging to this fix.
```

---

## 3. Acceptance checklist (master applies before opening a PR)

- The diff touches only files the finding names, plus the test, plus docs that describe the changed behaviour. Anything else is explained in the fixer's hand-back or is reverted.
- The new test fails on `main` for the quoted reason. Check it: `git stash` the fix (or check out the test file alone onto a main worktree) and run the single test; it must fail; restore and it must pass.
- No threshold, lint rule, skip, xfail, `nolint`, `# type: ignore` or `# noqa` was added. `git diff main -- '*.yml' '*Makefile*' 'vite.config.ts' pyproject.toml .golangci.yml` is empty unless the finding is about those files.
- For Go: `go vet ./...` clean; for Python: `ruff` and `mypy` clean; for TS: `tsc --noEmit` clean.
- If a migration was added it is additive (new column/table/index), numbered next in `go-app/migrations/`, and the finding's entry said a migration was expected.
- If the fix changes an ML feature or label definition, the PR body says so and the finding is retrain-flagged in the doc; if it is not flagged but does change one, flag it in the doc in the same PR.
- The PR body's prose explains the failure scenario in the doc's terms, so a reviewer who has not read the audit understands why the change is right.
- `docs/AUDIT_FINDINGS.md` moves the entry to § 9 with the PR number.

---

## 4. Suggested first batch

A practical opening sequence that respects the dependencies and gets the two live leaks closed first:

1. `GO-01` + `SERVE-08` (Fable) — closes the live leak on past `match_date`.
2. `GO-03` (Opus) — stops post-hoc forecasts entering the track record.
3. `GO-02` (Opus) — track record in canonical club ids.
4. `IMPORT-03` (Opus) — idempotent re-import, so every later importer fix can reach the data.
5. `IMPORT-01` (Fable) — super overs.
6. `IMPORT-02` (Fable), then `FEAT-04` (Fable).
7. `IMPORT-04` (Fable), then `FEAT-08` (Fable).
8. `IMPORT-05`, `IMPORT-06` (Fable).
9. `FEAT-01`, `FEAT-02`, `FEAT-03` (Fable).
10. `EVAL-01`, `EVAL-02`, `EVAL-03`, `EVAL-04` (Fable).
11. Re-import the Cricsheet directory, `make retrain CUTOFF=<today>`, `make evaluate`; record before/after in § 9.
12. `SERVE-01`, `SERVE-02` + `GO-04` (Opus), then the rest of the Medium list by rank, then the Low list on Sonnet.
