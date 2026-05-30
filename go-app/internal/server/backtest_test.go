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

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/stretchr/testify/require"
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

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var payload backtestSelectResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, "T20", payload.Filters["format"])
	require.Equal(t, "IND", payload.Filters["team1"])
	require.Equal(t, "AUS", payload.Filters["team2"])
	require.Len(t, payload.Candidates, 1)
	got := payload.Candidates[0]
	require.Equal(t, int64(111), got.MatchID)
	require.Equal(t, "IND", got.Team1)
	require.Equal(t, "AUS", got.Team2)
	require.Equal(t, "IND", got.WinnerTeamCode)
}

func TestBacktestMatchHandler_SelectMode_MissingParams(t *testing.T) {
	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND", nil)
	app.backtestMatchHandler(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestBacktestMatchHandler_SelectMode_DBError(t *testing.T) {
	// Force the DAO seam to return an error so the handler responds 500
	orig := listPlayedByFmtTeams
	defer func() { listPlayedByFmtTeams = orig }()
	listPlayedByFmtTeams = func(_ context.Context, _ string, _ string, _ string) ([]db.BacktestCandidate, error) {
		return nil, errors.New("db failure")
	}

	app := NewApp(context.Background(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/match?format=T20&team1=IND&team2=AUS", nil)

	app.backtestMatchHandler(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// no extra helpers
