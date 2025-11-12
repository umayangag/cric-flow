package csvx

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strconv"
	"strings"
)

// KeeperRow represents a single keepers CSV row.
type KeeperRow struct {
	Name  string
	Value int // 0 or 1
}

// ParseKeepersCSV parses the given CSV file from fsys at path and returns rows.
// Behavior mirrors the previous cmd/import-keepers implementation:
// - Trims spaces; skips header row and empty names
// - Interprets second column as int or boolean-like string; defaults to 1
func ParseKeepersCSV(fsys fs.FS, path string) ([]KeeperRow, error) {
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

	reader := csv.NewReader(bufio.NewReader(f))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	var rows []KeeperRow
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
