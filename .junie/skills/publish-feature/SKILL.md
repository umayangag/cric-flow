When the user says "/publish-feature"

# Publish feature (review loop)

Runs a publish loop: ensure all checks pass, trigger Gemini PR review, wait 15 minutes, fix Gemini comments and re-run checks before pushing, then repeat. Stop after **10 cycles** or when there are **no unresolved Gemini review threads**. First resolve any **existing** unresolved Gemini comments on the PR (if present), then run the main cycle.

## Prerequisites

- GitHub CLI (`gh`) authenticated with `repo` scope
- Current branch has commits to publish (PR may or may not exist yet)

**Thread fetch (same as fix-gemini-reviews):** Use the two-phase approach to keep responses small. **Phase 1** — paginate `reviewThreads` with minimal fields (`id`, `isResolved`, `comments(first:1){ nodes { author { login } } }` only); filter to `isResolved==false` and `author.login=="gemini-code-assist"`. For **count only** (e.g. single check after 15 min), use Phase 1 and count nodes. For **full thread data** (to implement fixes), add **Phase 2** — for each thread ID, query `node(id)` for `comments(first:1){ nodes { path line body } }`. Resolve threads via the GraphQL mutation from fix-gemini-reviews.

**Rate limit (GraphQL / gh blocked):** If any `gh` call fails with a rate-limit message (e.g. `API rate limit already exceeded`, `rate limit exceeded`, or similar), do **not** retry in a loop. (1) Fetch rate limit expiry with `gh api /rate_limit` (REST; may still succeed when GraphQL is limited). Read `resources.graphql.reset` (or `resources.core.reset`) and report: "GitHub API rate limit hit. GraphQL (or core) resets at \<ISO or local time\>. Run /publish-feature again after that time." (2) **Optional REST fallback:** Use REST (see below) to get the PR number and list **all** review comments by `gemini-code-assist` for that PR. Implement fixes from path/line/body, run **run-check-all-incremental** if there are changes, commit and push. **Limitation:** REST does not expose which threads are unresolved, and **resolving threads is only possible via GraphQL**. So after the REST-based fix and push, tell the user to run /publish-feature again after the rate limit resets so threads can be resolved. If REST also fails (e.g. 403/404), just report the reset time and stop.

**REST fallback — get PR number and Gemini review comments (no unresolved filter; resolve still needs GraphQL):** Owner/repo from git: `OWNER_REPO=$(git remote get-url origin | sed -n 's/.*github\.com[:/]\([^/]*\/[^./]*\).*/\1/p')` then `OWNER=${OWNER_REPO%/*}` and `REPO=${OWNER_REPO#*/}` (strip `.git` from REPO if present). Branch: `BRANCH=$(git branch --show-current)`. (1) Get PR number: `gh api "/repos/${OWNER}/${REPO}/pulls?state=open&head=${OWNER}:${BRANCH}" -q '.[0].number'`. (2) List review comments (paginated): `gh api "/repos/${OWNER}/${REPO}/pulls/${PR}/comments?per_page=100" --paginate`. (3) Filter to Gemini only: pipe the JSON array to `jq -c '.[] | select(.user.login=="gemini-code-assist") | {path, line, body}'`. Use `path` and `line` (REST uses `line`; if missing use `original_line`) and `body` to implement fixes. You cannot resolve these threads via REST; run the workflow again after GraphQL reset to resolve.

---

## Initial setup (before the cycle)

### 1. Run checks

Apply **run-check-all-incremental** (frontend → go-app → ml-service) until all pass. Do not proceed until all pass. In the main cycle, run it again only **before push and only if there are uncommitted changes** when fixing threads.

### 2. Ensure PR exists (do not comment yet)

- **If a PR exists for the current branch:** note the PR number (e.g. `gh pr view --json number -q .number` or `gh pr list --head $(git branch --show-current) -q .number`).
- **If no PR exists:** create one (`gh pr create --fill` or equivalent), then note the PR number.

### 3. Check for existing unresolved Gemini threads

