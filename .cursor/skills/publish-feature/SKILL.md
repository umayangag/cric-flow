---
name: publish-feature
description: Runs a publish loop: ensure all checks pass, trigger Gemini PR review, wait, fix Gemini comments and re-run checks before pushing, then repeat. Use when the user says "/publish-feature" to iterate on a PR until review is clean or max cycles reached.
---

# Publish feature (review loop)

When the user says **/publish-feature**, run the following workflow. First resolve any **existing** unresolved Gemini comments on the PR (if present), then run the main cycle. Stop after **10 cycles** or when there are **no unresolved Gemini review threads**.

---

## Prerequisites

- GitHub CLI (`gh`) authenticated with `repo` scope
- Current branch has commits to publish (PR may or may not exist yet)

---

## Initial setup (before the cycle)

### 1. Run checks

Apply **run-check-all-incremental** (frontend → go-app → ml-service → context-provider) until all pass. Do not proceed until all pass. In the main cycle, run it again only **before push and only if there are uncommitted changes** when fixing threads.

### 2. Ensure PR exists (do not comment yet)

- **If a PR exists for the current branch:** note the PR number (e.g. `gh pr view --json number -q .number` or `gh pr list --head $(git branch --show-current) -q .number`).
- **If no PR exists:** create one (`gh pr create --fill` or equivalent), then note the PR number.

### 3. Check for existing unresolved Gemini threads

- **Fetch** unresolved review threads by `gemini-code-assist` for the PR (same GraphQL/pagination as in **fix-gemini-reviews**).
- **If count > 0:** There are existing comments. **Resolve them first:** implement the suggested fixes (per-thread path/line/body or ```suggestion```), run **run-check-all-incremental** only if there are uncommitted changes (fix and re-run failed part until all pass), resolve the fixed threads via GraphQL, commit and push (e.g. `git add -u && git commit -m "Fix Gemini comments" && git push`). Then **start the main cycle**: do step A (post `/gemini review` on the PR), then B, C, D.
- **If count = 0:** No existing comments. **Start the main cycle**: do step A (post `/gemini review` on the PR), then B, C, D. Do not skip step A — without it there will be no suggestions.

---

## Main cycle (repeat until exit condition)

**Mandatory:** Every cycle iteration **must** begin with step A. Do **not** run step B (wait) or step C (fetch) until you have posted `/gemini review` on the PR in this iteration. Without this comment, Gemini does not run and there will be no review suggestions.

### A. Request Gemini review

- **Post** a comment on the PR with body `/gemini review` (e.g. `gh pr comment <PR> --body "/gemini review"`).
- Do this **first**, before waiting or fetching. Then proceed to step B.

### B. Wait for Gemini (size-based + learning)

**Wait up to a total of 15 minutes** (from the time of step A) before ending the wait and proceeding to step C. Do not proceed to step C before the initial wait; do not keep waiting past 15 minutes total. If Gemini does not reply after the initial wait, keep polling every 90 seconds until total elapsed time reaches 15 minutes.

- **Initial wait (PR size–based):** Get PR stats: `gh pr view <PR> --json changedFiles,additions,deletions`. Compute `totalChanges = additions + deletions`.  
  - **Small:** `changedFiles ≤ 5` and `totalChanges < 250` → wait **2** minutes.  
  - **Medium:** `changedFiles ≤ 15` or `totalChanges < 1000` → wait **5** minutes.  
  - **Large:** else → wait **7** minutes.  
  If `.cursor/skills/publish-feature/optimum_wait_minutes.txt` exists and contains a positive integer **smaller** than this size-based value, use that instead (learned shorter wait).
