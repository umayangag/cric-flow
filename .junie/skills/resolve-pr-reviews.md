When the user types `/review pr {{pr_number}}`, perform these steps using github mcp:

1. access the comment on PR numer {{pr_number}}
2. use context provider to understand the codebase and review comments.
3. do not create new branch. use the same branch as the PR.
4. fix the review comments from gemini code assist.
5. fix unit tests and lint errors using `make lint` and `make test`.
6. then commit the changes to the PR and add a new comment "/gemini review".

