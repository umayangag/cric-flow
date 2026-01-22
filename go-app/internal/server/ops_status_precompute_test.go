package server

import (
    "encoding/json"
    "testing"
    "time"

    "github.com/umayangag/cric-info-scrapers/go-app/internal/precompute"
)

// helper to extract map[string]any safely
func getMap(m map[string]any, key string, t *testing.T) map[string]any {
    v, ok := m[key]
    if !ok {
        t.Fatalf("missing key %s", key)
    }
    mv, ok := v.(map[string]any)
    if !ok {
        t.Fatalf("key %s is not a map: %T", key, v)
    }
    return mv
}

func TestBuildPrecomputeSection_NoFinishedRun_AllMissing(t *testing.T) {
    // Arrange
    orig := getPrecomputeStatus
    t.Cleanup(func() { getPrecomputeStatus = orig })
    getPrecomputeStatus = func() precompute.Status {
        return precompute.Status{} // zero FinishedAt
    }
    now := time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC)

    // Act
    sec := buildPrecomputeSection(now)

    // Assert
    if sec["last_run"] != "" || sec["as_of"] != "" {
        b, _ := json.Marshal(sec)
        t.Fatalf("expected empty last_run/as_of, got %s", string(b))
    }
    formats := getMap(sec, "formats", t)
    wantMissing := []string{"TEST", "ODI", "T20I", "T20"}
    for _, f := range wantMissing {
        st := getMap(formats, f, t)["status"].(string)
        if st != "missing" {
            t.Fatalf("format %s expected missing, got %s", f, st)
        }
    }
}

func TestBuildPrecomputeSection_Today_OkForRanFormats(t *testing.T) {
    orig := getPrecomputeStatus
    t.Cleanup(func() { getPrecomputeStatus = orig })
    finished := time.Date(2026, 1, 21, 6, 30, 0, 0, time.UTC)
    getPrecomputeStatus = func() precompute.Status {
        return precompute.Status{FinishedAt: finished, Formats: []string{"ODI", "T20"}}
    }
    now := time.Date(2026, 1, 21, 18, 0, 0, 0, time.UTC)

    sec := buildPrecomputeSection(now)
    if sec["last_run"] == "" || sec["as_of"] == "" {
        t.Fatalf("expected last_run/as_of to be set")
    }
    formats := getMap(sec, "formats", t)
    if st := getMap(formats, "ODI", t)["status"].(string); st != "ok" {
        t.Fatalf("ODI expected ok, got %s", st)
    }
    if st := getMap(formats, "T20", t)["status"].(string); st != "ok" {
        t.Fatalf("T20 expected ok, got %s", st)
    }
    if st := getMap(formats, "TEST", t)["status"].(string); st != "missing" {
        t.Fatalf("TEST expected missing, got %s", st)
    }
    if st := getMap(formats, "T20I", t)["status"].(string); st != "missing" {
        t.Fatalf("T20I expected missing, got %s", st)
    }
}

func TestBuildPrecomputeSection_Yesterday_StaleForRanFormats(t *testing.T) {
    orig := getPrecomputeStatus
    t.Cleanup(func() { getPrecomputeStatus = orig })
    finished := time.Date(2026, 1, 20, 23, 50, 0, 0, time.UTC)
    getPrecomputeStatus = func() precompute.Status {
        return precompute.Status{FinishedAt: finished, Formats: []string{"TEST", "T20I"}}
    }
    now := time.Date(2026, 1, 21, 0, 10, 0, 0, time.UTC)

    sec := buildPrecomputeSection(now)
    formats := getMap(sec, "formats", t)
    if st := getMap(formats, "TEST", t)["status"].(string); st != "stale" {
        t.Fatalf("TEST expected stale, got %s", st)
    }
    if st := getMap(formats, "T20I", t)["status"].(string); st != "stale" {
        t.Fatalf("T20I expected stale, got %s", st)
    }
    if st := getMap(formats, "ODI", t)["status"].(string); st != "missing" {
        t.Fatalf("ODI expected missing, got %s", st)
    }
    if st := getMap(formats, "T20", t)["status"].(string); st != "missing" {
        t.Fatalf("T20 expected missing, got %s", st)
    }
}