- **If the user says Gemini has already finished:** skip the wait and go to step C; do not update the stored value.
- **Otherwise:**  
  1. Wait the chosen **initial_wait** minutes.  
  2. **Poll:** Fetch unresolved Gemini thread count for the PR (same GraphQL as in fix-gemini-reviews; count the nodes).  
  3. If count **> 0:** proceed to step C and **learn:** elapsed = minutes from step A (comment) to this poll. Write `ceil(elapsed)` to `optimum_wait_minutes.txt`, capped at **15**. Future runs use this if it is smaller than the size-based wait.  
  4. If count **= 0:** wait **90 seconds**, poll again. **Repeat until count &gt; 0 or total elapsed time since step A reaches 15 minutes**, then proceed to step C. Once **total wait is 15 minutes** with still 0 threads, stop waiting and proceed to step C (with 0 threads, then exit). When count first became &gt; 0, use that poll’s elapsed time to update the file (capped at 15). If we hit 15 min with count still 0, set file to `min(15, stored+1)`.

### C. Fetch threads; fix only if needed

- **Fetch** unresolved review threads by `gemini-code-assist` for the PR (same GraphQL/pagination as in **fix-gemini-reviews**).
- **If 0 threads:** exit the loop (no fix, no check, no push). Summarize and stop.
- **If 1+ threads:** continue:
  - Implement the suggested fixes (per-thread path/line/body or ```suggestion```).
  - **Before pushing:** run **run-check-all-incremental** only if there are uncommitted changes; fix failures and re-run only the failed part until all pass.
  - Resolve the fixed threads via GraphQL, then commit and push (e.g. `git add -u && git commit -m "Fix Gemini comments" && git push`).
  - **After every push:** post `/gemini review` on the PR (e.g. `gh pr comment <PR> --body "/gemini review"`) so Gemini runs again on the new commits.

### D. Loop or exit

- **Exit** if cycle = 10 or (after step C) unresolved Gemini thread count = 0.
- Otherwise **go back to step A** (post `/gemini review` again, then B, C, D).

---

## Optimum wait (PR size + learning)

- **Size-based default:** Small PR (≤5 files, &lt;250 line changes) → 2 min; medium (≤15 files or &lt;1000 lines) → 5 min; large → 7 min.
- **File:** `.cursor/skills/publish-feature/optimum_wait_minutes.txt` — single integer (minutes). If present and **smaller** than the size-based wait, use it as the initial wait (learned that less time was enough). Write when we first see unresolved Gemini threads: `ceil(elapsed_minutes_since_comment)`, cap at 15. If we hit 15 min with count still 0, set file to `min(15, stored+1)`.
- **Max wait:** Wait up to **15 minutes total** (from step A). If Gemini has not replied after the initial wait, keep polling every 90 seconds until total elapsed time since step A is 15 minutes; then proceed to step C and stop (no threads → exit).

---

## Cycle counter

Track the current cycle (1–10). At the start of each iteration, state the cycle number.

---

## Summary

**Initial setup:**  
| Step | Action |
|------|--------|
| 1 | run-check-all-incremental until all pass (once; again only before push when there are local changes) |
| 2 | Create PR if needed; note PR number (do not comment yet) |
| 3 | Fetch unresolved Gemini threads; if &gt; 0 → fix, run-check-all-incremental (if changes), resolve, push; then start cycle. If 0 → start cycle. |

**Main cycle (do A then B then C then D; never skip A):**  
| Step | Action |
|------|--------|
| A | **Post** `/gemini review` on PR (required first; no suggestions without it) |
| B | Wait = size-based (small: 2 min, medium: 5 min, large: 7 min); if file has smaller value use it. Then poll every 90s; **wait up to 15 min total** from step A; proceed when count &gt;0 or total wait = 15 min; update file with elapsed when threads appear (cap 15). If no response after 15 min total, proceed to C and exit. |
| C | Fetch unresolved Gemini threads; if 0 → exit; else fix, run-check-all-incremental (if changes), resolve, push, then post `/gemini review` on the PR |
| D | If cycle &lt; 10 and threads &gt; 0 → go to A; else exit |

Exit when: **cycle = 10** or **unresolved Gemini threads = 0**.
