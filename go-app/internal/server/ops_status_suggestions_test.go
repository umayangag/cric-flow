package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// helper to stringify suggestions for contains checks
func stringifySuggestions(sugs []map[string]any) string {
	b, _ := json.Marshal(sugs)
	return string(b)
}

func TestComputeSuggestions_Table(t *testing.T) {
	type setup struct {
		db       map[string]any
		pre      map[string]any
		exp      map[string]any
		arts     map[string]any
		services map[string]bool
		fielding map[string]any
		weather  map[string]any
	}
	type assertion func(t *testing.T, got []map[string]any)

	hasAll := func(substrs ...string) assertion {
		return func(t *testing.T, got []map[string]any) {
			s := stringifySuggestions(got)
			for _, sub := range substrs {
				if !strings.Contains(s, sub) {
					t.Fatalf("missing expected substring %q in %s", sub, s)
				}
			}
		}
	}

	noSuggestions := func() assertion {
		return func(t *testing.T, got []map[string]any) {
			if len(got) != 0 {
				t.Fatalf("expected no suggestions, got %v", got)
			}
		}
	}

	tests := []struct {
		name   string
		setup  setup
		assert assertion
	}{
		{
			name:   "db_disconnected",
			setup:  setup{db: map[string]any{"connected": false}, services: map[string]bool{"ml_health": false}},
			assert: hasAll("make migrate", "cricsheet-import"),
		},
		{
			name: "db_empty_counts",
			setup: setup{
				db:       map[string]any{"connected": true, "counts": map[string]any{"players": 0.0, "matches": 5.0}},
				services: map[string]bool{"ml_health": true},
			},
			assert: hasAll("make migrate", "cricsheet-import"),
		},
		{
			name: "precompute_stale_and_missing",
			setup: setup{
				db: map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0}},
				pre: map[string]any{
					"formats": map[string]any{
						"ODI":  map[string]any{"status": "stale"},
						"T20I": map[string]any{"status": "missing"},
						"TEST": map[string]any{"status": "ok"},
						"T20":  map[string]any{"status": "ok"},
					},
				},
				services: map[string]bool{"ml_health": true},
			},
			assert: hasAll("Precompute missing/stale for ODI,T20I", "make precompute", "precompute-asof"),
		},
		{
			name: "exports_missing_single_and_multi",
			setup: setup{
				db: map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0}},
				exp: map[string]any{"formats": map[string]any{
					"ODI":  map[string]any{"files": []map[string]any{}},
					"TEST": map[string]any{"files": []map[string]any{{"name": "batting_on.csv", "exists": true}}},
					"T20I": map[string]any{"files": []map[string]any{{"name": "x.csv", "exists": true}}},
					"T20":  map[string]any{"files": []map[string]any{{"name": "y.csv", "exists": true}}},
				}},
				services: map[string]bool{"ml_health": true},
			},
			assert: hasAll("Exports missing for ODI", "-format=ODI"),
		},
		{
			name: "exports_missing_multiple_formats",
			setup: setup{
				db: map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0}},
				exp: map[string]any{"formats": map[string]any{
					"ODI":  map[string]any{"files": []map[string]any{}},
					"T20I": map[string]any{"files": []map[string]any{}},
					"TEST": map[string]any{"files": []map[string]any{{"name": "x.csv", "exists": true}}},
					"T20":  map[string]any{"files": []map[string]any{{"name": "y.csv", "exists": true}}},
				}},
				services: map[string]bool{"ml_health": true},
			},
			assert: hasAll("Exports missing for ODI,T20I", "-formats=ODI,T20I"),
		},
		{
			name: "artifacts_missing_and_ml_unhealthy",
			setup: setup{
				db: map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0}},
				arts: map[string]any{"formats": map[string]any{
					"ODI": map[string]any{
						"batting": map[string]any{"exists": true},
						"bowling": map[string]any{"exists": false},
					},
					"T20I": map[string]any{
						"batting": map[string]any{"exists": false},
						"bowling": map[string]any{"exists": false},
					},
					"TEST": map[string]any{
						"batting": map[string]any{"exists": true},
						"bowling": map[string]any{"exists": true},
					},
					"T20": map[string]any{
						"batting": map[string]any{"exists": true},
						"bowling": map[string]any{"exists": true},
					},
				}},
				services: map[string]bool{"ml_health": false},
			},
			assert: hasAll("ML artifacts missing for ODI,T20I", "make train-all", "ML service not healthy"),
		},
		{
			name: "fielding_missing_when_db_ready",
			setup: setup{
				db:       map[string]any{"connected": true, "counts": map[string]any{"players": 5.0, "matches": 7.0}},
				fielding: map[string]any{"available": false, "rows": 0.0}, services: map[string]bool{"ml_health": true},
			},
			assert: hasAll("Fielding data missing", "backfill-fielding"),
		},
		{
			name: "weather_missing_when_db_ready",
			setup: setup{
				db:      map[string]any{"connected": true, "counts": map[string]any{"players": 5.0, "matches": 7.0}},
				weather: map[string]any{"available": false, "rows": 0.0}, services: map[string]bool{"ml_health": true},
			},
			assert: hasAll("Weather data missing", "weather-import"),
		},
		{
			name: "all_ready_no_suggestions",
			setup: setup{
				db: map[string]any{"connected": true, "counts": map[string]any{"players": 10.0, "matches": 20.0}},
				pre: map[string]any{
					"formats": map[string]any{
						"TEST": map[string]any{"status": "ok"},
						"ODI":  map[string]any{"status": "ok"},
						"T20I": map[string]any{"status": "ok"},
						"T20":  map[string]any{"status": "ok"},
					},
				},
				exp: map[string]any{
					"formats": map[string]any{
						"TEST": map[string]any{"files": []map[string]any{{"name": "a.csv", "exists": true}}},
						"ODI":  map[string]any{"files": []map[string]any{{"name": "b.csv", "exists": true}}},
						"T20I": map[string]any{"files": []map[string]any{{"name": "c.csv", "exists": true}}},
						"T20":  map[string]any{"files": []map[string]any{{"name": "d.csv", "exists": true}}},
					},
				},
				arts: map[string]any{
					"formats": map[string]any{
						"TEST": map[string]any{
							"batting": map[string]any{"exists": true},
							"bowling": map[string]any{"exists": true},
						},
						"ODI": map[string]any{
							"batting": map[string]any{"exists": true},
							"bowling": map[string]any{"exists": true},
						},
						"T20I": map[string]any{
							"batting": map[string]any{"exists": true},
							"bowling": map[string]any{"exists": true},
						},
						"T20": map[string]any{
							"batting": map[string]any{"exists": true},
							"bowling": map[string]any{"exists": true},
						},
					},
				},
				services: map[string]bool{
					"ml_health": true,
				}, fielding: map[string]any{"available": true, "rows": 100.0}, weather: map[string]any{"available": true, "rows": 200.0},
			},
			assert: noSuggestions(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeSuggestions(
				tc.setup.db,
				tc.setup.pre,
				tc.setup.exp,
				tc.setup.arts,
				tc.setup.services,
				tc.setup.fielding,
				tc.setup.weather,
			)
			tc.assert(t, got)
		})
	}
}
