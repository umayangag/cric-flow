---
name: fix-gemini-reviews
description: Fetches unresolved PR review threads authored by gemini-code-assist, implements the suggested fixes, and resolves threads via GraphQL. Use when the user types "/fix-gemini-reviews" with a PR number, or when asked to fix Gemini PR review comments.
---

# Fix Gemini Reviews

Implements suggested fixes from unresolved PR review threads by `gemini-code-assist`, then resolves those threads via GitHub GraphQL.

## Prerequisites

- GitHub CLI (`gh`) authenticated with `repo` scope
- Run inside the target repo or pass `-R owner/repo`
- Stay on the same branch for fixes

## 1. Fetch unresolved Gemini review threads

Use GraphQL via `gh api graphql`. Pass integers with `-F` to avoid coercion errors. Filter for `isResolved == false` and `author.login == "gemini-code-assist"`. Uses pagination to fetch all threads (PRs with >100 threads are rare but possible).

```bash
PR=53  # or: gh pr view --json number -q .number
OWNER=$(gh repo view --json owner -q .owner.login)
REPO=$(gh repo view --json name -q .name)

> pr_reviews.json
CURSOR=""
while true; do
  QUERY='query($owner: String!, $repo: String!, $number: Int!, $after: String) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        reviewThreads(first: 100, after: $after) {
          pageInfo { hasNextPage endCursor }
          nodes { id isResolved comments(first: 1) { nodes { author { login } path line body } } }
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
        | select((.isResolved==false) and (.comments.nodes[0].author.login=="gemini-code-assist"))
        | {id: .id, path: .comments.nodes[0].path, line: .comments.nodes[0].line, body: .comments.nodes[0].body}' >> pr_reviews.json
  HAS_NEXT=$(echo "$RESULT" | jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage')
  [ "$HAS_NEXT" != "true" ] && break
  CURSOR=$(echo "$RESULT" | jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor')
done
```

Sanity check: `jq -r 'select(.!=null) | .id' pr_reviews.json | wc -l`

## 2. Implement fixes

For each entry in `pr_reviews.json`:
- Open `path:line`, read `body` for context or ```suggestion``` block
- Apply the change; confirm with user if fix is suspicious
- Run `make check-all` to verify

## 3. Resolve fixed threads (GraphQL)

```bash
jq -r 'select(.!=null) | .id' pr_reviews.json | while read -r TID; do
  echo "Resolving $TID" >&2
  gh api graphql \
    -f query='mutation($id: ID!) { resolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }' \
    -f id="$TID"
done
```

## 4. Verify

Confirm no remaining unresolved Gemini threads (uses pagination for PRs with many threads):

```bash
> /tmp/gemini_resolved.txt
CURSOR=""
while true; do
  QUERY='query($owner: String!, $repo: String!, $number: Int!, $after: String) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        reviewThreads(first: 100, after: $after) {
          pageInfo { hasNextPage endCursor }
          nodes { isResolved comments(first: 1) { nodes { author { login } } } }
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
        | select(.comments.nodes[0].author.login=="gemini-code-assist")
        | .isResolved' >> /tmp/gemini_resolved.txt
  HAS_NEXT=$(echo "$RESULT" | jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage')
  [ "$HAS_NEXT" != "true" ] && break
  CURSOR=$(echo "$RESULT" | jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor')
done
echo "Gemini threads by status:"
sort /tmp/gemini_resolved.txt | uniq -c
# Expect only "true" remaining
```

## 5. Report, commit, push, re-review

- Summarize fixes, files modified; keep `pr_reviews.json` as artifact if useful
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
