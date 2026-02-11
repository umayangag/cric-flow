When the user types `/review pr {{pr_number}}`, perform these steps using github mcp:

1. access the last conversation on PR numer {{pr_number}}
2. fix the latest unresolved review comments in that conversation and mark those comments as resolve or add a reply comment to confirm the issue was fixed.
3. fix unit tests and lint errors using `make lint` and `make test`.
4. then commit the changes to the PR and add a new comment "/gemini review".


