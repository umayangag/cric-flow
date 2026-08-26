<!-- Generated from ../../.cursor/skills/publish-feature/SKILL.md by scripts/sync-junie-skills.py. Edit the Cursor copy, then re-run the script. -->

When the user says "/publish-feature" — Runs a publish loop: ensure all checks pass, trigger Gemini PR review, wait, fix Gemini comments and re-run checks before pushing, then repeat. Use when the user says "/publish-feature" to iterate on a PR until review is clean or max cycles reached.

# Publish feature (review loop)

When the user says **/publish-feature**, run the following workflow. First resolve any **existing** unresolved Gemini comments on the PR (if present), then run the main cycle. Stop after **10 cycles** or when there are **no unresolved Gemini review threads**.

---

## Prerequisites

- GitHub CLI (`gh`) authenticated with `repo` scope
- Current branch has commits to publish (PR may or may not exist yet)

**Thread fetch (same as fix-gemini-reviews):** Use the two-phase approach to keep responses small. **Phase 1** — paginate `reviewThreads` with minimal fields (`id`, `isResolved`, `comments(first:1){ nodes { author { login } } }` only); filter to `isResolved==false` and `author.login=="gemini-code-assist"`; write IDs to a file. **Phase 2** — for those IDs, query `nodes(ids)` in **chunks of 100** for `comments(first:1){ nodes { path line body } }` only. In the main cycle, step B runs Phase 1 and saves IDs; step C reuses that file and runs Phase 2 only (no second Phase 1). Resolve via the same GraphQL mutation as fix-gemini-reviews (`thread { id }` only).

**Rate limit (GraphQL / gh blocked):** If any `gh` call fails with a rate-limit message (e.g. `API rate limit already exceeded`, `rate limit exceeded`, or similar), do **not** retry in a loop. (1) Fetch rate limit expiry with `gh api /rate_limit` (REST; may still succeed when GraphQL is limited). Read `resources.graphql.reset` (or `resources.core.reset`) and report to the user: **"GitHub API rate limit hit. GraphQL (or core) resets at \<ISO or local time\>. Run /publish-feature again after that time."** (2) **Optional REST fallback:** Use REST (see below) to get the PR number and list **all** review comments by `gemini-code-assist` for that PR. Implement fixes from path/line/body, run **run-check-all-incremental** if there are changes, commit and push. **Limitation:** REST does not expose which threads are unresolved, and **resolving threads is only possible via GraphQL**. So after the REST-based fix and push, tell the user to run /publish-feature again after the rate limit resets so threads can be resolved (and the main cycle can use GraphQL). If REST also fails (e.g. 403/404), just report the reset time and stop.

**REST fallback — get PR number and Gemini review comments (no unresolved filter; resolve still needs GraphQL):** Use when GraphQL is rate-limited but REST is available. Owner/repo from git: `OWNER_REPO=$(git remote get-url origin | sed -n 's/.*github\.com[:/]\([^/]*\/[^./]*\).*/\1/p')` then `OWNER=${OWNER_REPO%/*}` and `REPO=${OWNER_REPO#*/}` (strip `.git` from REPO if present). Branch: `BRANCH=$(git branch --show-current)`. (1) Get PR number: `gh api "/repos/${OWNER}/${REPO}/pulls?state=open&head=${OWNER}:${BRANCH}" -q '.[0].number'`. (2) List review comments (paginated): `gh api "/repos/${OWNER}/${REPO}/pulls/${PR}/comments?per_page=100" --paginate`. (3) Filter to Gemini only and extract path, line, body: pipe the JSON array to `jq -c '.[] | select(.user.login=="gemini-code-assist") | {path, line, body}'`. Use `path` and `line` (REST uses `line` for the line in the file; if missing use `original_line`) and `body` to implement fixes. You cannot resolve these threads via REST; run the workflow again after GraphQL reset to resolve.

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

### B. Wait 15 minutes, then check once (minimize GraphQL usage)

**Wait 15 minutes after posting "/gemini review", then check the PR once for new comments.** Do not poll; a single check minimizes GraphQL usage.

- **If the user says Gemini has already finished:** skip the wait and go to step C.
- **Otherwise:** wait 15 minutes, then **check once**: run **Phase 1 only** (same as fix-gemini-reviews: paginate with minimal fields id, isResolved, comments(first:1){ nodes { author { login } } }; filter to isResolved==false and author=="gemini-code-assist"). **Write the unresolved thread IDs to a file** (e.g. `CYCLE_UNRESOLVED_IDS`). Count = number of IDs. No path/line/body in this step.
- Proceed to step C. If count = 0, step C will exit the loop (no comments by then → stop).

### C. Fetch threads; fix only if needed

- **If count = 0** (from step B): exit the loop (no fix, no check, no push). Summarize and stop.
- **If count > 0:** **Reuse the ID file from step B** — do **not** re-run Phase 1. Run **Phase 2 only** (same as fix-gemini-reviews: for those IDs, query `nodes(ids)` in chunks of 100 for path/line/body). Use the resulting list for fixes. This avoids one full Phase 1 pagination per cycle. Then:
  - Implement the suggested fixes (per-thread path/line/body or ```suggestion```).
  - **Before pushing:** run **run-check-all-incremental** only if there are uncommitted changes; fix failures and re-run only the failed part until all pass.
  - Resolve the fixed threads via GraphQL, then commit and push (e.g. `git add -u && git commit -m "Fix Gemini comments" && git push`).
  - Step C ends with the push. The next iteration's step A will post `/gemini review` to trigger review of the new commits; do **not** post it here (that would duplicate the request).

### D. Loop or exit

- **Exit** if cycle = 10 or (after step C) unresolved Gemini thread count = 0.
- Otherwise **go back to step A** (post `/gemini review` again, then B, C, D).

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
| B | **Wait 15 minutes** (no polling); then **check once**: run Phase 1, write unresolved IDs to a file, count = number of IDs. Proceed to C. If count 0, C exits and stop. |
| C | If count 0 → exit. Else **reuse step B’s ID file**; run Phase 2 only (nodes in chunks of 100). Fix, run-check-all-incremental (if changes), resolve, push. |
| D | If cycle &lt; 10 and threads &gt; 0 → go to A; else exit |

Exit when: **cycle = 10** or **unresolved Gemini threads = 0**.

**If GitHub rate limit is hit:** Run `gh api /rate_limit`, read `resources.graphql.reset` (or `core.reset`), report the reset time. Optionally use **REST fallback** (repos/owner/repo/pulls?head=, then pulls/{number}/comments; filter by user.login) to fetch Gemini comments, implement fixes, commit and push; tell user to run again after reset to resolve threads (resolve is GraphQL-only).
