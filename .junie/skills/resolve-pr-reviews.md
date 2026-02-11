When the user types `/review pr {{pr_number}}`, perform these steps using github mcp:

1. access the latest conversations on PR numer {{pr_number}}
2. do not create new branch. use the same branch as the PR.
3. fix the latest unresolved review comments/coversations given by gemini code assist.
4. fix unit tests and lint errors using `make lint` and `make test`.
5. then commit the changes to the PR and add a new comment "/gemini review".


