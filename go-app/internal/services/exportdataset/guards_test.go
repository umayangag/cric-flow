package exportdataset_test

import (
	"bytes"
	"context"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

type assertFnG func(t *testing.T, err error)

//nolint:unparam // sub is kept for future diverse cases even if tests pass same value now
func assertErrContainsG(sub string) assertFnG {
	return func(t *testing.T, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		require.NotEqual(t, nil || indexOfG(s, sub) < 0, err)
	}
}

func indexOfG(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func TestServices_Guards(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		act    func() error
		assert assertFnG
	}{
		{
			name:   "batting nil receiver",
			act:    func() error { var s *svc.BattingService; return s.ExportUnified(context.Background(), &bytes.Buffer{}) },
			assert: assertErrContainsG("nil service or repo"),
		},
		{
			name: "batting nil repo",
			act: func() error {
				s := svc.NewBattingService(nil)
				return s.ExportLegacy(context.Background(), &bytes.Buffer{})
			},
			assert: assertErrContainsG("nil service or repo"),
		},
		{
			name:   "bowling nil receiver",
			act:    func() error { var s *svc.BowlingService; return s.ExportUnified(context.Background(), &bytes.Buffer{}) },
			assert: assertErrContainsG("nil service or repo"),
		},
		{
			name: "bowling nil repo",
			act: func() error {
				s := svc.NewBowlingService(nil)
				return s.ExportLegacy(context.Background(), &bytes.Buffer{})
			},
			assert: assertErrContainsG("nil service or repo"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			err := tc.act()
			tc.assert(t, err)
		})
	}
}
