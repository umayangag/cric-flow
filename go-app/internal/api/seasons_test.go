// revive:disable:var-naming — package name "api" is intentional and conventional here.
package api

import (
    "context"
    "database/sql"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestGetNextSeasonHandler_Validation(t *testing.T) {
    // Missing cutoff
    req := httptest.NewRequest(http.MethodGet, "/seasons/next", nil)
    rr := httptest.NewRecorder()
    getNextSeasonHandler(rr, req)
    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400, got %d", rr.Code)
    }

    // Invalid cutoff
    req = httptest.NewRequest(http.MethodGet, "/seasons/next?cutoff=2022/12/31", nil)
    rr = httptest.NewRecorder()
    getNextSeasonHandler(rr, req)
    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400, got %d", rr.Code)
    }
}

func TestGetNextSeasonHandler_SuccessAndNull(t *testing.T) {
    // Stub the DAO seam
    orig := getNextSeasonFunc
    defer func() { getNextSeasonFunc = orig }()

    calls := 0
    getNextSeasonFunc = func(_ context.Context, _ time.Time, _ string) (sql.NullInt64, error) {
        calls++
        if calls == 1 {
            // return a season
            return sql.NullInt64{Int64: 2023, Valid: true}, nil
        }
        // return null
        return sql.NullInt64{Valid: false}, nil
    }

    // With format param and valid cutoff
    req := httptest.NewRequest(http.MethodGet, "/seasons/next?cutoff=2022-12-31&format=T20", nil)
    rr := httptest.NewRecorder()
    getNextSeasonHandler(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rr.Code)
    }
    var resp struct{ NextSeason *int `json:"next_season"` }
    if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
        t.Fatalf("invalid json: %v", err)
    }
    if resp.NextSeason == nil || *resp.NextSeason != 2023 {
        t.Fatalf("expected next_season 2023, got %#v", resp.NextSeason)
    }

    // Second call should return null
    req = httptest.NewRequest(http.MethodGet, "/seasons/next?cutoff=2024-01-01", nil)
    rr = httptest.NewRecorder()
    getNextSeasonHandler(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rr.Code)
    }
    resp = struct{ NextSeason *int `json:"next_season"` }{}
    if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
        t.Fatalf("invalid json: %v", err)
    }
    if resp.NextSeason != nil {
        t.Fatalf("expected next_season null, got %#v", *resp.NextSeason)
    }
}
