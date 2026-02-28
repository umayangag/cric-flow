package precompute

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
)

// stringRows implements db.Rows for tests (yields format codes from a slice).
type stringRows struct {
	vals []string
	idx  int
}

func (r *stringRows) Next() bool {
	if r.idx >= len(r.vals) {
		return false
	}
	r.idx++
	return true
}

func (r *stringRows) Scan(dest ...any) error {
	if len(dest) < 1 {
		return nil
	}
	if p, ok := dest[0].(*string); ok {
		*p = r.vals[r.idx-1]
		return nil
	}
	return nil
}

func (r *stringRows) Close() {}

func (r *stringRows) Err() error { return nil }

func setupPrecomputeDB(t *testing.T, mockDB *mocks.MockDB) {
	t.Helper()
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })
}

func TestGetStatus_ReturnsSnapshot(t *testing.T) {
	// GetStatus returns a copy of current status; no panic.
	st := GetStatus()
	_ = st.Running
	_ = st.Season
	_ = st.Formats
	_ = st.Phase
	_ = st.LastError
}

func TestDiscoverFormatCodes(t *testing.T) {
	// Do not use t.Parallel(); empty-provided case uses db.SetDB (global).

	cases := []struct {
		name     string
		setup    func(*mocks.MockDB)
		provided []string
		want     []string
		wantErr  bool
	}{
		{
			name:     "non_empty_returns_provided",
			setup:    nil,
			provided: []string{"T20I", "ODI", "TEST"},
			want:     []string{"T20I", "ODI", "TEST"},
			wantErr:  false,
		},
		{
			name: "empty_uses_db_returns_codes",
			setup: func(m *mocks.MockDB) {
				setupPrecomputeDB(t, m)
				m.On("Query", mock.Anything, mock.Anything).
					Return(&stringRows{vals: []string{"TEST", "ODI", "T20"}}, nil)
			},
			provided: nil,
			want:     []string{"TEST", "ODI", "T20"},
			wantErr:  false,
		},
		{
			name: "empty_single_code",
			setup: func(m *mocks.MockDB) {
				setupPrecomputeDB(t, m)
				m.On("Query", mock.Anything, mock.Anything).Return(&stringRows{vals: []string{"T20I"}}, nil)
			},
			provided: nil,
			want:     []string{"T20I"},
			wantErr:  false,
		},
		{
			name:     "empty_no_db_returns_err",
			setup:    func(_ *mocks.MockDB) { db.SetDB(nil); t.Cleanup(func() {}) },
			provided: nil,
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				mockDB := &mocks.MockDB{}
				tc.setup(mockDB)
			}
			got, err := discoverFormatCodes(context.Background(), tc.provided)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
