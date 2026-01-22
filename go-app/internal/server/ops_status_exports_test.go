package server

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestBuildExportsSection_EmptyOrMissingDir(t *testing.T) {
    // Use a temp dir and a non-existing subdir to ensure graceful handling
    tmp := t.TempDir()
    root := filepath.Join(tmp, "does-not-exist")

    sec := buildExportsSection(root)
    b, _ := json.Marshal(sec)
    _ = b // for easier debugging on failure
    if sec["root"] != root {
        t.Fatalf("root mismatch: got %v want %v", sec["root"], root)
    }
    fm, ok := sec["formats"].(map[string]any)
    if !ok {
        t.Fatalf("formats missing or wrong type: %T", sec["formats"])}
    // Expect all formats present with empty files arrays
    for _, f := range []string{"TEST","ODI","T20I","T20"} {
        vf, ok := fm[f].(map[string]any)
        if !ok {
            t.Fatalf("format %s missing", f)
        }
        files, ok := vf["files"].([]map[string]any)
        if !ok {
            // might have been decoded as []any when going through JSON, but here we used direct value
            // so assert zero length using generic interface path
            if arr, ok2 := vf["files"].([]any); ok2 {
                if len(arr) != 0 { t.Fatalf("files not empty for %s", f) }
                continue
            }
            // If still not slice, fail
            t.Fatalf("files for %s wrong type: %T", f, vf["files"])
        }
        if len(files) != 0 {
            t.Fatalf("expected no files for %s, got %d", f, len(files))
        }
    }
}

func writeFileWithLines(t *testing.T, dir, name string, lines int) string {
    t.Helper()
    p := filepath.Join(dir, name)
    f, err := os.Create(p)
    if err != nil { t.Fatalf("create: %v", err) }
    defer f.Close()
    for i := 0; i < lines; i++ {
        if _, err := f.WriteString("row\n"); err != nil { t.Fatalf("write: %v", err) }
    }
    // Ensure modtime is deterministic (set to fixed time)
    mt := time.Date(2026,1,21,12,0,0,0,time.UTC)
    if err := os.Chtimes(p, mt, mt); err != nil { t.Fatalf("chtimes: %v", err) }
    return p
}

func TestBuildExportsSection_UnifiedFilesAppliedToAllFormats(t *testing.T) {
    root := t.TempDir()
    writeFileWithLines(t, root, "batting_on.csv", 3)
    writeFileWithLines(t, root, "bowling_on.csv", 2)

    sec := buildExportsSection(root)
    fm := sec["formats"].(map[string]any)
    for _, f := range []string{"TEST","ODI","T20I","T20"} {
        vf := fm[f].(map[string]any)
        files := vf["files"].([]map[string]any)
        if len(files) < 2 {
            t.Fatalf("expected >=2 files for %s, got %d", f, len(files))
        }
        // Find our two known files
        var seenBat, seenBowl bool
        for _, e := range files {
            switch e["name"] {
            case "batting_on.csv":
                seenBat = e["exists"].(bool)
                if rows, ok := e["rows"].(int); ok && rows == 0 {
                    t.Fatalf("batting rows should be >0")
                }
                if _, ok := e["modified"].(string); !ok {
                    t.Fatalf("modified missing for batting_on.csv")
                }
            case "bowling_on.csv":
                seenBowl = e["exists"].(bool)
            }
        }
        if !seenBat || !seenBowl {
            t.Fatalf("unified files not surfaced for %s (bat:%v bowl:%v)", f, seenBat, seenBowl)
        }
    }
}

func TestBuildExportsSection_PerFormatDetection(t *testing.T) {
    root := t.TempDir()
    writeFileWithLines(t, root, "my_batting_ODI_2020.csv", 4)
    writeFileWithLines(t, root, "stats_bowling_T20I.csv", 5)

    sec := buildExportsSection(root)
    fm := sec["formats"].(map[string]any)

    // ODI should include the batting file
    odi := fm["ODI"].(map[string]any)
    var hasODI bool
    for _, e := range odi["files"].([]map[string]any) {
        if e["name"] == "my_batting_ODI_2020.csv" && e["exists"].(bool) {
            hasODI = true
        }
    }
    if !hasODI { t.Fatalf("ODI file not detected") }

    // T20I should include the bowling file
    t20i := fm["T20I"].(map[string]any)
    var hasT20I bool
    for _, e := range t20i["files"].([]map[string]any) {
        if e["name"] == "stats_bowling_T20I.csv" && e["exists"].(bool) {
            hasT20I = true
        }
    }
    if !hasT20I { t.Fatalf("T20I file not detected") }
}
