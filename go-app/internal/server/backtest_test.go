package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func TestBacktestMatchHandler_SelectMode_Success(t *testing.T) {
	// Backup and stub seam
	orig := listPlayedByFmtTeams
	defer func() { listPlayedByFmtTeams = orig }()

	listPlayedByFmtTeams = func(_ context.Context, _ string, _ string, _ string) ([]db.BacktestCandidate, error) {
		return []db.BacktestCandidate{
			{
				MatchID:    111,
				StableID:   sql.NullString{Valid: true, String: "2024-10-30-IND-AUS"},
				MatchDate:  time.Date(2024, 10, 30, 14, 0, 0, 0, time.UTC),
				Venue:      sql.NullString{Valid: true, String: "Wankhede Stadium"},
				Season:     sql.NullString{Valid: true, String: "2024"},
				FormatCode: sql.NullString{Valid: true, String: "T20"},
				Team1:      "IND",
				Team2:      "AUS",
				WinnerTeam: sql.NullString{Valid: true, String: "IND"},
			},
		}, nil
	}

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS", nil)

	app.backtestMatchHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var payload backtestSelectResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Filters["format"] != "T20" || payload.Filters["team1"] != "IND" || payload.Filters["team2"] != "AUS" {
		t.Fatalf("unexpected filters: %+v", payload.Filters)
	}
	if len(payload.Candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(payload.Candidates))
	}
	got := payload.Candidates[0]
	if got.MatchID != 111 || got.Team1 != "IND" || got.Team2 != "AUS" || got.WinnerTeamCode != "IND" {
		t.Fatalf("unexpected candidate: %+v", got)
	}
}

func TestBacktestMatchHandler_SelectMode_MissingParams(t *testing.T) {
	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND", nil)
	app.backtestMatchHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestBacktestMatchHandler_SelectMode_DBError(t *testing.T) {
	// Force the DAO seam to return an error so the handler responds 500
	orig := listPlayedByFmtTeams
	defer func() { listPlayedByFmtTeams = orig }()
	listPlayedByFmtTeams = func(_ context.Context, _ string, _ string, _ string) ([]db.BacktestCandidate, error) {
		return nil, errors.New("db failure")
	}

	app := NewApp(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS", nil)

	app.backtestMatchHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

// no extra helpers
