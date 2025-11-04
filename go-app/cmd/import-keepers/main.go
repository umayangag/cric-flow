// Command import-keepers updates the is_wicket_keeper flag for players from a CSV.
//
// CSV schema (minimal):
//   Name[,IsWicketKeeper]
// - Name: player name string (matched case-insensitively against player.player_name)
// - IsWicketKeeper: optional boolean/int; if omitted, defaults to 1 for all listed names
//
// Usage examples:
//   go run ./go-app/cmd/import-keepers --file ./data/keepers.csv --dry-run
//   go run ./go-app/cmd/import-keepers --file ./data/keepers.csv --apply
//   go run ./go-app/cmd/import-keepers --file ./data/keepers.csv --apply --others-zero
//
// Notes:
// - By default, this tool runs in dry-run mode. Use --apply to persist changes.
// - With --others-zero, any player not present in the CSV will be set to 0.
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

type KeeperRow struct {
	Name  string
	Value int // 0 or 1
}

func parseCSV(path string) ([]KeeperRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(bufio.NewReader(f))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	rows := []KeeperRow{}
	line := 0
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv read error at line %d: %w", line, err)
		}
		line++
		if line == 1 {
			// Header detection: must contain Name
			// We allow either [Name] or [Name,IsWicketKeeper]
			continue
		}
		if len(rec) == 0 {
			continue
		}
		name := strings.TrimSpace(rec[0])
		if name == "" || strings.EqualFold(name, "Name") {
			continue
		}
		val := 1
		if len(rec) > 1 {
			v := strings.TrimSpace(rec[1])
			if v != "" {
				// accept 1/0/true/false/yes/no/y/n
				if n, err := strconv.Atoi(v); err == nil {
					if n != 0 {
						val = 1
					} else {
						val = 0
					}
				} else {
					lv := strings.ToLower(v)
					if lv == "true" || lv == "yes" || lv == "y" { val = 1 }
					if lv == "false" || lv == "no" || lv == "n" { val = 0 }
				}
			}
		}
		rows = append(rows, KeeperRow{Name: name, Value: val})
	}
	return rows, nil
}

func main() {
	var file string
	var apply bool
	var othersZero bool
	flag.StringVar(&file, "file", "", "path to CSV file with keepers")
	flag.BoolVar(&apply, "apply", false, "apply changes (default is dry-run)")
	flag.BoolVar(&othersZero, "others-zero", false, "set is_wicket_keeper=0 for players not in CSV")
	flag.Parse()

	if strings.TrimSpace(file) == "" {
		log.Fatalf("--file is required")
	}

	rows, err := parseCSV(file)
	if err != nil {
		log.Fatalf("parse CSV: %v", err)
	}
	log.Printf("parsed %d rows from %s", len(rows), file)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	// Build a set of target names (lowercased)
	targets := make(map[string]int, len(rows))
	for _, r := range rows {
		targets[strings.ToLower(r.Name)] = r.Value
	}

	// Preview changes
	preview := func() error {
		// Count matches and potential misses
		matched := 0
		for name, v := range targets {
			q := `SELECT COUNT(1) FROM player WHERE lower(player_name) = $1`
			var cnt int64
			if err := db.Pool.QueryRow(ctx, q, name).Scan(&cnt); err != nil {
				return err
			}
			if cnt == 0 {
				log.Printf("WARN: no player matched for name '%s'", name)
			} else {
				matched += int(cnt)
				log.Printf("PLAN: set is_wicket_keeper=%d for %d row(s) name='%s'", v, cnt, name)
			}
		}
		if othersZero {
			log.Printf("PLAN: set is_wicket_keeper=0 for players NOT in provided CSV")
		}
		log.Printf("dry-run summary: targets=%d matched=%d", len(targets), matched)
		return nil
	}

	applyChanges := func() error {
		// Apply target updates
		upd := `UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2`
		for name, v := range targets {
			if _, err := db.Pool.Exec(ctx, upd, v, name); err != nil {
				return fmt.Errorf("update keeper for '%s': %w", name, err)
			}
		}
		if othersZero {
			// Set others to 0 (exclude listed names)
			// Use a temp table approach for large sets could be better; here use NOT IN for simplicity
			placeholders := make([]string, 0, len(targets))
			args := make([]any, 0, len(targets))
			i := 1
			for name := range targets {
				placeholders = append(placeholders, fmt.Sprintf("$%d", i))
				args = append(args, name)
				i++
			}
			q := "UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN (" + strings.Join(placeholders, ",") + ")"
			if _, err := db.Pool.Exec(ctx, q, args...); err != nil {
				return fmt.Errorf("zero others: %w", err)
			}
		}
		return nil
	}

	if !apply {
		if err := preview(); err != nil {
			log.Fatalf("dry-run failed: %v", err)
		}
		log.Printf("dry-run complete. Re-run with --apply to persist changes.")
		return
	}
	if err := applyChanges(); err != nil {
		log.Fatalf("apply failed: %v", err)
	}
	log.Printf("apply complete.")
}
