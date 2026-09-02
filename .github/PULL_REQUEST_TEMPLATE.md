## Summary

- [ ] Purpose of this PR (what and why):

## Testing & Verification

- [ ] Ran the checks for every component touched (`make check-all`, or the component targets)
- [ ] (If Go changed) Ran `make -C go-app coverage` then `make -C go-app coverage-check`
- [ ] (If Python changed) Ran `PATH="$(pwd)/ml-service/.venv/bin:$PATH" make -C ml-service coverage`

## Go Unit Test Standards Checklist (required)

For any new/modified Go tests, confirm adherence to our gold standard (see `go-app/docs/testing-guidelines.md`, summarised in `docs/quality-and-debugging.md`).

- [ ] Table-driven tests with subtests via `t.Run`
- [ ] `t.Parallel()` at test function start (and in subtests only if fully isolated)
- [ ] External test package used (e.g., `package foo_test`) where feasible
- [ ] Clear Arrange → Act → Assert structure in each subtest
- [ ] All mock interactions defined in Arrange (`EXPECT()` calls) with explicit call count assertions as needed
- [ ] Deterministic fixtures; no randomness/time without injection
- [ ] Fail-fast assertions via `require` for critical checks; no conditional logic in tests

## Additional Notes

- [ ] Docs updated if behavior/commands changed (README/docs); `make gen-architecture-map` re-run if contracts changed
- [ ] No secrets committed; env vars documented in `.env.example` if needed
