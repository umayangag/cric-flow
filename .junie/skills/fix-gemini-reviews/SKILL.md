when user types "/fix reviews{{pr_number}}"
# Fix Gemini Reviews
**Description**: Fetches unresolved PR comments from `gemini-code-assist[bot]`, implements the suggested fixes, and resolves the threads.

## Instructions
1. **Fetch PR Context**:
   - Determine the current PR number (ask the user or use `gh pr view --json number -q .number`).
   - Use `gh pr view <PR_NUMBER> --json reviewThreads` to get all threads.

2. **Filter & Analyze**:
   - Identify unresolved threads where the first comment author is `gemini-code-assist[bot]`.
   - For each thread, extract:
     - The thread `id` (needed for resolution).
     - The file `path` and `line` number.
     - The comment `body` (feedback) and any ```suggestion``` blocks.

3. **Implement Fixes**:
   - For each identified issue:
     - Read the affected file at the specified line.
     - If a `suggestion` block exists, apply it exactly.
     - If only feedback exists, analyze the code and implement a fix that addresses Gemini's concern (correctness, efficiency, or security).
     - Verify the change doesn't break syntax or existing logic.

4. **Verify & Resolve**:
   - After applying changes, run project tests (if available) to ensure everything is green.
   - For each fixed thread, use the GitHub GraphQL API to mark it as resolved:
     ```bash
     gh api graphql -f query='mutation($id: ID!) { resolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }' -f id="<THREAD_ID>"
     ```

5. **Report**:
   - Provide a summary of how many comments were addressed and which files were modified.