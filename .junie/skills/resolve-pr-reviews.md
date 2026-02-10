When the user types `/review pr {{pr_number}}`, perform these steps using github mcp:

1. access the comment on PR numer {{pr_number}}
2. fix the review comments from gemini code assist.
4. fix unit tests and lint errors using using `make lint` and `make test`.
4. then commit the changes to the PR and add a new comment "/gemini review".

