When the user types `/review pr {{pr_number}}`, perform these steps using github mcp:

1. access the comment on PR numer {{pr_number}}
2. fix the review comments from gemini code assist. 
3. once each suggestion is fixed, resolve each comment one by one. 
4. fix unit tests and lint errors using `make lint` and `make test`.
4. then commit the changes to the PR and add a new comment "/gemini review". 
5. then wait until any new review comments appear (which might take upto 5 minutes)
6. repeat from number 1. until all review comments are resolved or review comments are negligible.
