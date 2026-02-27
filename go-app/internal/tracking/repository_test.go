package tracking

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
)

// row types for table-driven tests: implement db.Row so mock QueryRow can return them.
type scanBoolRow bool

func (r scanBoolRow) Scan(dest ...any) error {
	if len(dest) < 1 {
		return nil
	}
	if p, ok := dest[0].(*bool); ok {
		*p = bool(r)
		return nil
	}
	return nil
}

type scanTimeRow time.Time

func (r scanTimeRow) Scan(dest ...any) error {
	if len(dest) < 1 {
		return nil
	}
	t := time.Time(r)
	// Scan(&completedAt) passes **time.Time so driver can set the pointer.
	if pp, ok := dest[0].(**time.Time); ok {
		*pp = &t
		return nil
	}
	if p, ok := dest[0].(*time.Time); ok {
		*p = t
		return nil
	}
	return nil
}

type scanErrRow struct{ err error }

func (r scanErrRow) Scan(_ ...any) error { return r.err }

func setupDB(t *testing.T, mock *mocks.MockDB) {
	t.Helper()
	db.SetDB(mock)
	t.Cleanup(func() { db.SetDB(nil) })
}

func TestHasInProgressForCommand(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global) and would race.

	cases := []struct {
		name    string
		setup   func(*mocks.MockDB)
		command string
		want    bool
		wantErr bool
	}{
		{
			name:    "db_not_available_returns_false",
			setup:   func(*mocks.MockDB) { db.SetDB(nil) },
			command: "precompute-features",
			want:    false,
			wantErr: false,
		},
		{
			name: "exists_true",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(true))
			},
			command: "precompute-features",
			want:    true,
			wantErr: false,
		},
		{
			name: "exists_false",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(false))
			},
			command: "export-dataset",
			want:    false,
			wantErr: false,
		},
		{
			name: "query_error",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanErrRow{err: errors.New("db error")})
			},
			command: "x",
			want:    false,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.MockDB{}
			if tc.setup != nil {
				tc.setup(m)
			}
			got, err := HasInProgressForCommand(context.Background(), tc.command)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			m.AssertExpectations(t)
		})
	}
}

func TestHasCompletedSuccessfullyForCommand(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	cases := []struct {
		name    string
		setup   func(*mocks.MockDB)
		command string
		want    bool
		wantErr bool
	}{
		{
			name:    "db_not_available",
			setup:   func(*mocks.MockDB) { db.SetDB(nil) },
			command: "precompute-features",
			want:    false,
			wantErr: false,
		},
		{
			name:    "empty_command",
			setup:   func(m *mocks.MockDB) { setupDB(t, m) },
			command: "",
			want:    false,
			wantErr: false,
		},
		{
			name: "completed_exists",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(true))
			},
			command: "precompute-features",
			want:    true,
			wantErr: false,
		},
		{
			name: "no_completed",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(false))
			},
			command: "export-dataset",
			want:    false,
			wantErr: false,
		},
		{
			name: "query_error",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanErrRow{err: errors.New("db error")})
			},
			command: "x",
			want:    false,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.MockDB{}
			if tc.setup != nil {
				tc.setup(m)
			}
			got, err := HasCompletedSuccessfullyForCommand(context.Background(), tc.command)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			m.AssertExpectations(t)
		})
	}
}

func TestGetLastCompletedAtForCommand(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	now := time.Now().UTC()

	cases := []struct {
		name     string
		setup    func(*mocks.MockDB)
		command  string
		wantNil  bool
		wantErr  bool
		wantTime *time.Time
	}{
		{
			name:    "db_not_available",
			setup:   func(*mocks.MockDB) { db.SetDB(nil) },
			command: "precompute-features",
			wantNil: true,
			wantErr: false,
		},
		{
			name:    "empty_command",
			setup:   func(m *mocks.MockDB) { setupDB(t, m) },
			command: "",
			wantNil: true,
			wantErr: false,
		},
		{
			name: "no_rows_returns_nil_nil",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanErrRow{err: sql.ErrNoRows})
			},
			command: "x",
			wantNil: true,
			wantErr: false,
		},
		{
			name: "returns_completed_at",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanTimeRow(now))
			},
			command:  "precompute-features",
			wantNil:  false,
			wantErr:  false,
			wantTime: &now,
		},
		{
			name: "query_error",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanErrRow{err: errors.New("db error")})
			},
			command: "x",
			wantNil: true,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.MockDB{}
			if tc.setup != nil {
				tc.setup(m)
			}
			got, err := GetLastCompletedAtForCommand(context.Background(), tc.command)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantNil, got == nil)
			if tc.wantTime != nil && got != nil {
				assert.True(t, got.Equal(*tc.wantTime), "got %v want %v", *got, *tc.wantTime)
			}
			m.AssertExpectations(t)
		})
	}
}

func TestHasInProgressForAnyCommand(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	commands := []string{"import", "export"}

	cases := []struct {
		name     string
		setup    func(*mocks.MockDB)
		commands []string
		want     bool
		wantErr  bool
	}{
		{
			name:     "db_not_available",
			setup:    func(*mocks.MockDB) { db.SetDB(nil) },
			commands: commands,
			want:     false,
			wantErr:  false,
		},
		{
			name:     "empty_commands",
			setup:    func(m *mocks.MockDB) { setupDB(t, m) },
			commands: nil,
			want:     false,
			wantErr:  false,
		},
		{
			name: "exists_true",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(true))
			},
			commands: commands,
			want:     true,
			wantErr:  false,
		},
		{
			name: "exists_false",
			setup: func(m *mocks.MockDB) {
				setupDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(false))
			},
			commands: commands,
			want:     false,
			wantErr:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.MockDB{}
			if tc.setup != nil {
				tc.setup(m)
			}
			got, err := HasInProgressForAnyCommand(context.Background(), tc.commands)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			m.AssertExpectations(t)
		})
	}
}

// mockAnyContext and mockAnyString for On() matchers.
func mockAnyContext() interface{} { return mock.MatchedBy(func(context.Context) bool { return true }) }
func mockAnyString() interface{}  { return mock.MatchedBy(func(string) bool { return true }) }
