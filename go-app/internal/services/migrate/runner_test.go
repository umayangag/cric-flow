package migrate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/migrate"

	"github.com/stretchr/testify/require"
)

type assertErrFn func(t *testing.T, err error)

func assertNoErr() assertErrFn {
	return func(t *testing.T, err error) {
		t.Helper()
		require.NoError(t, err)
	}
}

func assertErr() assertErrFn {
	return func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
	}
}

func TestRunner_Run(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		migrate func(ctx context.Context, dir string) error
		timeout time.Duration
		assert  assertErrFn
	}{
		{
			name:    "success path",
			migrate: func(_ context.Context, _ string) error { return nil },
			timeout: 10 * time.Millisecond,
			assert:  assertNoErr(),
		},
		{
			name:    "error propagates",
			migrate: func(_ context.Context, _ string) error { return errors.New("boom") },
			timeout: 10 * time.Millisecond,
			assert:  assertErr(),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			r := svc.Runner{Migrate: tc.migrate, Timeout: tc.timeout}
			err := r.Run(context.Background(), "migrations")
			tc.assert(t, err)
		})
	}
}