- **Fetch** unresolved review threads by `gemini-code-assist` for the PR using the **two-phase fetch** (Phase 1: paginate with minimal fields — id, isResolved, author only — and collect IDs; Phase 2: for each ID, `node(id)` to get path/line/body). See fix-gemini-reviews skill for exact GraphQL.
- **If count > 0:** There are existing comments. **Resolve them first:** implement the suggested fixes (per-thread path/line/body or ```suggestion```), run **run-check-all-incremental** only if there are uncommitted changes (fix and re-run failed part until all pass), resolve the fixed threads via GraphQL, commit and push (e.g. `git add -u && git commit -m "Fix Gemini comments" && git push`). Then **start the main cycle**: do step A (post `/gemini review` on the PR), then B, C, D.
- **If count = 0:** No existing comments. **Start the main cycle**: do step A (post `/gemini review` on the PR), then B, C, D. Do not skip step A — without it there will be no suggestions.

---

## Main cycle (repeat until exit condition)

**Mandatory:** Every cycle iteration **must** begin with step A. Do **not** run step B or step C until you have posted `/gemini review` on the PR in this iteration. Without this comment, Gemini does not run and there will be no review suggestions.

**Cycle counter:** Track the current cycle (1–10). At the start of each iteration, state the cycle number.

### A. Request Gemini review

Post a comment on the PR with body `/gemini review`:

```bash
gh pr comment <PR> --body "/gemini review"
```

Do this **first**, before waiting or fetching. Then proceed to step B.

### B. Wait 15 minutes, then check once (minimize GraphQL usage)

**Wait 15 minutes after posting "/gemini review", then check the PR once for new comments.** Do not poll; a single check minimizes GraphQL usage.

- **If the user says Gemini has already finished:** skip the wait and go to step C.
- **Otherwise:** wait 15 minutes, then **check once**: fetch unresolved Gemini thread count using **Phase 1 only** (paginate with minimal fields: id, isResolved, comments(first:1){ nodes { author { login } } }; filter to isResolved==false and author=="gemini-code-assist"; count nodes). No path/line/body needed for count.
- Proceed to step C. If count = 0, step C will exit the loop (no comments by then → stop).

### C. Fetch threads; fix only if needed

- **Fetch** unresolved review threads by `gemini-code-assist` for the PR using the **two-phase fetch** (Phase 1: paginate with minimal fields to get IDs; Phase 2: for each ID, `node(id)` to get path/line/body). Use the resulting list for fixes. (If step B already did Phase 1 and count was 0, treat as 0 threads and exit without a second fetch.)
- **If 0 threads:** exit the loop (no fix, no check, no push). Summarize and stop. If no comments appeared after the 15-minute wait, report that and stop.
- **If 1+ threads:** continue:
  - Implement the suggested fixes (per-thread path/line/body or ```suggestion```).
  - **Before pushing:** run **run-check-all-incremental** only if there are uncommitted changes; fix failures and re-run only the failed part until all pass.
  - Resolve the fixed threads via GraphQL, then commit and push (e.g. `git add -u && git commit -m "Fix Gemini comments" && git push`).
  - **After every push:** post `/gemini review` on the PR so Gemini runs again on the new commits:

```bash
gh pr comment <PR> --body "/gemini review"
```

### D. Loop or exit

- **Exit** if cycle = 10 or (after step C) unresolved Gemini thread count = 0.
- Otherwise **go back to step A** (post `/gemini review` again, then B, C, D).

---

## Summary table

**Initial setup:**
| Step | Action |
|------|--------|
| 1 | run-check-all-incremental until all pass (once; again only before push when there are local changes) |
| 2 | Create PR if needed; note PR number (do not comment yet) |
| 3 | Fetch unresolved Gemini threads; if > 0 → fix, run-check-all-incremental (if changes), resolve, push; then start cycle. If 0 → start cycle. |

**Main cycle (do A then B then C then D; never skip A):**
| Step | Action |
|------|--------|
| A | **Post** `/gemini review` on PR (required first; no suggestions without it) |
| B | **Wait 15 minutes** (no polling); then **check once** for unresolved Gemini threads (Phase 1 count only). Proceed to C. If no comments by then, C exits and stop. |
| C | Fetch unresolved Gemini threads; if 0 → exit; else fix, run-check-all-incremental (if changes), resolve, push, then post `/gemini review` on the PR |
| D | If cycle < 10 and threads > 0 → go to A; else exit |

**Exit when:** cycle = 10 or unresolved Gemini threads = 0.

**If GitHub rate limit is hit:** Run `gh api /rate_limit`, read `resources.graphql.reset` (or `core.reset`), report the reset time. Optionally use **REST fallback** (repos/owner/repo/pulls?head=, then pulls/{number}/comments; filter by user.login) to fetch Gemini comments, implement fixes, commit and push; tell user to run again after reset to resolve threads (resolve is GraphQL-only).
