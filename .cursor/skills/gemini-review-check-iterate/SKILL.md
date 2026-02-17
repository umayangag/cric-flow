---
name: gemini-review-check-iterate
description: Runs Gemini code review (gemini "/code-review" --yolo), then run-check-all-incremental until all checks pass, re-runs Gemini review and iterates until no high or medium issues remain, then commits and pushes to the current branch. Use when the user wants a full review-and-fix cycle with Gemini and local checks before committing.
---

# Gemini review, check-all, iterate, then commit

## Goal

Run a full quality cycle: Gemini code review → fix and pass all local checks → re-review until no high/medium issues → commit and push.

## Workflow

1. **Run Gemini code review** (from repo root):
   ```bash
   gemini "/code-review" --yolo -e code-review > local-review.md
   ```
   If `gemini` is not on PATH or the command fails, report and skip to manual review.

2. **Apply run-check-all-incremental**: Run lint, format, and tests per component (frontend → go-app → ml-service → context-provider). Fix any failures and re-run only the failed step until all pass. Do not run full `make check-all` in a loop.

3. **Re-run Gemini review**: Run the same `gemini "/code-review" ...` command again. Read `local-review.md` and parse for **high** or **medium** severity issues (or equivalent wording in the review).

4. **Iterate**: If the review still reports high or medium issues:
   - Address each issue in code (or document why deferred).
   - Re-run only the failed checks from run-check-all-incremental if needed.
   - Run Gemini review again and repeat until no high/medium issues remain.

5. **Commit and push**: When checks pass and the review has no high/medium issues:
   - Stage changes: `git add -A` (or appropriate paths).
   - Commit with a clear message describing the changes.
   - Push to the current branch: `git push origin $(git branch --show-current)`.

## Parsing the review

- Treat findings labeled **high**, **critical**, or **must fix** as blocking; address before commit.
- Treat **medium** or **should fix** as blocking for this workflow; address or explicitly document as accepted risk.
- **Low** / **nice to have** may be noted but do not block commit.

## Efficiency

- After fixing review feedback, re-run only affected component checks (e.g. go-app only) when possible.
- Use run-check-all-incremental’s per-step commands to re-run only failed steps.
