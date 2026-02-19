package migrate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	cmd "github.com/umayangag/cric-flow/go-app/internal/commands/migrate"
)

type assertErrFn func(t *testing.T, err error)

func assertNoErr() assertErrFn {
	return func(t *testing.T, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	}
}

func assertErr() assertErrFn {
	return func(t *testing.T, err error) {
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	}
}

func TestRunner_Run(t *testing.T) {
	t.Parallel()
	cases := []struct {
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
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := cmd.Runner{Migrate: tc.migrate, Timeout: tc.timeout}
			err := r.Run(context.Background(), "migrations")
			tc.assert(t, err)
		})
	}
}
