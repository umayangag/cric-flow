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

func TestComputeSuggestions_DBDisconnected(t *testing.T) {
    db := map[string]any{"connected": false}
    sugs := computeSuggestions(db, nil, nil, nil, map[string]bool{"ml_health": false})
    got := stringifySuggestions(sugs)
    if !strings.Contains(got, "make migrate") || !strings.Contains(got, "cricsheet-import") {
        t.Fatalf("expected DB migrate/import suggestions, got %s", got)
    }
}

func TestComputeSuggestions_DBEmptyCounts(t *testing.T) {
    db := map[string]any{
        "connected": true,
        "counts": map[string]any{"players": 0.0, "matches": 5.0, "innings": 7.0},
    }
    sugs := computeSuggestions(db, nil, nil, nil, map[string]bool{"ml_health": true})
    got := stringifySuggestions(sugs)
    if !strings.Contains(got, "make migrate") || !strings.Contains(got, "cricsheet-import") {
        t.Fatalf("expected DB migrate/import suggestions, got %s", got)
    }
}

func TestComputeSuggestions_PrecomputeStaleAndMissing(t *testing.T) {
    pre := map[string]any{
        "formats": map[string]any{
            "ODI":  map[string]any{"status": "stale"},
            "T20I": map[string]any{"status": "missing"},
            "TEST": map[string]any{"status": "ok"},
            "T20":  map[string]any{"status": "ok"},
        },
    }
    sugs := computeSuggestions(map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0, "innings": 1.0}}, pre, nil, nil, map[string]bool{"ml_health": true})
    got := stringifySuggestions(sugs)
    if !strings.Contains(got, "Precompute missing/stale for ODI,T20I") {
        t.Fatalf("expected precompute suggestion for ODI,T20I, got %s", got)
    }
    if !strings.Contains(got, "make precompute") || !strings.Contains(got, "precompute-asof") {
        t.Fatalf("expected precompute commands, got %s", got)
    }
}

func TestComputeSuggestions_ExportsMissing_SingleAndMulti(t *testing.T) {
    // Single missing
    exp1 := map[string]any{
        "formats": map[string]any{
            "ODI":  map[string]any{"files": []map[string]any{}},
            "TEST": map[string]any{"files": []map[string]any{{"name": "batting_on.csv", "exists": true}}},
            "T20I": map[string]any{"files": []map[string]any{{"name": "x.csv", "exists": true}}},
            "T20":  map[string]any{"files": []map[string]any{{"name": "y.csv", "exists": true}}},
        },
    }
    sugs1 := computeSuggestions(map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0, "innings": 1.0}}, nil, exp1, nil, map[string]bool{"ml_health": true})
    g1 := stringifySuggestions(sugs1)
    if !strings.Contains(g1, "Exports missing for ODI") || !strings.Contains(g1, "-format=ODI") {
        t.Fatalf("expected single-format export suggestion, got %s", g1)
    }

    // Multiple missing
    exp2 := map[string]any{
        "formats": map[string]any{
            "ODI":  map[string]any{"files": []map[string]any{}},
            "T20I": map[string]any{"files": []map[string]any{}},
            "TEST": map[string]any{"files": []map[string]any{{"name": "x.csv", "exists": true}}},
            "T20":  map[string]any{"files": []map[string]any{{"name": "y.csv", "exists": true}}},
        },
    }
    sugs2 := computeSuggestions(map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0, "innings": 1.0}}, nil, exp2, nil, map[string]bool{"ml_health": true})
    g2 := stringifySuggestions(sugs2)
    if !strings.Contains(g2, "Exports missing for ODI,T20I") || !strings.Contains(g2, "-formats=ODI,T20I") {
        t.Fatalf("expected multi-format export suggestion, got %s", g2)
    }
}

func TestComputeSuggestions_ArtifactsMissing(t *testing.T) {
    arts := map[string]any{
        "formats": map[string]any{
            "ODI":  map[string]any{"batting": map[string]any{"exists": true}, "bowling": map[string]any{"exists": false}},
            "T20I": map[string]any{"batting": map[string]any{"exists": false}, "bowling": map[string]any{"exists": false}},
            "TEST": map[string]any{"batting": map[string]any{"exists": true}, "bowling": map[string]any{"exists": true}},
            "T20":  map[string]any{"batting": map[string]any{"exists": true}, "bowling": map[string]any{"exists": true}},
        },
    }
    sugs := computeSuggestions(map[string]any{"connected": true, "counts": map[string]any{"players": 1.0, "matches": 1.0, "innings": 1.0}}, nil, nil, arts, map[string]bool{"ml_health": false})
    got := stringifySuggestions(sugs)
    if !strings.Contains(got, "ML artifacts missing for ODI,T20I") {
        t.Fatalf("expected artifacts missing suggestion, got %s", got)
    }
    if !strings.Contains(got, "make train-all") {
        t.Fatalf("expected train-all command, got %s", got)
    }
    // Also expect ML service unhealthy suggestion at the end
    if !strings.Contains(got, "ML service not healthy") {
        t.Fatalf("expected ML service suggestion, got %s", got)
    }
}

func TestComputeSuggestions_AllReady_NoSuggestions(t *testing.T) {
    db := map[string]any{"connected": true, "counts": map[string]any{"players": 10.0, "matches": 20.0, "innings": 30.0}}
    pre := map[string]any{"formats": map[string]any{
        "TEST": map[string]any{"status": "ok"},
        "ODI":  map[string]any{"status": "ok"},
        "T20I": map[string]any{"status": "ok"},
        "T20":  map[string]any{"status": "ok"},
    }}
    exp := map[string]any{"formats": map[string]any{
        "TEST": map[string]any{"files": []map[string]any{{"name": "a.csv", "exists": true}}},
        "ODI":  map[string]any{"files": []map[string]any{{"name": "b.csv", "exists": true}}},
        "T20I": map[string]any{"files": []map[string]any{{"name": "c.csv", "exists": true}}},
        "T20":  map[string]any{"files": []map[string]any{{"name": "d.csv", "exists": true}}},
    }}
    arts := map[string]any{"formats": map[string]any{
        "TEST": map[string]any{"batting": map[string]any{"exists": true}, "bowling": map[string]any{"exists": true}},
        "ODI":  map[string]any{"batting": map[string]any{"exists": true}, "bowling": map[string]any{"exists": true}},
        "T20I": map[string]any{"batting": map[string]any{"exists": true}, "bowling": map[string]any{"exists": true}},
        "T20":  map[string]any{"batting": map[string]any{"exists": true}, "bowling": map[string]any{"exists": true}},
    }}
    sugs := computeSuggestions(db, pre, exp, arts, map[string]bool{"ml_health": true})
    if len(sugs) != 0 {
        t.Fatalf("expected no suggestions, got %v", sugs)
    }
}
