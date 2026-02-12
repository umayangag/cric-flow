When the user types "/fix reviews {{pr_number}}"

# Fix Gemini Reviews
Description: Fetch unresolved PR review threads authored by `gemini-code-assist` that are still unresolved, implement the suggested fixes, and resolve the threads via GraphQL. This documents the exact, working `gh` + GraphQL commands to avoid trial-and-error.

## Prerequisites
- GitHub CLI (gh) authenticated: `gh auth status` should show a token with `repo` scope for the target repo.
- Ensure repo context is correct (run inside the repo or pass `-R owner/repo`).
- Have the PR number handy (e.g., `53`). You can also fetch it from current branch: `gh pr view --json number -q .number`.
- Stay in the same branch for fixes.

## 1) Fetch unresolved Gemini review threads (exact, working)
Use a single GraphQL query via `gh api graphql`. Important details we learned:
- Pass integers with `-F` so they are typed as Int (avoids "Could not coerce value" errors).
- Query `reviewThreads` and take the first comment to identify the author and file context.
- Filter in `jq` for `isResolved == false` and `author.login == "gemini-code-assist"`.

Save results (id, path, line, body) to a file for downstream automation:

```bash
PR=53
# Optional: set explicit repo; otherwise rely on cwd
OWNER=$(gh repo view --json owner -q .owner.login)
REPO=$(gh repo view --json name  -q .name)

# Fetch review threads and filter unresolved Gemini ones
gh api graphql \
  -F number=$PR \
  -f owner="$OWNER" -f repo="$REPO" \
  -f query='query($owner: String!, $repo: String!, $number: Int!) { repository(owner: $owner, name: $repo) { pullRequest(number: $number) { reviewThreads(first: 100) { nodes { id isResolved comments(first: 1) { nodes { author { login } path line body } } } } } } }' \
  --jq '.data.repository.pullRequest.reviewThreads.nodes[]
        | select((.isResolved==false) and (.comments.nodes[0].author.login=="gemini-code-assist"))
        | {id: .id, path: .comments.nodes[0].path, line: .comments.nodes[0].line, body: .comments.nodes[0].body}' \
  > pr_reviews.json

# Quick sanity: count unresolved Gemini threads
jq -r 'select(.!=null) | .id' pr_reviews.json | wc -l
```

Tips:
- To list nicely in the terminal without saving: append `| jq -r '"\(.id) | \(.path):\(.line) | \(.body|gsub("\n"; " "))"'`.
- If your repo is private and token lacks access, `gh` will error; fix by `gh auth login --scopes repo`.

## 2) Implement fixes
- For each entry in `pr_reviews.json`, open `path:line`, read `body` for context or a ```suggestion``` block, and apply the change. If the fix is suspicious or not needed confirm with the user.
- Run linters/tests as usual (`make lint && make test`).

## 3) Resolve fixed threads (GraphQL mutation)
Resolution is only available via GraphQL. Use the exact mutation below. Single thread:

```bash
THREAD_ID="<THREAD_ID>"
gh api graphql \
  -f query='mutation($id: ID!) { resolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }' \
  -f id="$THREAD_ID"
```

Batch resolve all IDs in `pr_reviews.json` after applying fixes:

```bash
jq -r 'select(.!=null) | .id' pr_reviews.json | while read -r TID; do
  echo "Resolving $TID" >&2
  gh api graphql \
    -f query='mutation($id: ID!) { resolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }' \
    -f id="$TID"
done
```

## 4) Optional helpers
- Current PR number on this branch: `gh pr view --json number -q .number`
- Confirm there are no remaining unresolved Gemini threads:

```bash
gh api graphql \
  -F number=$PR -f owner="$OWNER" -f repo="$REPO" \
  -f query='query($owner: String!, $repo: String!, $number: Int!) { repository(owner: $owner, name: $repo) { pullRequest(number: $number) { reviewThreads(first: 100) { nodes { isResolved comments(first: 1) { nodes { author { login } } } } } } } }' \
  --jq '.data.repository.pullRequest.reviewThreads.nodes[]
        | select((.comments.nodes[0].author.login=="gemini-code-assist"))
        | .isResolved' | sort | uniq -c
# Expect only "true" remaining after resolution
```

## 5) Reporting
- Summarize how many Gemini comments were addressed, which files were modified, and attach `pr_reviews.json` as an artifact if useful.

## 6) Push the fixes
```bash
git add .
git commit -m "Fix Gemini comments"

## 7) Trigger re-review
Finally, once you ensure all local changes are commited and pushed, post a comment to trigger a new Gemini review:
```bash
gh pr comment $PR --body "/gemini review"
```

Notes:
- use the same branch for fixes. do not create a new branch.