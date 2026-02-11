When the user types `/review pr {{pr_number}}`, perform these steps using github mcp:

1. access the comment on PR numer {{pr_number}}
2. use context provider to understand the codebase and review comments.
3. do not create plans.
4. do not create new branch. use the same branch as the PR.
5. fix the unresolved review comments from gemini code assist.
6. fix unit tests and lint errors using `make lint` and `make test`.
7. then commit the changes to the PR and add a new comment "/gemini review".
8. resolve the comments in the PR you already addressed.

