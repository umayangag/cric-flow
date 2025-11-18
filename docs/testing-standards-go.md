# Go Unit Testing Standards

These standards define the gold‑standard unit test style for the Go codebase, based on
`go-app/internal/services/weatherimport/service_test.go`. All new and refactored tests
must follow this guidance.

## Core Principles
- Table‑driven tests with subtests via `t.Run` and `t.Parallel()` at the test level.
- External test packages (`package xxx_test`) to validate the public API and reduce coupling.
- Clear 3‑phase structure per subtest: Arrange → Act → Assert.
- Declare all mock interactions in the Arrange phase; avoid hidden side effects.
- Deterministic, small fixtures near the test; avoid randomness and `time.Now()`.
- Fail‑fast assertions with `require` for critical checks; prefer `require` over `assert` unless non‑fatal checks are needed.
- No branching in test logic (no `if` in the happy/error path checks); vary cases via table entries.
- Name tests by unit and pattern, e.g., `TestService_Import_Table`.

## Mocks and Expectations
- Use generated mocks under the corresponding package’s `internal/mocks` directory.
- Fresh mocks per subtest (no sharing across tests or subtests).
- Define `EXPECT()` calls in Arrange and assert call counts explicitly when relevant
  (e.g., `AssertNumberOfCalls`).

## Canonical Skeleton
```go
package yourpkg_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    yourpkg "github.com/umayangag/cric-info-scrapers/go-app/internal/yourpkg"
    "github.com/umayangag/cric-info-scrapers/go-app/internal/yourpkg/internal/mocks"
)

func TestYourUnit_Behavior_Table(t *testing.T) {
    t.Parallel()

    // Base fixtures
    base := someFixture()

    cases := []struct {
        name    string
        input1  Type
        input2  Type
        arrange func(ctx context.Context, dep1 *mocks.MockDep1, dep2 *mocks.MockDep2)
        assert  func(t *testing.T, got OutputType, err error, dep2 *mocks.MockDep2)
    }{
        {
            name:   "happy-path applies changes",
            input1: ..., input2: ...,
            arrange: func(ctx context.Context, d1 *mocks.MockDep1, d2 *mocks.MockDep2) {
                d1.EXPECT().Method(ctx, ...).Return(...)
                d2.EXPECT().Write(ctx, ...).Return(nil)
            },
            assert: func(t *testing.T, got OutputType, err error, d2 *mocks.MockDep2) {
                require.NoError(t, err)
                require.Equal(t, want, got)
                d2.AssertNumberOfCalls(t, "Write", 1)
            },
        },
        {
            name:   "short-circuits on invalid input",
            input1: invalid,
            arrange: func(context.Context, *mocks.MockDep1, *mocks.MockDep2) {
                // no expectations — must fail early
            },
            assert: func(t *testing.T, _ OutputType, err error, _ *mocks.MockDep2) {
                require.Error(t, err)
                require.ErrorContains(t, err, "invalid")
            },
        },
        // ... more cases
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            ctx := context.Background()
            dep1 := mocks.NewMockDep1(t)
            dep2 := mocks.NewMockDep2(t)

            // Arrange
            tc.arrange(ctx, dep1, dep2)

            // Act
            sut := yourpkg.New(dep1, dep2)
            got, err := sut.DoSomething(ctx, tc.input1, tc.input2)

            // Assert
            tc.assert(t, got, err, dep2)
        })
    }
}
```

## Naming & Organization
- Test files end with `_test.go` and live next to the code under test.
- Prefer a single top‑level test per primary behavior. Use clear case names describing behavior
  (e.g., "repo error", "invalid match id", "dry‑run returns count").
- Keep helpers small and pure; mark helpers with `t.Helper()`.

## Assertions & Errors
- Use `require.NoError`, `require.Error`, and `require.ErrorContains` for clarity.
- Assert return values and mock call counts whenever side‑effects matter.

## Concurrency & Isolation
- Use `t.Parallel()` at the start of the test function.
- Within subtests, add `t.Parallel()` only if mocks/fixtures are fully isolated and no global state is mutated.

## Verification Commands
- Run all Go tests with race detector and coverage:
  - `cd go-app && go test -race -cover ./...`
- Run focused package tests while refactoring:
  - `cd go-app && go test -race -cover ./internal/services/...`
- Optional local coverage report:
  - `cd go-app && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`

## Reference
- Canonical example: `go-app/internal/services/weatherimport/service_test.go`.
