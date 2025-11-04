// Command import-retired updates the is_retired flag in the player table from a CSV.
//
// CSV schema (minimum):
//   - Name (string)            required; matched case-insensitively against player.player_name
//   - Retired (bool/int)       optional; if present, rows with truthy Retired will be marked retired (1).
//                               If absent, all listed names will be marked retired (1).
//
// Usage examples:
//   go run ./go-app/cmd/import-retired --file ./data/retired.csv --dry-run
//   go run ./go-app/cmd/import-retired --file ./data/retired.csv --apply
//   go run ./go-app/cmd/import-retired --file ./data/retired.csv --apply --others-zero
//
// Flags:
//   --file         path to CSV file (required)
//   --dry-run      do not write to DB; print intended changes (default true)
//   --apply        apply changes (sets dry-run=false)
//   --others-zero  set is_retired=0 for players not listed (use with caution)
//   --timeout      operation timeout (default 60s)
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

type row struct {
	Name    string
	Retired bool
}

func parseCSV(path string) ([]row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

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
			if val == "" {
				// empty -> skip? default to true per above behavior
			} else if val == "1" || val == "true" || val == "yes" || val == "y" {
				ret = true
			} else if val == "0" || val == "false" || val == "no" || val == "n" {
				ret = false
			} else {
				// try parse int fallback
				ret = val != "0"
			}
		}
		if ret { // only include truthy rows for marking retired
			out = append(out, row{Name: name, Retired: true})
		}
	}
	return out, nil
}

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
			q := fmt.Sprintf(`UPDATE player SET is_retired = 0 WHERE lower(player_name) NOT IN (%s) AND is_retired IS DISTINCT FROM 0`, strings.Join(placeholders, ","))
			ct, err := db.Pool.Exec(ctx, q, args...)
			if err != nil {
				return changed, zeroed, err
			}
			zeroed = ct.RowsAffected()
		}
	}
	return changed, zeroed, nil
}

func main() {
	var (
		file       string
		dryRun     bool
		apply      bool
		othersZero bool
		timeout    time.Duration
	)
	flag.StringVar(&file, "file", "", "CSV file path (required)")
	flag.BoolVar(&dryRun, "dry-run", true, "print intended changes without writing to DB")
	flag.BoolVar(&apply, "apply", false, "apply changes (overrides --dry-run)")
	flag.BoolVar(&othersZero, "others-zero", false, "set is_retired=0 for players not listed in CSV")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "operation timeout")
	flag.Parse()

	if file == "" {
		log.Fatal("--file is required")
	}

	rows, err := parseCSV(file)
	if err != nil {
		log.Fatalf("parse csv: %v", err)
	}
	unique := map[string]struct{}{}
	var names []string
	for _, r := range rows {
		n := strings.TrimSpace(r.Name)
		if n == "" {
			continue
		}
		if _, ok := unique[strings.ToLower(n)]; ok {
			continue
		}
		unique[strings.ToLower(n)] = struct{}{}
		names = append(names, n)
	}

	if !apply {
		// dry-run always, unless --apply
		fmt.Printf("[DRY-RUN] Would mark %d players as retired. others-zero=%v\n", len(names), othersZero)
		for _, n := range names {
			fmt.Printf("  - %s\n", n)
		}
		if othersZero {
			fmt.Println("[DRY-RUN] Would set is_retired=0 for all other players not listed")
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	// Optional: run migrations to ensure schema
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "/migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	changed, zeroed, err := applyRetired(ctx, names, othersZero)
	if err != nil {
		log.Fatalf("apply failed: %v", err)
	}
	fmt.Printf("Applied. Marked retired: %d, zeroed others: %d\n", changed, zeroed)
}