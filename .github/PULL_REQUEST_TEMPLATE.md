## Summary

- [ ] Purpose of this PR (what and why):

## Testing & Verification

- [ ] Ran `cd go-app && go test -race -cover ./...`
- [ ] (If Python changed) Ran `cd ml-service && pytest -q`

## Go Unit Test Standards Checklist (required)

For any new/modified Go tests, confirm adherence to our gold standard (see `docs/quality-and-debugging.md`).

- [ ] Table-driven tests with subtests via `t.Run`
- [ ] `t.Parallel()` at test function start (and in subtests only if fully isolated)
- [ ] External test package used (e.g., `package foo_test`) where feasible
- [ ] Clear Arrange → Act → Assert structure in each subtest
- [ ] All mock interactions defined in Arrange (`EXPECT()` calls) with explicit call count assertions as needed
- [ ] Deterministic fixtures; no randomness/time without injection
- [ ] Fail-fast assertions via `require` for critical checks; no conditional logic in tests

## Additional Notes

- [ ] Docs updated if behavior/commands changed (README/docs)
- [ ] No secrets committed; env vars documented in `.env.example` if needed
