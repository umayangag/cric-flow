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
- **If count > 0:** There are existing comments. **Resolve them first:** implement the suggested fixes (per-thread path/line/body or ```suggestion```), run **run-check-all-incremental** only if there are uncommitted changes (fix and re-run failed part until all pass), resolve the fixed threads via GraphQL, commit and push (e.g. `git add . && git commit -m "Fix Gemini comments" && git push`). Then **start the main cycle** from step A.
- **If count = 0:** No existing comments. **Start the main cycle** from step A.

---

## Main cycle (repeat until exit condition)

- Comment on the PR: `/gemini review` (e.g. `gh pr comment <PR> --body "/gemini review"`).

### B. Wait for Gemini (learn optimum time)

- **Stored value:** Read `.cursor/skills/publish-feature/optimum_wait_minutes.txt`; if missing or not a positive integer, use **5**.
- **If the user says Gemini has already finished:** skip the wait and go to step C; do not update the stored value.
- **Otherwise:**  
  1. Wait **optimum_wait_minutes** (from file or 5).  
  2. **Poll:** Fetch unresolved Gemini thread count for the PR (same GraphQL as in fix-gemini-reviews; count the nodes).  
  3. If count **> 0:** proceed to step C and **learn:** elapsed = minutes from step A (comment) to this poll. Write `ceil(elapsed)` to `optimum_wait_minutes.txt` so future runs use this wait. If elapsed &lt; stored value, this shortens the default; if elapsed &gt; stored, this avoids under-waiting next time.  
  4. If count **= 0:** wait **90 seconds**, poll again. Repeat until count &gt; 0 or **total wait ≥ 10 minutes**, then proceed to step C. When count first became &gt; 0, use that poll’s elapsed time to update the file as above.

### C. Fetch threads; fix only if needed

- **Fetch** unresolved review threads by `gemini-code-assist` for the PR (same GraphQL/pagination as in **fix-gemini-reviews**).
- **If 0 threads:** exit the loop (no fix, no check, no push). Summarize and stop.
- **If 1+ threads:** continue:
  - Implement the suggested fixes (per-thread path/line/body or ```suggestion```).
  - **Before pushing:** run **run-check-all-incremental** only if there are uncommitted changes; fix failures and re-run only the failed part until all pass.
  - Resolve the fixed threads via GraphQL, then commit and push (e.g. `git add -u && git commit -m "Fix Gemini comments" && git push`).

### D. Loop or exit

- **Exit** if cycle = 10 or (after step C) unresolved Gemini thread count = 0.
- Otherwise **go back to step A**.

---

## Optimum wait (learning)

- **File:** `.cursor/skills/publish-feature/optimum_wait_minutes.txt` — single integer (minutes). Read at start of step B; write when we first see unresolved Gemini threads and know elapsed time.
- **Update rule:** When proceeding to step C because thread count &gt; 0, set file content to `ceil(elapsed_minutes_since_comment)` so the next run uses that as the initial wait. Cap stored value at 10. If we hit 10 min with count still 0, set file to `min(10, stored+1)` so the next run tries slightly longer before first poll.

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

**Main cycle:**  
| Step | Action |
|------|--------|
| A | Comment `/gemini review` on PR |
| B | Wait optimum_wait_minutes (from file, default 5), then poll every 90s for thread count; proceed when &gt;0 or 10 min; update file with elapsed when threads appear |
| C | Fetch unresolved Gemini threads; if 0 → exit; else fix, run-check-all-incremental (if changes), resolve, push |
| D | If cycle &lt; 10 and threads &gt; 0 → go to A; else exit |

Exit when: **cycle = 10** or **unresolved Gemini threads = 0**.
