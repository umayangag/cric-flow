// Package csvx contains CSV parsing utilities shared across commands.
package csvx

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
)

// RetiredRow represents a CSV row for retired players.
type RetiredRow struct {
	Name    string
	Retired bool
}

// ParseRetiredCSV parses a CSV of retired players from the provided filesystem path.
// Behavior mirrors the original cmd/import-retired parser:
// - Header detection for name column: name|player_name
// - Optional retired flag column: retired|is_retired
// - When retired column missing, listed names are treated as retired=true
// - Truthy values: 1,true,yes,y (case-insensitive). Falsy: 0,false,no,n
// - Empty names are skipped
// - Only rows with Retired=true are included in the output
func ParseRetiredCSV(fsys fs.FS, path string) ([]RetiredRow, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if c, ok := f.(io.Closer); ok {
			if err := c.Close(); err != nil {
				slog.Warn("close csv file failed", slog.String("path", path), slog.Any("err", err))
			}
		}
	}()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	nameIdx := -1
	retIdx := -1
	for i, h := range head {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "name" || h == "player_name" {
			nameIdx = i
		}
		if h == "retired" || h == "is_retired" {
			retIdx = i
		}
	}
	if nameIdx < 0 {
		return nil, errors.New("CSV must contain a 'Name' or 'player_name' column")
	}

	var out []RetiredRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if nameIdx >= len(rec) {
			continue
		}
		name := strings.TrimSpace(rec[nameIdx])
		if name == "" {
			continue
		}
		ret := true // default to true when column missing
		if retIdx >= 0 && retIdx < len(rec) {
			val := strings.TrimSpace(strings.ToLower(rec[retIdx]))
			switch val {
			case "1", "true", "yes", "y":
				ret = true
			case "0", "false", "no", "n":
				ret = false
			default:
				ret = true
			}
		}
		if ret {
			out = append(out, RetiredRow{Name: name, Retired: true})
		}
	}
	return out, nil
}
