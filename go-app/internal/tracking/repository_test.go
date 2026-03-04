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

func TestInProgressByCommand(t *testing.T) {
	t.Run("db_not_available_returns_empty_map", func(t *testing.T) {
		db.SetDB(nil)
		defer func() { db.SetDB(nil) }()
		got, err := InProgressByCommand(context.Background())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})
}

func TestCancelStaleInProgressMigrations(t *testing.T) {
	// Do not use t.Parallel(); uses db.SetDB (global).

	t.Run("db_not_available_or_too_short_returns_zero", func(t *testing.T) {
		db.SetDB(nil)
		defer func() { db.SetDB(nil) }()
		n, err := CancelStaleInProgressMigrations(context.Background(), "reason", 500*time.Millisecond)
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})

	t.Run("happy_path_returns_updated_count", func(t *testing.T) {
		m := &mocks.MockDB{}
		setupDB(t, m)
		// Expect a single QueryRow call and scan an integer count.
		m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(scanIntRow(3))

		n, err := CancelStaleInProgressMigrations(context.Background(), "reason", 2*time.Hour)
		require.NoError(t, err)
		require.Equal(t, 3, n)
		m.AssertExpectations(t)
	})
}

func TestGetInProgressMigrationIDForCommand(t *testing.T) {
	// Do not use t.Parallel(); uses db.SetDB (global).

	t.Run("db_not_available_or_empty_command_returns_zero", func(t *testing.T) {
		db.SetDB(nil)
		defer func() { db.SetDB(nil) }()
		id, err := GetInProgressMigrationIDForCommand(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, 0, id)
	})

	t.Run("no_rows_returns_zero", func(t *testing.T) {
		m := &mocks.MockDB{}
		setupDB(t, m)
		m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
			Return(scanErrRow{err: sql.ErrNoRows})

		id, err := GetInProgressMigrationIDForCommand(context.Background(), "precompute")
		require.NoError(t, err)
		require.Equal(t, 0, id)
		m.AssertExpectations(t)
	})

	t.Run("happy_path_returns_id", func(t *testing.T) {
		m := &mocks.MockDB{}
		setupDB(t, m)
		m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
			Return(scanIntRow(42))

		id, err := GetInProgressMigrationIDForCommand(context.Background(), "precompute")
		require.NoError(t, err)
		require.Equal(t, 42, id)
		m.AssertExpectations(t)
	})

	t.Run("query_error_propagated", func(t *testing.T) {
		m := &mocks.MockDB{}
		setupDB(t, m)
		m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
			Return(scanErrRow{err: errors.New("db error")})

		id, err := GetInProgressMigrationIDForCommand(context.Background(), "precompute")
		require.Error(t, err)
		require.Equal(t, 0, id)
		m.AssertExpectations(t)
	})
}

// stubRows implements db.Rows for testing scanMigrations and repository helpers.
type stubRows struct {
	migrations []Migration
	index      int
	err        error
}

func (s *stubRows) Next() bool {
	if s.index < len(s.migrations) {
		s.index++
		return true
	}
	return false
}

func (s *stubRows) Scan(dest ...any) error {
	if s.index == 0 || s.index > len(s.migrations) {
		return errors.New("scan called with no current row")
	}
	m := s.migrations[s.index-1]

	if len(dest) != 8 {
		return errors.New("unexpected dest len")
	}

	if p, ok := dest[0].(*int); ok {
		*p = m.ID
	}
	if p, ok := dest[1].(*string); ok {
		*p = m.Command
	}
	if p, ok := dest[2].(*[]byte); ok {
		*p = []byte(m.Args)
	}
	if p, ok := dest[3].(*time.Time); ok {
		*p = m.StartedAt
	}
	if pp, ok := dest[4].(**time.Time); ok {
		*pp = m.CompletedAt
	}
	if p, ok := dest[5].(*MigrationStatus); ok {
		*p = m.Status
	}
	if p, ok := dest[6].(*[]byte); ok {
		*p = []byte(m.Metadata)
	}
	if pp, ok := dest[7].(**string); ok {
		if m.ErrorMessage == "" {
			*pp = nil
		} else {
			msg := m.ErrorMessage
			*pp = &msg
		}
	}
	return nil
}

