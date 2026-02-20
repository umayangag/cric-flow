When the user types "/fix-gemini-reviews" with a PR number (or use current branch PR)

# Fix Gemini Reviews

Fetch unresolved PR review threads authored by `gemini-code-assist`, implement the suggested fixes, and resolve the threads via GraphQL. Use the two-phase fetch to keep responses small: Phase 1 paginate with minimal fields (id, isResolved, author only); Phase 2 for each thread ID use `node(id)` to get path/line/body.

## Prerequisites

- GitHub CLI (`gh`) authenticated with `repo` scope
- Run inside the target repo or pass `-R owner/repo`
- PR number: e.g. `53`, or `gh pr view --json number -q .number` for current branch
- Stay on the same branch for fixes

## 1. Fetch unresolved Gemini review threads (two-phase)

Use GraphQL via `gh api graphql`. Pass integers with `-F` to avoid coercion errors.

**Phase 1 — List unresolved Gemini thread IDs (paginate with minimal payload):**

```bash
PR=53   # or: gh pr view --json number -q .number
OWNER=$(gh repo view --json owner -q .owner.login)
REPO=$(gh repo view --json name -q .name)

UNRESOLVED_IDS=$(mktemp)
CURSOR=""
while true; do
  QUERY='query($owner: String!, $repo: String!, $number: Int!, $after: String) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        reviewThreads(first: 100, after: $after) {
          pageInfo { hasNextPage endCursor }
          nodes { id isResolved comments(first: 1) { nodes { author { login } } } }
        }
      }
    }
  }'
  if [ -z "$CURSOR" ]; then
    RESULT=$(gh api graphql -F number=$PR -f owner="$OWNER" -f repo="$REPO" -f query="$QUERY")
  else
    RESULT=$(gh api graphql -F number=$PR -f owner="$OWNER" -f repo="$REPO" -f query="$QUERY" -f after="$CURSOR")
  fi
  echo "$RESULT" | jq -r '.data.repository.pullRequest.reviewThreads.nodes[]
        | select((.isResolved==false) and (.comments.nodes[0]?.author.login=="gemini-code-assist"))
        | .id' >> "$UNRESOLVED_IDS"
  HAS_NEXT=$(echo "$RESULT" | jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage')
  [ "$HAS_NEXT" != "true" ] && break
  CURSOR=$(echo "$RESULT" | jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor')
done
```

**Phase 2 — Fetch path, line, body only for those thread IDs (batched to reduce cost):**

GitHub’s `nodes(ids)` is most efficient with at most 100 IDs per request. Batch IDs into chunks of 100 to stay under limits and reduce per-request cost.

```bash
PR_REVIEWS_JSON=$(mktemp)
NODES_QUERY='query($ids: [ID!]!) {
  nodes(ids: $ids) {
    ... on PullRequestReviewThread {
      id
      comments(first: 1) { nodes { path line body } }
    }
  }
}'
if [ -s "$UNRESOLVED_IDS" ]; then
  ID_LIST=$(jq -R -s 'split("\n") | map(select(length > 0))' "$UNRESOLVED_IDS")
  TOTAL=$(echo "$ID_LIST" | jq 'length')
  for start in $(seq 0 100 $((TOTAL - 1))); do
    end=$((start + 100))
    TIDS_JSON=$(echo "$ID_LIST" | jq ".[$start:$end]")
    gh api graphql -f query="$NODES_QUERY" -F ids="$TIDS_JSON" | \
      jq -c '.data.nodes[] | select(.!=null) | {id: .id, path: .comments.nodes[0]?.path, line: .comments.nodes[0]?.line, body: .comments.nodes[0]?.body}' >> "$PR_REVIEWS_JSON"
  done
fi
```

Use `$PR_REVIEWS_JSON` (or copy to `pr_reviews.json`) for the fix step. Sanity check: `jq -r 'select(.!=null) | .id' "$PR_REVIEWS_JSON" | wc -l`

**When GraphQL is rate-limited:** Use REST to fetch comments and implement fixes; resolving threads still requires GraphQL (run again after reset). Owner/repo from `git remote get-url origin`. PR: `gh api "/repos/${OWNER}/${REPO}/pulls?state=open&head=${OWNER}:${BRANCH}" -q '.[0].number'`. Comments: `gh api "/repos/${OWNER}/${REPO}/pulls/${PR}/comments?per_page=100" --paginate`. Filter: `jq -c '.[] | select(.user.login=="gemini-code-assist") | {path, line, body}'`. Use `line` or `original_line` and `path`, `body`. Report rate limit reset: `gh api /rate_limit` → `resources.graphql.reset`.

## 2. Implement fixes

For each entry in the reviews file:
- Open `path:line`, read `body` for context or a ```suggestion``` block
- Apply the change; confirm with user if the fix is suspicious
- Run project checks (e.g. `make check-all` or run-check-all-incremental) to verify

## 3. Resolve fixed threads (GraphQL)

Request only the minimal field to reduce mutation cost:

```bash
jq -r 'select(.!=null) | .id' "$PR_REVIEWS_JSON" | while read -r TID; do
  echo "Resolving $TID" >&2
  gh api graphql \
    -f query='mutation($id: ID!) { resolveReviewThread(input: {threadId: $id}) { thread { id } } }' \
    -f id="$TID"
done
```

## 4. Verify (optional — saves one full Phase 1 pagination if skipped)

Confirm no remaining unresolved Gemini threads (paginate as in Phase 1), then inspect `isResolved` for threads where author is `gemini-code-assist`. Expect only `true` after resolution. Skip this step if you do not need to verify; it runs a full Phase 1–style pagination and costs one extra query.

## 5. Report, commit, push, re-review

- Summarize how many comments were addressed and which files were modified
- Commit and push on the same branch:

```bash
git add .
git commit -m "Fix Gemini comments"
git push
```

- Trigger new Gemini review:

```bash
gh pr comment $PR --body "/gemini review"
```

## Notes

- Use the same branch for fixes; do not create a new branch
- If token lacks access: `gh auth login --scopes repo`
