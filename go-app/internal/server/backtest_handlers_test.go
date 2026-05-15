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
			if got != tc.want {
				t.Fatalf("chooseBacktestMode(%q,%q)=%q want %q", tc.modeIn, tc.matchID, got, tc.want)
			}
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
			if !almostEqual(got, tc.want) {
				t.Fatalf("computeR2(%v,%v)=%v want %v", tc.sse, tc.y, got, tc.want)
			}
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
			if got != tc.want {
				t.Fatalf("winnerAccuracy(%q,%q)=%v want %v", tc.pred, tc.act, got, tc.want)
			}
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
				if !p.IncludePlayer || !p.IncludeTeam {
					t.Fatalf("metrics defaults not applied: %+v", p)
				}
				require.Equal(t, 100, p.Limit)
				if p.Order != "asc" {
					t.Fatalf("default order=asc, got %q", p.Order)
				}
				if p.Cache != "readwrite" {
					t.Fatalf("default cache=readwrite, got %q", p.Cache)
				}
			},
		},
		{
			name:    "limit cap and selections",
			rawURL:  "/api/backtest/accuracy-trend?format=t20&team1=IND&team2=AUS&limit=9999&order=desc&cache=off&metrics=player",
			wantErr: nil,
			assertFunc: func(t *testing.T, p accuracyTrendParams) {
				require.Equal(t, 500, p.Limit)
				if p.Order != "desc" {
					t.Fatalf("order=desc expected, got %q", p.Order)
				}
				if p.Cache != "off" {
					t.Fatalf("cache=off expected, got %q", p.Cache)
				}
				if !p.IncludePlayer || p.IncludeTeam {
					t.Fatalf(
						"metrics selection expected player only, got player=%v team=%v",
						p.IncludePlayer,
						p.IncludeTeam,
					)
				}
			},
		},
		{
			name:    "unknown metrics token => defaults to both",
			rawURL:  "/api/backtest/accuracy-trend?format=odi&team1=IND&team2=AUS&metrics=unknown",
			wantErr: nil,
			assertFunc: func(t *testing.T, p accuracyTrendParams) {
				if !p.IncludePlayer || !p.IncludeTeam {
					t.Fatalf("unknown metrics should default to both true, got %+v", p)
				}
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
				if err.Error() != tc.wantErr.Error() {
					t.Fatalf("error mismatch: got %v, want %v", err, tc.wantErr)
				}
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
			if got != tc.want {
				t.Fatalf("parseUseUnifiedModel(%q, %v)=%v want %v", tc.rawURL, tc.defaultVal, got, tc.want)
			}
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
			if got != tc.want {
				t.Fatalf("parseUseLatestModel(%q, %v)=%v want %v", tc.rawURL, tc.defaultVal, got, tc.want)
			}
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

	if got, ok := summary["n"]; !ok || got != 3 {
		t.Fatalf("summary n expected 3, got %v", summary["n"])
	}
	if got := summary["player_runs_mae_avg"]; !almostEqual(got, 15) {
		t.Fatalf("player_runs_mae_avg got %v want 15", got)
	}
	if got := summary["team_runs_mae_avg"]; !almostEqual(got, 6) {
		t.Fatalf("team_runs_mae_avg got %v want 6", got)
	}
	if got := summary["team_winner_accuracy_avg"]; !almostEqual(got, 1) {
		t.Fatalf("team_winner_accuracy_avg got %v want 1", got)
	}
	if len(prog) != 3 {
		t.Fatalf("progressive len got %d want 3", len(prog))
	}
	if got := prog[0]["player_runs_mae_avg"]; !almostEqual(got, 10) {
		t.Fatalf("prog0 player_runs_mae_avg got %v want 10", got)
	}
	if got := prog[0]["team_runs_mae_avg"]; !almostEqual(got, 5) {
		t.Fatalf("prog0 team_runs_mae_avg got %v want 5", got)
	}
	if got := prog[1]["team_runs_mae_avg"]; !almostEqual(got, 6) {
		t.Fatalf("prog1 team_runs_mae_avg got %v want 6", got)
	}
	if got := prog[1]["team_winner_accuracy_avg"]; !almostEqual(got, 1) {
		t.Fatalf("prog1 winner_accuracy got %v want 1", got)
	}
}