func (s *stubRows) Close() {}

func (s *stubRows) Err() error { return s.err }

func TestScanMigrations(t *testing.T) {
	now := time.Now().UTC()
	migs := []Migration{
		{
			ID:          1,
			Command:     "precompute",
			Args:        []byte(`{"k":"v"}`),
			StartedAt:   now,
			CompletedAt: &now,
			Status:      StatusCompleted,
			Metadata:    []byte(`{"meta":true}`),
		},
		{
			ID:        2,
			Command:   "export-dataset",
			Args:      []byte(`{}`),
			StartedAt: now.Add(time.Minute),
			Status:    StatusInProgress,
		},
	}

	rows := &stubRows{migrations: migs}

	got, err := scanMigrations(rows)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, migs[0].ID, got[0].ID)
	assert.Equal(t, migs[0].Command, got[0].Command)
	assert.Equal(t, migs[0].Status, got[0].Status)
	assert.Equal(t, migs[0].CompletedAt, got[0].CompletedAt)

	assert.Equal(t, migs[1].ID, got[1].ID)
	assert.Equal(t, migs[1].Command, got[1].Command)
	assert.Equal(t, migs[1].Status, got[1].Status)
}

func TestScanMigrations_ErrPropagation(t *testing.T) {
	rows := &stubRows{err: errors.New("rows error")}

	got, err := scanMigrations(rows)
	require.Error(t, err)
	require.Nil(t, got)
}

func TestGetInProgressMigrations_DBUnavailable(t *testing.T) {
	db.SetDB(nil)
	t.Cleanup(func() { db.SetDB(nil) })

	got, err := GetInProgressMigrations(context.Background())
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestGetInProgressMigrations_HappyPath(t *testing.T) {
	m := &mocks.MockDB{}
	setupDB(t, m)

	migs := []Migration{
		{ID: 1, Command: "precompute"},
		{ID: 2, Command: "export-dataset"},
	}
	rows := &stubRows{migrations: migs}

	m.On("Query", mock.Anything, mock.Anything, mock.Anything).
		Return(rows, nil)

	got, err := GetInProgressMigrations(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, migs[0].ID, got[0].ID)
	assert.Equal(t, migs[1].ID, got[1].ID)
	m.AssertExpectations(t)
}

func TestGetRecentMigrations_DBUnavailable(t *testing.T) {
	db.SetDB(nil)
	t.Cleanup(func() { db.SetDB(nil) })

	got, err := GetRecentMigrations(context.Background(), 5)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestGetRecentMigrations_HappyPath(t *testing.T) {
	m := &mocks.MockDB{}
	setupDB(t, m)

	migs := []Migration{
		{ID: 1, Command: "precompute"},
		{ID: 2, Command: "export-dataset"},
	}
	rows := &stubRows{migrations: migs}

	m.On("Query", mock.Anything, mock.Anything, mock.Anything).
		Return(rows, nil)

	got, err := GetRecentMigrations(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, migs[0].ID, got[0].ID)
	assert.Equal(t, migs[1].ID, got[1].ID)
	m.AssertExpectations(t)
}

func TestGetMigrationsPaginated_DBUnavailable(t *testing.T) {
	db.SetDB(nil)
	t.Cleanup(func() { db.SetDB(nil) })

	got, total, err := GetMigrationsPaginated(context.Background(), 10, 0)
	require.NoError(t, err)
	require.Nil(t, got)
	require.Equal(t, 0, total)
}

func TestGetMigrationsPaginated_HappyPath(t *testing.T) {
	m := &mocks.MockDB{}
	setupDB(t, m)

	migs := []Migration{
		{ID: 1, Command: "precompute"},
		{ID: 2, Command: "export-dataset"},
	}
	rows := &stubRows{migrations: migs}

	// COUNT(*) total
	m.On("QueryRow", mock.Anything, mock.Anything).
		Return(scanIntRow(len(migs)))

	// page rows
	m.On("Query", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(rows, nil)

	got, total, err := GetMigrationsPaginated(context.Background(), 10, 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, len(migs), total)
	assert.Equal(t, migs[0].ID, got[0].ID)
	assert.Equal(t, migs[1].ID, got[1].ID)
	m.AssertExpectations(t)
}
