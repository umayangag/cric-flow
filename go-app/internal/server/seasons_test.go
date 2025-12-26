package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGetNextSeasonHandler_Validation follows table-driven + AAA with require assertions.
func TestGetNextSeasonHandler_Validation(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "missing cutoff -> 400", url: "/seasons/next"},
		{name: "invalid cutoff format -> 400", url: "/seasons/next?cutoff=2022/12/31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()
			// Act
			getNextSeasonHandler(rr, req)
			// Assert
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

// TestGetNextSeasonHandler_Scenarios uses subtests; avoid t.Parallel due to global stub.
func TestGetNextSeasonHandler_Scenarios(t *testing.T) {
	type daoFn = func(ctx context.Context, cutoff time.Time, format string) (sql.NullInt64, error)

	// Preserve and restore the original seam
	orig := getNextSeasonFunc
	t.Cleanup(func() { getNextSeasonFunc = orig })

	cases := []struct {
		name       string
		stub       daoFn
		url        string
		expectCode int
		expectJSON string // for quick check of next_season value
		assertFn   func(t *testing.T, body []byte)
	}{
		{
			name: "returns season when DAO has value",
			stub: func(_ context.Context, _ time.Time, _ string) (sql.NullInt64, error) {
				return sql.NullInt64{Int64: 2023, Valid: true}, nil
			},
			url:        "/seasons/next?cutoff=2022-12-31&format=T20",
			expectCode: http.StatusOK,
			assertFn: func(t *testing.T, body []byte) {
				var resp struct {
					NextSeason *int `json:"next_season"`
				}
				require.NoError(t, json.Unmarshal(body, &resp))
				require.NotNil(t, resp.NextSeason)
				require.Equal(t, 2023, *resp.NextSeason)
			},
		},
		{
			name: "returns null when DAO has no value",
			stub: func(_ context.Context, _ time.Time, _ string) (sql.NullInt64, error) {
				return sql.NullInt64{Valid: false}, nil
			},
			url:        "/seasons/next?cutoff=2024-01-01",
			expectCode: http.StatusOK,
			assertFn: func(t *testing.T, body []byte) {
				var resp struct {
					NextSeason *int `json:"next_season"`
				}
				require.NoError(t, json.Unmarshal(body, &resp))
				require.Nil(t, resp.NextSeason)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			getNextSeasonFunc = tc.stub
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()
			// Act
			getNextSeasonHandler(rr, req)
			// Assert
			require.Equal(t, tc.expectCode, rr.Code)
			tc.assertFn(t, rr.Body.Bytes())
		})
	}
}
