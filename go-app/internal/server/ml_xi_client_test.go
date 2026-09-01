package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

func TestAsOfParam_RendersADateAndOmitsTheZeroTime(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", asOfParam(time.Time{}))
	assert.Equal(t, "2025-09-01", asOfParam(time.Date(2025, 9, 1, 14, 30, 0, 0, time.UTC)))
}

// xiCaptureServer answers any /xi/* POST with the given body and records the request JSON.
func xiCaptureServer(t *testing.T, response string) (*BacktestMLClient, *map[string]interface{}) {
	t.Helper()
	captured := map[string]interface{}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &captured))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return &BacktestMLClient{BaseURL: srv.URL, HTTP: srv.Client()}, &captured
}

func TestPredictMatchWinXI_SendsAsOfOnlyWhenSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		asOf      time.Time
		wantField bool
		wantValue string
	}{
		{
			name:      "a backtest date is sent as YYYY-MM-DD",
			asOf:      time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			wantField: true,
			wantValue: "2025-03-01",
		},
		{name: "a live prediction omits the field", asOf: time.Time{}, wantField: false},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, captured := xiCaptureServer(t, `{"team1_win_probability":0.6,"objective_probability":0.55}`)

			p, err := client.PredictMatchWinXI(context.Background(), predictteam.XIWinRequest{
				Format:         "T20",
				Team1PlayerIDs: []int64{1},
				Team2PlayerIDs: []int64{2},
				AsOf:           tc.asOf,
			})

			require.NoError(t, err)
			assert.InDelta(t, 0.6, p, 1e-9)
			value, present := (*captured)["as_of"]
			assert.Equal(t, tc.wantField, present)
			if tc.wantField {
				assert.Equal(t, tc.wantValue, value)
			}
		})
	}
}

func TestOptimizeXI_SendsAsOf(t *testing.T) {
	t.Parallel()

	client, captured := xiCaptureServer(
		t, `{"selected_player_ids":[1],"win_probability":0.5,"evaluations":1,"improved_over_seed":0}`,
	)

	_, err := client.OptimizeXI(context.Background(), predictteam.XIOptimizationRequest{
		Format:            "T20",
		PoolPlayerIDs:     []int64{1, 2},
		OpponentPlayerIDs: []int64{3},
		AsOf:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	assert.Equal(t, "2025-03-01", (*captured)["as_of"])
}
