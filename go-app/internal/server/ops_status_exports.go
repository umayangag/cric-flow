package server

import (
    "bufio"
    "log/slog"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// buildExportsSection inspects a filesystem root for exported CSVs and returns
// a map suitable to be embedded under the `exports` key of /ops/status.
// The function is conservative and pattern-based:
// - Recognizes unified files: batting_on.csv, bowling_on.csv (apply to all formats)
// - Recognizes per-format files by substring match containing the format code
//   and either "bat" or "bowl" tokens.
// Row counting is capped to avoid heavy reads.
func buildExportsSection(root string) map[string]any {
    // Ensure root exists; if not, return empty scaffold
    formats := []string{"TEST", "ODI", "T20I", "T20"}
    out := map[string]any{
        "root":    root,
        "formats": map[string]any{},
    }

    // Prepare container per format
    fm := map[string]any{}
    for _, f := range formats {
        fm[f] = map[string]any{
            "files": []map[string]any{},
        }
    }

    // List directory entries (non-recursive)
    entries, err := os.ReadDir(root)
    if err != nil {
        // Directory may not exist yet; return empty structures
        out["formats"] = fm
        return out
    }

    // Collect unified files once
    var unifiedBat, unifiedBowl string
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        name := e.Name()
        lower := strings.ToLower(name)
        if strings.HasSuffix(lower, ".csv") {
            if lower == "batting_on.csv" {
                unifiedBat = name
            }
            if lower == "bowling_on.csv" {
                unifiedBowl = name
            }
        }
    }

    // Helper to append a file entry for a format if it matches
    appendFile := func(format, file string) {
        full := filepath.Join(root, file)
        info, err := os.Stat(full)
        exists := err == nil && !info.IsDir()
        entry := map[string]any{
            "name":    file,
            "exists":  exists,
        }
        if exists {
            entry["modified"] = info.ModTime().UTC().Format(time.RFC3339)
            if rows, err := countCSVRowsCapped(full, 10000); err == nil {
                entry["rows"] = rows
            }
        }
        // append to format's files list
        ff := fm[format].(map[string]any)
        files := ff["files"].([]map[string]any)
        ff["files"] = append(files, entry)
        fm[format] = ff
    }

    // First, if unified files exist, add them for all formats
    if unifiedBat != "" {
        for _, f := range formats {
            appendFile(f, unifiedBat)
        }
    }
    if unifiedBowl != "" {
        for _, f := range formats {
            appendFile(f, unifiedBowl)
        }
    }

    // Next, scan for per-format files
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        name := e.Name()
        lower := strings.ToLower(name)
        if !strings.HasSuffix(lower, ".csv") {
            continue
        }
        // skip unified, already handled
        if lower == "batting_on.csv" || lower == "bowling_on.csv" {
            continue
        }
        // crude classification tokens
        isBat := strings.Contains(lower, "bat")
        isBowl := strings.Contains(lower, "bowl")
        if !isBat && !isBowl {
            continue
        }
        for _, f := range formats {
            if strings.Contains(strings.ToLower(name), strings.ToLower(f)) {
                appendFile(f, name)
            }
        }
    }

    out["formats"] = fm
    return out
}

// countCSVRowsCapped counts lines in a file up to a maximum; returns the
// counted number and never reads the entire file if not needed.
func countCSVRowsCapped(path string, max int) (int, error) {
    f, err := os.Open(path)
    if err != nil {
        return 0, err
    }
    defer func() {
        if cerr := f.Close(); cerr != nil {
            slog.Warn("ops-status: close csv failed", slog.String("path", path), slog.Any("err", cerr))
        }
    }()
    r := bufio.NewReaderSize(f, 64*1024)
    count := 0
    for count < max {
        line, err := r.ReadBytes('\n')
        _ = line // ignore content
        if len(line) > 0 {
            count++
        }
        if err != nil {
            // eof or other errors end loop
            break
        }
    }
    return count, nil
}
