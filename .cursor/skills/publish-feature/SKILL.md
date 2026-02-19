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

**Thread fetch (same as fix-gemini-reviews):** Use the two-phase approach to keep responses small. **Phase 1** — paginate `reviewThreads` with minimal fields (`id`, `isResolved`, `comments(first:1){ nodes { author { login } } }` only); filter to `isResolved==false` and `author.login=="gemini-code-assist"`. For **count only** (e.g. polling), use Phase 1 and count nodes. For **full thread data** (to implement fixes), add **Phase 2** — for each thread ID, query `node(id)` for `comments(first:1){ nodes { path line body } }`. Resolve threads via the same GraphQL mutation as in fix-gemini-reviews.

**Rate limit (GraphQL / gh blocked):** If any `gh` call fails with a rate-limit message (e.g. `API rate limit already exceeded`, `rate limit exceeded`, or similar), do **not** retry in a loop. Instead: (1) Fetch rate limit expiry with `gh api /rate_limit` (REST; may still succeed when GraphQL is limited). (2) From the JSON, read `resources.graphql.reset` (Unix timestamp; GraphQL is what PR/thread calls use) or, if missing, `resources.core.reset`. (3) Report to the user: **"GitHub API rate limit hit. GraphQL (or core) resets at \<ISO or local time\>. Run /publish-feature again after that time."** Example to get human-readable time: `gh api /rate_limit -q '.resources.graphql.reset' | xargs -I{} date -r {} 2>/dev/null || date -d @{} 2>/dev/null` (or convert the Unix value in code). Then stop the workflow.

---

## Initial setup (before the cycle)

### 1. Run checks

Apply **run-check-all-incremental** (frontend → go-app → ml-service) until all pass. Do not proceed until all pass. In the main cycle, run it again only **before push and only if there are uncommitted changes** when fixing threads.

### 2. Ensure PR exists (do not comment yet)

- **If a PR exists for the current branch:** note the PR number (e.g. `gh pr view --json number -q .number` or `gh pr list --head $(git branch --show-current) -q .number`).
- **If no PR exists:** create one (`gh pr create --fill` or equivalent), then note the PR number.

### 3. Check for existing unresolved Gemini threads

- **Fetch** unresolved review threads by `gemini-code-assist` for the PR using the **same two-phase fetch as fix-gemini-reviews** (Phase 1: paginate with minimal fields — id, isResolved, author only — and collect IDs; Phase 2: for each ID, `node(id)` to get path/line/body). This keeps responses small.
- **If count > 0:** There are existing comments. **Resolve them first:** implement the suggested fixes (per-thread path/line/body or ```suggestion```), run **run-check-all-incremental** only if there are uncommitted changes (fix and re-run failed part until all pass), resolve the fixed threads via GraphQL, commit and push (e.g. `git add -u && git commit -m "Fix Gemini comments" && git push`). Then **start the main cycle**: do step A (post `/gemini review` on the PR), then B, C, D.
- **If count = 0:** No existing comments. **Start the main cycle**: do step A (post `/gemini review` on the PR), then B, C, D. Do not skip step A — without it there will be no suggestions.

---

## Main cycle (repeat until exit condition)

**Mandatory:** Every cycle iteration **must** begin with step A. Do **not** run step B (wait) or step C (fetch) until you have posted `/gemini review` on the PR in this iteration. Without this comment, Gemini does not run and there will be no review suggestions.

### A. Request Gemini review

- **Post** a comment on the PR with body `/gemini review` (e.g. `gh pr comment <PR> --body "/gemini review"`).
- Do this **first**, before waiting or fetching. Then proceed to step B.

### B. Wait for Gemini (size-based + learning)

**Poll every 90 seconds, up to 15 minutes since the last "/gemini review" comment** (i.e. since step A in this cycle), then stop and proceed to step C. Do not proceed to step C before the initial wait; do not keep waiting past 15 minutes from that comment. If Gemini has not replied after the initial wait, keep polling every 90 seconds until 15 minutes have elapsed since step A.

- **Initial wait (PR size–based):** Get PR stats: `gh pr view <PR> --json changedFiles,additions,deletions`. Compute `totalChanges = additions + deletions`.  
  - **Small:** `changedFiles ≤ 5` and `totalChanges < 250` → wait **2** minutes.  
  - **Medium:** `changedFiles ≤ 15` or `totalChanges < 1000` → wait **5** minutes.  
  - **Large:** else → wait **7** minutes.  
  If `.cursor/skills/publish-feature/optimum_wait_minutes.txt` exists and contains a positive integer **smaller** than this size-based value, use that instead (learned shorter wait).
- **If the user says Gemini has already finished:** skip the wait and go to step C; do not update the stored value.
- **Otherwise:**  
  1. Wait the chosen **initial_wait** minutes.  
  2. **Poll:** Fetch unresolved Gemini thread count using **Phase 1 only** (paginate with minimal fields: id, isResolved, comments(first:1){ nodes { author { login } } }; filter to isResolved==false and author=="gemini-code-assist"; count nodes). No path/line/body needed for count.  
  3. If count **> 0:** proceed to step C and **learn:** elapsed = minutes from step A (comment) to this poll. Write `ceil(elapsed)` to `optimum_wait_minutes.txt`, capped at **15**. Future runs use this if it is smaller than the size-based wait.  
  4. If count **= 0:** wait **90 seconds**, poll again. **Repeat until count &gt; 0 or 15 minutes have passed since the last "/gemini review" comment (step A)**; then proceed to step C. Once 15 minutes have elapsed with still 0 threads, stop waiting and proceed to step C (with 0 threads, then exit). When count first became &gt; 0, use that poll’s elapsed time to update the file (capped at 15). If we hit 15 min with count still 0, set file to `min(15, stored+1)`.

### C. Fetch threads; fix only if needed

- **Fetch** unresolved review threads by `gemini-code-assist` for the PR using the **same two-phase fetch as fix-gemini-reviews** (Phase 1: paginate with minimal fields to get IDs; Phase 2: for each ID, `node(id)` to get path/line/body). Use the resulting list for fixes.
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
- **Max wait:** Poll every **90 seconds**, up to **15 minutes since the last "/gemini review" comment** (step A). Then proceed to step C and stop (no threads → exit).

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
| B | Wait = size-based (small: 2 min, medium: 5 min, large: 7 min); if file has smaller value use it. Then **poll every 90s, up to 15 min since last "/gemini review" comment** (step A); proceed when count &gt;0 or 15 min elapsed; update file with elapsed when threads appear (cap 15). If no response after 15 min, proceed to C and exit. |
| C | Fetch unresolved Gemini threads; if 0 → exit; else fix, run-check-all-incremental (if changes), resolve, push, then post `/gemini review` on the PR |
| D | If cycle &lt; 10 and threads &gt; 0 → go to A; else exit |

Exit when: **cycle = 10** or **unresolved Gemini threads = 0**.

**If GitHub rate limit is hit:** Run `gh api /rate_limit`, read `resources.graphql.reset` (or `core.reset`), report the reset time to the user and stop.
