package server

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
)

func almostEqual(a, b float64) bool {
	const eps = 1e-9
	return math.Abs(a-b) <= eps
}

func TestChooseBacktestMode(t *testing.T) {
	testCases := []struct {
		name    string
		modeIn  string
		matchID string
		want    string
	}{
		{name: "empty mode no match_id => select", modeIn: "", matchID: "", want: "select"},
		{name: "empty mode with match_id => evaluate", modeIn: "", matchID: "123", want: "evaluate"},
		{name: "explicit select kept", modeIn: "select", matchID: "", want: "select"},
		{name: "explicit evaluate kept", modeIn: "evaluate", matchID: "", want: "evaluate"},
		{name: "whitespace trimmed", modeIn: "  select  ", matchID: "", want: "select"},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := chooseBacktestMode(tc.modeIn, tc.matchID)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestComputeR2(t *testing.T) {
	testCases := []struct {
		name string
		sse  float64
		y    []float64
		want float64
	}{
		{name: "empty actuals => 0", sse: 0, y: nil, want: 0},
		{name: "constant actuals => 0", sse: 0, y: []float64{5, 5, 5}, want: 0},
		{name: "perfect prediction => 1", sse: 0, y: []float64{1, 2, 3, 4}, want: 1},
		{
			name: "non-perfect < 1",
			sse:  2,
			y:    []float64{1, 2, 3, 4},
			want: 1 - (2.0 / 5.0),
		}, // ssTot for [1,2,3,4] is 5
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := computeR2(tc.sse, tc.y)
			require.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestWinnerAccuracy(t *testing.T) {
	testCases := []struct {
		name string
		pred string
		act  string
		want float64
	}{
		{name: "empty any => 0", pred: "", act: "IND", want: 0},
		{name: "match case-insensitive => 1", pred: "ind", act: "IND", want: 1},
		{name: "different => 0", pred: "IND", act: "AUS", want: 0},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := winnerAccuracy(tc.pred, tc.act)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseBacktestAccuracyTrendParams(t *testing.T) {
	makeReq := func(rawURL string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, rawURL, nil)
		return r
	}

	testCases := []struct {
		name       string
		rawURL     string
		wantErr    error
		assertFunc func(t *testing.T, p accuracyTrendParams)
	}{
		{
			name:    "defaults: metrics omitted => both included; limit default; order asc; cache readwrite",
			rawURL:  "/api/backtest/accuracy-trend?format=odi&team1=IND&team2=AUS",
			wantErr: nil,
			assertFunc: func(t *testing.T, p accuracyTrendParams) {
				require.True(t, p.IncludePlayer, "IncludePlayer default")
				require.True(t, p.IncludeTeam, "IncludeTeam default")
				require.Equal(t, 100, p.Limit)
				require.Equal(t, "asc", p.Order)
				require.Equal(t, "readwrite", p.Cache)
			},
		},
		{
			name:    "limit cap and selections",
			rawURL:  "/api/backtest/accuracy-trend?format=t20&team1=IND&team2=AUS&limit=9999&order=desc&cache=off&metrics=player",
			wantErr: nil,
			assertFunc: func(t *testing.T, p accuracyTrendParams) {
				require.Equal(t, 500, p.Limit)
				require.Equal(t, "desc", p.Order)
				require.Equal(t, "off", p.Cache)
				require.True(t, p.IncludePlayer, "IncludePlayer")
				require.False(t, p.IncludeTeam, "IncludeTeam")
			},
		},
		{
			name:    "unknown metrics token => defaults to both",
			rawURL:  "/api/backtest/accuracy-trend?format=odi&team1=IND&team2=AUS&metrics=unknown",
			wantErr: nil,
			assertFunc: func(t *testing.T, p accuracyTrendParams) {
				require.True(t, p.IncludePlayer, "IncludePlayer")
				require.True(t, p.IncludeTeam, "IncludeTeam")
			},
		},
		{
			name:    "invalid start_date",
			rawURL:  "/api/backtest/accuracy-trend?format=odi&team1=IND&team2=AUS&start_date=2025-13-40",
			wantErr: errInvalidStartDate,
		},
		{
			name:    "invalid end_date",
			rawURL:  "/api/backtest/accuracy-trend?format=odi&team1=IND&team2=AUS&end_date=2024-02-30",
			wantErr: errInvalidEndDate,
		},
		{
			name:    "end before start",
			rawURL:  "/api/backtest/accuracy-trend?format=odi&team1=IND&team2=AUS&start_date=2025-01-10&end_date=2024-12-31",
			wantErr: errEndBeforeStart,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			r := makeReq(tc.rawURL)
			p, err := parseBacktestAccuracyTrendParams(r)
			if tc.wantErr != nil {
 			require.Error(t, err)
				require.Equal(t, tc.wantErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			if tc.assertFunc != nil {
				tc.assertFunc(t, p)
			}
		})
	}
}

func TestParseUseUnifiedModel(t *testing.T) {
	testCases := []struct {
		name       string
		rawURL     string
		defaultVal bool
		want       bool
	}{
		{"default false", "/api/backtest?match_id=1", false, false},
		{"default true", "/api/backtest?match_id=1", true, true},
		{"use_unified_model=1", "/api/backtest?match_id=1&use_unified_model=1", false, true},
		{"use_unified_model=true", "/api/backtest?match_id=1&use_unified_model=true", false, true},
		{"model=unified", "/api/backtest?match_id=1&model=unified", false, true},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.rawURL, nil)
			got := parseUseUnifiedModel(r, tc.defaultVal)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseUseLatestModel(t *testing.T) {
	testCases := []struct {
		name       string
		rawURL     string
		defaultVal bool
		want       bool
	}{
		{"default false", "/api/backtest?match_id=1", false, false},
		{"default true", "/api/backtest?match_id=1", true, true},
		{"use_latest_model=1", "/api/backtest?match_id=1&use_latest_model=1", false, true},
		{"use_latest_model=true", "/api/backtest?match_id=1&use_latest_model=true", false, true},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.rawURL, nil)
			got := parseUseLatestModel(r, tc.defaultVal)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIntPtr(t *testing.T) {
	got := intPtr(42)
	require.NotNil(t, got)
	require.Equal(t, 42, *got)
}

func TestFloat32Ptr(t *testing.T) {
	got := float32Ptr(3.14)
	require.NotNil(t, got)
	require.Equal(t, float32(3.14), *got)
}

func TestComputeAccuracyTrendSummaryAndProgressive(t *testing.T) {
	// Same test as services/backtest; ensures server wiring produces same result.
	items := []accuracyTrendItem{
		{Metrics: map[string]float64{"player_runs_mae": 10, "team_runs_mae": 5}},
		{Metrics: map[string]float64{"team_runs_mae": 7, "team_winner_accuracy": 1}},
		{Metrics: map[string]float64{"player_runs_mae": 20}},
	}
	svcItems := make([]backtest.AccuracyTrendItem, len(items))
	for i := range items {
		svcItems[i] = backtest.AccuracyTrendItem{
			MatchID:   items[i].MatchID,
			MatchDate: items[i].MatchDate,
			Format:    items[i].Format,
			Team1:     items[i].Team1,
			Team2:     items[i].Team2,
			Metrics:   items[i].Metrics,
		}
	}
	summary, prog := backtest.ComputeSummaryAndProgressive(svcItems)

	require.Equal(t, float64(3), summary["n"])
	require.InDelta(t, 15.0, summary["player_runs_mae_avg"], 1e-9)
	require.InDelta(t, 6.0, summary["team_runs_mae_avg"], 1e-9)
	require.InDelta(t, 1.0, summary["team_winner_accuracy_avg"], 1e-9)
	require.Len(t, prog, 3)
	require.InDelta(t, 10.0, prog[0]["player_runs_mae_avg"], 1e-9)
	require.InDelta(t, 5.0, prog[0]["team_runs_mae_avg"], 1e-9)
	require.InDelta(t, 6.0, prog[1]["team_runs_mae_avg"], 1e-9)
	require.InDelta(t, 1.0, prog[1]["team_winner_accuracy_avg"], 1e-9)
}
