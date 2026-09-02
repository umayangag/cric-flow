# Go App — Unit Test Guidelines

All unit tests in `go-app/` **must** follow these conventions. CI and code review
will enforce them.

---

## 1. External Test Packages

Always use the external test package so tests exercise the **public API** only:

```go
package foo_test          // ✅ correct
// package foo            // ❌ never use the internal package
```

> **Do NOT export private methods just for testing.** Write tests against the
> public surface; structure your code so that private logic is covered
> transitively through public methods.

---

## 2. Table-Driven Tests

- Name the slice **`testCases`** (not `tests`, `tt`, `tcs`, `cases`).
- Use a **single anonymous struct** that covers all fields for the test.
- Use **spaces** in test-case names for readability.

```go
testCases := []struct {
    name     string
    input    int
    expected int
}{
    {
        name:     "positive number",
        input:    5,
        expected: 25,
    },
    {
        name:     "zero value",
        input:    0,
        expected: 0,
    },
}
```

---

## 3. Parallel Subtests — No `for _, tc := range`

When subtests run in parallel, a closure over the loop variable can cause
races. **Do not** use `for _, tc := range testCases`. Instead, use an
index-based loop or reassign inside the loop body:

```go
for i := range testCases {
    tc := testCases[i]
    t.Run(tc.name, func(t *testing.T) {
        t.Parallel()
        // ...
    })
}
```

---

## 4. SUT Instantiation Inside `t.Run`

Create the system-under-test (SUT) and call the function **inside**
`t.Run(...)`, never before it. This keeps each subtest fully isolated:

```go
for i := range testCases {
    tc := testCases[i]
    t.Run(tc.name, func(t *testing.T) {
        t.Parallel()

        // Arrange — create SUT here
        svc := mypackage.NewService(tc.dep)

        // Act
        got, err := svc.DoSomething(tc.input)

        // Assert
        require.NoError(t, err)
        assert.Equal(t, tc.expected, got)
    })
}
```

---

## 5. Testify Assertions Only

Use `github.com/stretchr/testify/assert` and `require` for **all**
assertions. Never use raw `if got != want { t.Fatalf(...) }` patterns:

```go
// ✅ correct
assert.Equal(t, want, got)
require.NoError(t, err)
assert.Contains(t, body, "success")

// ❌ wrong
if got != want {
    t.Fatalf("expected %v, got %v", want, got)
}
```

Use `require` for preconditions that must hold (e.g., no error) and `assert`
for the actual test expectations.

---

## 6. Mocks

- Place mocks in the `internal/mocks` **sibling directory** of the package
  under test.
- Generate mocks from `contract.go` with [mockery](https://github.com/vektra/mockery),
  driven by `go-app/.mockery.yml`. Run `make mock` from the repo root — the repo
  deliberately avoids `go:generate` for mocks in favour of that one target.

```
internal/
  foo/
    contract.go       # interfaces
    service.go
    mocks/
      MockFooClient.go  # generated
```

---

## 7. Prefer Methods Over Functions

Where it makes sense, prefer **methods on a receiver** over standalone
functions. This improves testability (interfaces + mocks) and keeps related
behaviour grouped.

---

## 8. AAA Pattern

Structure every test body as **Arrange → Act → Assert**:

```go
t.Run(tc.name, func(t *testing.T) {
    // Arrange
    repo := mocks.NewMockRepo(t)
    repo.On("Get", tc.id).Return(tc.item, tc.err)
    svc := mypackage.NewService(repo)

    // Act
    result, err := svc.Fetch(tc.id)

    // Assert
    require.NoError(t, err)
    assert.Equal(t, tc.expected, result)
    repo.AssertExpectations(t)
})
```

---

## 9. Naming

- Test files: `{source}_test.go` (e.g., `service_test.go`).
- Test functions: `TestPublicMethod_scenario` or `TestPublicMethod`.
- Use `t.Helper()` in custom assertion / setup helpers.
- Use `t.Cleanup()` or `defer` for teardown.

---

## Quick Checklist

| # | Rule | Check |
|---|------|-------|
| 1 | External test package (`package foo_test`) | ☐ |
| 2 | Table slice named `testCases` | ☐ |
| 3 | Spaces in test-case names | ☐ |
| 4 | No `for _, tc := range` in parallel subtests | ☐ |
| 5 | SUT inside `t.Run` | ☐ |
| 6 | Testify only (no `t.Fatal`/`t.Error`) | ☐ |
| 7 | Mocks from `internal/mocks` via `make mock` | ☐ |
| 8 | No exported privates for testing | ☐ |
| 9 | AAA pattern | ☐ |
