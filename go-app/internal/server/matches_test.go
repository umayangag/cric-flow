package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func TestListMatchesHandler_Validation_MissingSeason(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/matches?after=2022-12-31", nil)
	rr := httptest.NewRecorder()
	listMatchesHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListMatchesHandler_Validation_InvalidSeason(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/matches?season=20A2&after=2022-12-31", nil)
	rr := httptest.NewRecorder()
	listMatchesHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListMatchesHandler_Validation_MissingAfter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/matches?season=2023", nil)
	rr := httptest.NewRecorder()
	listMatchesHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListMatchesHandler_Validation_InvalidAfter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/matches?season=2023&after=2022/12/31", nil)
	rr := httptest.NewRecorder()
	listMatchesHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListMatchesHandler_EmptyResult(t *testing.T) {
	// stub seam
	orig := listMatchesFunc
	defer func() { listMatchesFunc = orig }()
	listMatchesFunc = func(_ context.Context, _ int, _ time.Time, _ string) ([]db.MatchRow, error) {
		return []db.MatchRow{}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/matches?season=2023&after=2022-12-31", nil)
	rr := httptest.NewRecorder()
	listMatchesHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp []matchItem
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty list, got %d", len(resp))
	}
}

func TestListMatchesHandler_Success(t *testing.T) {
	// stub seam returning two matches
	orig := listMatchesFunc
	defer func() { listMatchesFunc = orig }()
	listMatchesFunc = func(_ context.Context, season int, _ time.Time, _ string) ([]db.MatchRow, error) {
		if season != 2023 {
			t.Fatalf("expected season 2023, got %d", season)
		}
		// order already ASC by date; ensure handler preserves
		d1, _ := time.Parse("2006-01-02", "2023-01-02")
		d2, _ := time.Parse("2006-01-02", "2023-02-15")
		fmt1 := sql.NullString{String: "T20", Valid: true}
		fmt2 := sql.NullString{String: "", Valid: false}
		return []db.MatchRow{
			{MatchID: 111, Date: d1, FormatCode: fmt1, Teams: [2]string{"Team A", "Team B"}},
			{MatchID: 222, Date: d2, FormatCode: fmt2, Teams: [2]string{"Team C", "Team D"}},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/matches?season=2023&after=2022-12-31", nil)
	rr := httptest.NewRecorder()
	listMatchesHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp []matchItem
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp))
	}
	if resp[0].MatchID != 111 || resp[0].Date != "2023-01-02" || resp[0].Format == nil || *resp[0].Format != "T20" {
		t.Fatalf("unexpected first item: %#v", resp[0])
	}
	if resp[1].MatchID != 222 || resp[1].Date != "2023-02-15" || resp[1].Format != nil {
		t.Fatalf("unexpected second item: %#v", resp[1])
	}
	if len(resp[0].Teams) != 2 || len(resp[1].Teams) != 2 {
		t.Fatalf("teams must have length 2")
	}
}
