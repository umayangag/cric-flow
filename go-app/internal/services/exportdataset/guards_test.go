package exportdataset_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

func TestServices_Guards(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		act     func() error
		wantErr string
	}{
		{
			name:    "batting nil receiver",
			act:     func() error { var s *svc.BattingService; return s.ExportUnified(context.Background(), &bytes.Buffer{}) },
			wantErr: "nil service or repo",
		},
		{
			name: "batting nil repo",
			act: func() error {
				s := svc.NewBattingService(nil)
				return s.ExportUnified(context.Background(), &bytes.Buffer{})
			},
			wantErr: "nil service or repo",
		},
		{
			name:    "bowling nil receiver",
			act:     func() error { var s *svc.BowlingService; return s.ExportUnified(context.Background(), &bytes.Buffer{}) },
			wantErr: "nil service or repo",
		},
		{
			name: "bowling nil repo",
			act: func() error {
				s := svc.NewBowlingService(nil)
				return s.ExportUnified(context.Background(), &bytes.Buffer{})
			},
			wantErr: "nil service or repo",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.act()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
