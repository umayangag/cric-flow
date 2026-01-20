package server

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func almostEqual(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

func TestChooseBacktestMode(t *testing.T) {
	tests := []struct {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseBacktestMode(tt.modeIn, tt.matchID)
			if got != tt.want {
				t.Fatalf("chooseBacktestMode(%q,%q)=%q want %q", tt.modeIn, tt.matchID, got, tt.want)
			}
		})
	}
}

func TestComputeR2(t *testing.T) {
	tests := []struct {
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
			want: 1 - (2.0 / (5.0)),
		}, // ssTot for [1,2,3,4] is 5
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeR2(tt.sse, tt.y)
			if !almostEqual(got, tt.want, 1e-9) {
				t.Fatalf("computeR2(%v,%v)=%v want %v", tt.sse, tt.y, got, tt.want)
			}
		})
	}
}

func TestWinnerAccuracy(t *testing.T) {
	tests := []struct {
		name string
		pred string
		act  string
		want float64
	}{
		{name: "empty any => 0", pred: "", act: "IND", want: 0},
		{name: "match case-insensitive => 1", pred: "ind", act: "IND", want: 1},
		{name: "different => 0", pred: "IND", act: "AUS", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := winnerAccuracy(tt.pred, tt.act)
			if got != tt.want {
				t.Fatalf("winnerAccuracy(%q,%q)=%v want %v", tt.pred, tt.act, got, tt.want)
			}
		})
	}
}

func TestParseBacktestAccuracyTrendParams(t *testing.T) {
	makeReq := func(rawURL string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, rawURL, nil)
		return r
	}

	tests := []struct {
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
				if p.Limit != 100 {
					t.Fatalf("default limit=100, got %d", p.Limit)
				}
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
				if p.Limit != 500 {
					t.Fatalf("limit should be capped to 500, got %d", p.Limit)
				}
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := makeReq(tt.rawURL)
			p, err := parseBacktestAccuracyTrendParams(r)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr.Error() {
					t.Fatalf("error mismatch: got %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.assertFunc != nil {
				tt.assertFunc(t, p)
			}
		})
	}
}

func TestComputeAccuracyTrendSummaryAndProgressive(t *testing.T) {
	// Craft 3 items with partial metric presence
	items := []accuracyTrendItem{
		{Metrics: map[string]float64{"player_runs_mae": 10, "team_runs_mae": 5}},
		{Metrics: map[string]float64{"team_runs_mae": 7, "team_winner_accuracy": 1}},
		{Metrics: map[string]float64{"player_runs_mae": 20}},
	}
	summary, prog := computeAccuracyTrendSummaryAndProgressive(items)

	// Summary checks
	if got, ok := summary["n"]; !ok || got != 3 {
		t.Fatalf("summary n expected 3, got %v", summary["n"])
	}
	// player_runs_mae present in 2 items: (10 + 20)/2 = 15
	if got := summary["player_runs_mae_avg"]; !almostEqual(got, 15, 1e-9) {
		t.Fatalf("player_runs_mae_avg got %v want 15", got)
	}
	// team_runs_mae present in 2 items: (5 + 7)/2 = 6
	if got := summary["team_runs_mae_avg"]; !almostEqual(got, 6, 1e-9) {
		t.Fatalf("team_runs_mae_avg got %v want 6", got)
	}
	// winner_accuracy present in 1 item: 1/1 = 1
	if got := summary["team_winner_accuracy_avg"]; !almostEqual(got, 1, 1e-9) {
		t.Fatalf("team_winner_accuracy_avg got %v want 1", got)
	}

	// Progressive length equals items
	if len(prog) != 3 {
		t.Fatalf("progressive len got %d want 3", len(prog))
	}
	// After first item: player=10, team=5
	if got := prog[0]["player_runs_mae_avg"]; !almostEqual(got, 10, 1e-9) {
		t.Fatalf("prog0 player_runs_mae_avg got %v want 10", got)
	}
	if got := prog[0]["team_runs_mae_avg"]; !almostEqual(got, 5, 1e-9) {
		t.Fatalf("prog0 team_runs_mae_avg got %v want 5", got)
	}
	// After second item: player still 10/1, team (5+7)/2 = 6, winner 1/1 = 1
	if got := prog[1]["team_runs_mae_avg"]; !almostEqual(got, 6, 1e-9) {
		t.Fatalf("prog1 team_runs_mae_avg got %v want 6", got)
	}
	if got := prog[1]["team_winner_accuracy_avg"]; !almostEqual(got, 1, 1e-9) {
		t.Fatalf("prog1 winner_accuracy got %v want 1", got)
	}
}
