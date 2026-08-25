package opsstatus

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildExportsSection inspects a filesystem root for exported CSVs and returns
// a map suitable to be embedded under the `exports` key of /ops/status.
func BuildExportsSection(root string) map[string]any {
	formats := CricketFormatCodes
	out := map[string]any{
		"root":    root,
		"formats": map[string]any{},
	}
	fm := map[string]any{}
	for _, f := range formats {
		fm[f] = map[string]any{
			"files": []map[string]any{},
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		out["formats"] = fm
		return out
	}

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

	appendFile := func(format, file string) {
		full := filepath.Join(root, file)
		info, err := os.Stat(full)
		exists := err == nil && !info.IsDir()
		entry := map[string]any{
			"name":   file,
			"exists": exists,
		}
		if exists {
			entry["modified"] = info.ModTime().UTC().Format(time.RFC3339)
			if rows, err := countCSVRowsCapped(full, 100000); err == nil {
				entry["rows"] = rows
			}
		}
		ff := fm[format].(map[string]any)
		files := ff["files"].([]map[string]any)
		ff["files"] = append(files, entry)
		fm[format] = ff
	}

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

	fileMatchesFormat := func(lowerName, format string) bool {
		fLower := strings.ToLower(format)
		if strings.HasSuffix(lowerName, "_"+fLower+".csv") {
			return true
		}
		if strings.Contains(lowerName, "_"+fLower+"_") {
			return true
		}
		return false
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".csv") {
			continue
		}
		if lower == "batting_on.csv" || lower == "bowling_on.csv" {
			continue
		}
		isBat := strings.Contains(lower, "bat")
		isBowl := strings.Contains(lower, "bowl")
		if !isBat && !isBowl {
			continue
		}
		for _, f := range formats {
			if fileMatchesFormat(lower, f) {
				appendFile(f, name)
			}
		}
	}
	out["formats"] = fm
	return out
}

// countCSVRowsCapped counts lines in a file up to a maximum.
func countCSVRowsCapped(path string, maxRows int) (int, error) {
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
	for count < maxRows {
		line, err := r.ReadBytes('\n')
		_ = line
		if len(line) > 0 {
			count++
		}
		if err != nil {
			break
		}
	}
	return count, nil
}
