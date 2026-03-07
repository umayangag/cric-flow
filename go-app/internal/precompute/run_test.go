package precompute

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/resources"
	pfcmd "github.com/umayangag/cric-flow/go-app/internal/services/precomputefeatures"
)

// stubRunner implements replayRunner for tests.
type stubRunner struct {
	err error
	// calls records (code, formatID) for each RunReplay invocation.
	calls []stubRunnerCall
}

type stubRunnerCall struct {
	Code     string
	FormatID int64
}

func (s *stubRunner) RunReplay(_ context.Context, code string, formatID int64, _ float64, _, _, _ int) error {
	s.calls = append(s.calls, stubRunnerCall{Code: code, FormatID: formatID})
	return s.err
}

func (s *stubRunner) RunReplayGlobalPool(_ context.Context, jobs []pfcmd.FormatJob, _, _ int) error {
	for _, j := range jobs {
		s.calls = append(s.calls, stubRunnerCall{Code: j.Code, FormatID: j.FormatID})
	}
	return s.err
}

// setupRunSeams replaces package-level seams and restores them on cleanup.
func setupRunSeams(t *testing.T, runner *stubRunner, formatIDs map[string]int64, formatIDErr error) {
	t.Helper()

	origLoadConfig := loadConfig
	origGetFormatID := getFormatIDByCode
	origGetLimit := getResourceLimit
	origNewRunner := newRunner

	loadConfig = func() *config.Config { return &config.Config{} }
	getFormatIDByCode = func(_ context.Context, code string) (int64, error) {
		if formatIDErr != nil {
			return 0, formatIDErr
		}
		id, ok := formatIDs[code]
		if !ok {
			return 0, errors.New("unknown format: " + code)
		}
		return id, nil
	}
	getResourceLimit = func(_ resources.Kind) int { return 2 }
	newRunner = func() replayRunner { return runner }

	t.Cleanup(func() {
		loadConfig = origLoadConfig
		getFormatIDByCode = origGetFormatID
		getResourceLimit = origGetLimit
		newRunner = origNewRunner
	})
}

// setupMockDB sets up a mock DB so db.Available() returns true and db.Pool is non-nil.
func setupMockDB(t *testing.T) {
	t.Helper()
	mockDB := &mocks.MockDB{}
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })
}

func TestRun(t *testing.T) {
	tests := []struct {
		name        string
		formats     []string
		opts        *RunOpts
		formatIDs   map[string]int64
		formatIDErr error
		runnerErr   error
		wantErr     bool
		wantErrMsg  string
		wantCalls   int
		wantStatus  string // expected phase after Run
	}{
		{
			name:       "happy_path_single_format",
			formats:    []string{"T20I"},
			opts:       nil,
			formatIDs:  map[string]int64{"T20I": 3},
			wantErr:    false,
			wantCalls:  1,
			wantStatus: "done",
		},
		{
			name:       "happy_path_multiple_formats",
			formats:    []string{"TEST", "ODI"},
			opts:       &RunOpts{Alpha: 0.5, LastN: 20},
			formatIDs:  map[string]int64{"TEST": 1, "ODI": 2},
			wantErr:    false,
			wantCalls:  2,
			wantStatus: "done",
		},
		{
			name:       "nil_formats_with_no_db_formats_returns_nil",
			formats:    []string{"NONE"},
			formatIDs:  map[string]int64{"NONE": 99},
			wantErr:    false,
			wantCalls:  1,
			wantStatus: "done",
		},
		{
			name:        "format_id_lookup_fails",
			formats:     []string{"T20I"},
			formatIDErr: errors.New("db error"),
			wantErr:     true,
			wantErrMsg:  "db error",
			wantCalls:   0,
			wantStatus:  "done",
		},
		{
			name:       "runner_fails",
			formats:    []string{"T20I"},
			formatIDs:  map[string]int64{"T20I": 3},
			runnerErr:  errors.New("replay failed"),
			wantErr:    true,
			wantErrMsg: "replay failed",
			wantCalls:  1,
			wantStatus: "done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetState(t)
			setupMockDB(t)

			runner := &stubRunner{err: tt.runnerErr}
			setupRunSeams(t, runner, tt.formatIDs, tt.formatIDErr)

			err := Run(context.Background(), "2024", tt.formats, tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}

			assert.Len(t, runner.calls, tt.wantCalls)

			st := GetStatus()
			if tt.wantStatus != "" {
				assert.Equal(t, tt.wantStatus, st.Phase)
			}
		})
	}
}

func TestRun_DBConnectFails_WhenPoolNil(t *testing.T) {
	resetState(t)
	// Ensure db.Pool is nil by clearing any mock.
	db.SetDB(nil)
	// Don't set db.Pool — it should be nil.
	origPool := db.Pool
	db.Pool = nil
	t.Cleanup(func() { db.Pool = origPool })

	origConnect := connectDB
	connectDB = func(_ context.Context) (*pgxpool.Pool, error) {
		return nil, errors.New("connection refused")
	}
	t.Cleanup(func() { connectDB = origConnect })

	err := Run(context.Background(), "2024", []string{"T20I"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestRun_OptsOverrideConfig(t *testing.T) {
	resetState(t)
	setupMockDB(t)

	runner := &stubRunner{}
	setupRunSeams(t, runner, map[string]int64{"T20I": 3}, nil)

	// Override alpha and lastN via opts.
	opts := &RunOpts{Alpha: 0.7, LastN: 15}
	err := Run(context.Background(), "2024", []string{"T20I"}, opts)
	require.NoError(t, err)
	assert.Len(t, runner.calls, 1)
}

func TestRun_StatusRecordsErrorOnFailure(t *testing.T) {
	resetState(t)
	setupMockDB(t)

	runner := &stubRunner{err: errors.New("boom")}
	setupRunSeams(t, runner, map[string]int64{"T20I": 3}, nil)

	err := Run(context.Background(), "2024", []string{"T20I"}, nil)
	require.Error(t, err)

	st := GetStatus()
	assert.Equal(t, "boom", st.LastError)
	assert.False(t, st.Running)
	assert.Equal(t, "done", st.Phase)
}
