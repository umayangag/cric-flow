package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// row represents a CSV row for retired players
type row struct {
	Name    string
	Retired bool
}

// parseCSV parses the retired CSV file.
func parseCSV(path string) ([]row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close csv file failed", slog.String("path", path), slog.Any("err", err))
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

	var out []row
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
		ret := true // default: if no Retired column, treat listed names as retired
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
		if ret { // only include truthy rows for marking retired
			out = append(out, row{Name: name, Retired: true})
		}
	}
	return out, nil
}

// applyRetired executes updates for retired flags.
func applyRetired(ctx context.Context, names []string, othersZero bool) (int64, int64, error) {
	if db.Pool == nil {
		return 0, 0, errors.New("db not initialized")
	}
	var changed int64
	// Mark listed players as retired
	for _, nm := range names {
		ct, err := db.Pool.Exec(ctx, `UPDATE player SET is_retired = 1 WHERE lower(player_name) = lower($1)`, nm)
		if err != nil {
			return changed, 0, err
		}
		changed += ct.RowsAffected()
	}
	var zeroed int64
	if othersZero {
		if len(names) == 0 {
			// nothing listed; zero all currently non-zero or null
			ct, err := db.Pool.Exec(ctx, `UPDATE player SET is_retired = 0 WHERE is_retired IS DISTINCT FROM 0`)
			if err != nil {
				return changed, zeroed, err
			}
			zeroed = ct.RowsAffected()
		} else {
			// Build NOT IN clause dynamically
			placeholders := make([]string, len(names))
			args := make([]any, len(names))
			for i, n := range names {
				placeholders[i] = fmt.Sprintf("lower($%d)", i+1)
				args[i] = n
			}
			q := fmt.Sprintf(`UPDATE player SET is_retired = 0 WHERE lower(player_name) NOT IN ($1) AND is_retired IS DISTINCT FROM 0`, strings.Join(placeholders, ","))
			ct, err := db.Pool.Exec(ctx, q, args...)
			if err != nil {
				return changed, zeroed, err
			}
			zeroed = ct.RowsAffected()
		}
	}
	return changed, zeroed, nil
}
