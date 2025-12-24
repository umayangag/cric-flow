// revive:disable:var-naming — package name "api" is intentional and conventional here.
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetMatchSquadsHandler_Validation_InvalidID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/match/abc/squads?asof=2023-01-05", nil)
	rr := httptest.NewRecorder()
	getMatchSquadsHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestGetMatchSquadsHandler_Validation_MissingAsOf(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/match/123/squads", nil)
	rr := httptest.NewRecorder()
	getMatchSquadsHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestGetMatchSquadsHandler_Validation_InvalidAsOf(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/match/123/squads?asof=2023/01/05", nil)
	rr := httptest.NewRecorder()
	getMatchSquadsHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestGetMatchSquadsHandler_NotFound(t *testing.T) {
	// stub seam
	orig := getMatchSquadsFunc
	defer func() { getMatchSquadsFunc = orig }()
	getMatchSquadsFunc = func(_ int64, _ time.Time, _ string) (matchSquadsData, error) {
		return matchSquadsData{}, errMatchNotFound
	}

	req := httptest.NewRequest(http.MethodGet, "/match/999/squads?asof=2023-01-05", nil)
	rr := httptest.NewRecorder()
	getMatchSquadsHandler(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGetMatchSquadsHandler_Incomplete(t *testing.T) {
	// stub seam
	orig := getMatchSquadsFunc
	defer func() { getMatchSquadsFunc = orig }()
	getMatchSquadsFunc = func(_ int64, _ time.Time, _ string) (matchSquadsData, error) {
		return matchSquadsData{}, errIncompleteSquad
	}

	req := httptest.NewRequest(http.MethodGet, "/match/123/squads?asof=2023-01-05", nil)
	rr := httptest.NewRecorder()
	getMatchSquadsHandler(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rr.Code)
	}
}

func TestGetMatchSquadsHandler_Success(t *testing.T) {
	// stub seam returning a simple structure
	orig := getMatchSquadsFunc
	defer func() { getMatchSquadsFunc = orig }()
	getMatchSquadsFunc = func(_ int64, _ time.Time, _ string) (matchSquadsData, error) {
		d, _ := time.Parse("2006-01-02", "2023-01-07")
		return matchSquadsData{
			MatchID: 123,
			Date:    d,
			Teams:   [2]string{"Team A", "Team B"},
			Squads: [2]squadDTO{
				{
					TeamName:  "Team A",
					ActualWin: 1,
					Players:   []playerPredictionDTO{{PlayerName: "A1"}},
				},
				{
					TeamName:  "Team B",
					ActualWin: 0,
					Players:   []playerPredictionDTO{{PlayerName: "B1"}},
				},
			},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/match/123/squads?asof=2023-01-05", nil)
	rr := httptest.NewRecorder()
	getMatchSquadsHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// minimal content checks
	body := rr.Body.String()
	if !contains(body, `"match_id": 123`) || !contains(body, `"date": "2023-01-07"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

// small helper to avoid importing strings package all over
func contains(s, substr string) bool { return len(s) >= len(substr) && stringIndex(s, substr) >= 0 }

// very small naive index to keep dependencies minimal in this file
func stringIndex(s, sep string) int {
	n := len(sep)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == sep {
			return i
		}
	}
	return -1
}
