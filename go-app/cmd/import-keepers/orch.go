package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// KeeperRow represents a single CSV row.
type KeeperRow struct {
	Name  string
	Value int // 0 or 1
}

// parseCSV reads the keepers CSV file.
func parseCSV(path string) ([]KeeperRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close csv file failed", slog.String("path", path), slog.Any("err", err))
		}
	}()
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
			// header
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
				if n, err := strconv.Atoi(v); err == nil {
					if n != 0 {
						val = 1
					} else {
						val = 0
					}
				} else {
					lv := strings.ToLower(v)
					if lv == "true" || lv == "yes" || lv == "y" {
						val = 1
					}
					if lv == "false" || lv == "no" || lv == "n" {
						val = 0
					}
				}
			}
		}
		rows = append(rows, KeeperRow{Name: name, Value: val})
	}
	return rows, nil
}

// Runner orchestrates the import/preview flow.
type Runner struct{}

func (Runner) Preview(ctx context.Context, targets map[string]int, othersZero bool) error {
	matched := 0
	for name, v := range targets {
		q := `SELECT COUNT(1) FROM player WHERE lower(player_name) = $1`
		var cnt int64
		if err := db.Pool.QueryRow(ctx, q, name).Scan(&cnt); err != nil {
			return err
		}
		if cnt == 0 {
			slog.Warn("no player matched for name", slog.String("name", name))
		} else {
			matched += int(cnt)
			slog.Info("PLAN: set is_wicket_keeper", slog.Int("value", v), slog.Int64("rows", cnt), slog.String("name", name))
		}
	}
	if othersZero {
		slog.Info("PLAN: set is_wicket_keeper=0 for players NOT in provided CSV")
	}
	slog.Info("dry-run summary", slog.Int("targets", len(targets)), slog.Int("matched", matched))
	return nil
}

func (Runner) Apply(ctx context.Context, targets map[string]int, othersZero bool) error {
	upd := `UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2`
	for name, v := range targets {
		if _, err := db.Pool.Exec(ctx, upd, v, name); err != nil {
			return fmt.Errorf("update keeper for '%s': %w", name, err)
		}
	}
	if othersZero {
		// Build NOT IN list of parameters. For very large sets, a temporary table approach may be more performant.
		placeholders := make([]string, 0, len(targets))
		args := make([]any, 0, len(targets))
		i := 1
		for name := range targets {
			placeholders = append(placeholders, fmt.Sprintf("$%d", i))
			args = append(args, name)
			i++
		}
		q := fmt.Sprintf(
			"UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN (%s)",
			strings.Join(placeholders, ","),
		)
		if _, err := db.Pool.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("zero others: %w", err)
		}
	}
	return nil
}
