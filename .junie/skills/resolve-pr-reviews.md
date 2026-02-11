When the user types `/review pr {{pr_number}}`, perform these steps using github mcp:

Access the GitHub PR comments for this branch. Identify the most recent review summary from gemini-code-assist[bot] 
and implement all the code suggestions mentioned in that specific review.
fix unit tests and lint errors using `make lint` and `make test`.
once the changes are committed, use the GitHub API to resolve each of the conversation threads you addressed.
add a new comment "/gemini review".