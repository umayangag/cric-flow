package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

type fakeDBProbe struct {
	pingErr     error
	counts      map[string]int64
	migCurrent  int
	migExpected int
	migStatus   string
	migErr      error
	tableStats  []db.TableStat
}

func (f fakeDBProbe) Ping(_ context.Context) error { return f.pingErr }
func (f fakeDBProbe) Count(_ context.Context, table string) (int64, error) {
	return f.counts[table], nil
}
func (f fakeDBProbe) CountFieldingByFormat(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (f fakeDBProbe) CountFieldingByFormatGrouped(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (f fakeDBProbe) MigrationInfo(_ context.Context) (int, int, string, error) {
	return f.migCurrent, f.migExpected, f.migStatus, f.migErr
}
func (f fakeDBProbe) LastMatchImportAt(_ context.Context) (time.Time, error) { return time.Time{}, nil }
func (f fakeDBProbe) TableStats(_ context.Context) ([]db.TableStat, error) {
	return f.tableStats, nil
}

func TestOpsStatusHandler_DBProbeMapping(t *testing.T) {
	tests := []struct {
		name       string
		probe      fakeDBProbe
		wantConn   bool
		wantStatus string
		wantCounts map[string]int64
	}{
		{
			name:       "disconnected_unknown_migration",
			probe:      fakeDBProbe{pingErr: assertErr{}, migExpected: 10, migStatus: "unknown"},
			wantConn:   false,
			wantStatus: "unknown",
			wantCounts: nil,
		},
		{
			name: "connected_out_of_date",
			probe: fakeDBProbe{
				counts:      map[string]int64{"players": 5, "matches": 7},
				migCurrent:  5,
				migExpected: 10,
				migStatus:   "out_of_date",
			},
			wantConn:   true,
			wantStatus: "out_of_date",
			wantCounts: map[string]int64{"players": 5, "matches": 7},
		},
		{
			name: "connected_ok",
			probe: fakeDBProbe{
				counts:      map[string]int64{"players": 1, "matches": 2},
				migCurrent:  10,
				migExpected: 10,
				migStatus:   "ok",
			},
			wantConn:   true,
			wantStatus: "ok",
			wantCounts: map[string]int64{"players": 1, "matches": 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{mlClient: nil, dbProbe: tc.probe}
			req := httptest.NewRequest(http.MethodGet, "/ops/status", nil)
			rr := httptest.NewRecorder()
			app.opsStatusHandler(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rr.Code)
			}
			// decode as generic map to inspect fields easily
			var m map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
			dbAny := m["db"].(map[string]any)
			if got := dbAny["connected"].(bool); got != tc.wantConn {
				t.Fatalf("connected mismatch: got %v want %v", got, tc.wantConn)
			}
			if migAny, ok := dbAny["migration"].(map[string]any); ok {
				if status, _ := migAny["status"].(string); status != tc.wantStatus {
					t.Fatalf("migration status mismatch: got %q want %q", status, tc.wantStatus)
				}
			} else {
				t.Fatalf("missing migration field")
			}
			if tc.wantConn && tc.wantCounts != nil {
				countsAny, ok := dbAny["counts"].(map[string]any)
				if !ok {
					t.Fatalf("missing counts")
				}
				for k, v := range tc.wantCounts {
					if got, ok := countsAny[k].(float64); !ok || int64(got) != v {
						t.Fatalf("count %s mismatch: got %v want %d", k, countsAny[k], v)
					}
				}
			}
		})
	}
}

// assertErr is a sentinel error for ping failure in tests.
type assertErr struct{}

func (assertErr) Error() string { return "ping failed" }
