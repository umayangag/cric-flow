package etlimporter

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// ParseBattingCSV parses a curated batting CSV into EtlBattingRow values.
// Expected header (case-insensitive, trimmed):
// player_name,season,format,runs,balls,fours,sixes,position
func ParseBattingCSV(r io.Reader) ([]db.EtlBattingRow, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("empty csv")
	}
	h := normalizeHeader(recs[0])
	need := []string{"player_name", "season", "format", "runs", "balls", "fours", "sixes", "position"}
	if !hasHeader(h, need) {
		return nil, fmt.Errorf("unexpected batting header: %v", h)
	}
	var out []db.EtlBattingRow
	for i := 1; i < len(recs); i++ {
		row := recs[i]
		if len(row) < len(h) {
			return nil, fmt.Errorf("row %d: wrong column count", i)
		}
		runs, err := getInt(row, h, "runs", i)
		if err != nil {
			return nil, err
		}
		balls, err := getInt(row, h, "balls", i)
		if err != nil {
			return nil, err
		}
		fours, err := getInt(row, h, "fours", i)
		if err != nil {
			return nil, err
		}
		sixes, err := getInt(row, h, "sixes", i)
		if err != nil {
			return nil, err
		}
		pos, err := getInt(row, h, "position", i)
		if err != nil {
			return nil, err
		}
		out = append(out, db.EtlBattingRow{
			PlayerName: strings.TrimSpace(row[indexOf(h, "player_name")]),
			Season:     strings.TrimSpace(row[indexOf(h, "season")]),
			Format:     strings.ToUpper(strings.TrimSpace(row[indexOf(h, "format")])),
			Runs:       runs,
			Balls:      balls,
			Fours:      fours,
			Sixes:      sixes,
			Position:   pos,
		})
	}
	return out, nil
}

// ParseBowlingCSV parses a curated bowling CSV into EtlBowlingRow values.
// Expected header (case-insensitive):
// player_name,season,format,overs,balls,maidens,runs,wickets,economy
func ParseBowlingCSV(r io.Reader) ([]db.EtlBowlingRow, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("empty csv")
	}
	h := normalizeHeader(recs[0])
	need := []string{"player_name", "season", "format", "overs", "balls", "maidens", "runs", "wickets", "economy"}
	if !hasHeader(h, need) {
		return nil, fmt.Errorf("unexpected bowling header: %v", h)
	}
	var out []db.EtlBowlingRow
	for i := 1; i < len(recs); i++ {
		row := recs[i]
		if len(row) < len(h) {
			return nil, fmt.Errorf("row %d: wrong column count", i)
		}
		overse, err := getFloat(row, h, "overs", i)
		if err != nil {
			return nil, err
		}
		balls, err := getInt(row, h, "balls", i)
		if err != nil {
			return nil, err
		}
		maidens, err := getInt(row, h, "maidens", i)
		if err != nil {
			return nil, err
		}
		runs, err := getInt(row, h, "runs", i)
		if err != nil {
			return nil, err
		}
		wickets, err := getInt(row, h, "wickets", i)
		if err != nil {
			return nil, err
		}
		econ, err := getFloat(row, h, "economy", i)
		if err != nil {
			return nil, err
		}
		out = append(out, db.EtlBowlingRow{
			PlayerName: strings.TrimSpace(row[indexOf(h, "player_name")]),
			Season:     strings.TrimSpace(row[indexOf(h, "season")]),
			Format:     strings.ToUpper(strings.TrimSpace(row[indexOf(h, "format")])),
			Overs:      overse,
			Balls:      balls,
			Maidens:    maidens,
			Runs:       runs,
			Wickets:    wickets,
			Economy:    econ,
		})
	}
	return out, nil
}

func normalizeHeader(h []string) []string {
	out := make([]string, len(h))
	for i, v := range h {
		out[i] = strings.ToLower(strings.TrimSpace(v))
	}
	return out
}

func hasHeader(h []string, need []string) bool {
	for _, n := range need {
		if indexOf(h, n) < 0 {
			return false
		}
	}
	return true
}

func indexOf(h []string, name string) int {
	for i, v := range h {
		if v == name {
			return i
		}
	}
	return -1
}

func atoi(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func atof(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

// getInt fetches and parses the integer value for the given column, wrapping any parse error
// with a consistent "row N <col>: <err>" message used by tests.
func getInt(row, header []string, colName string, rowNum int) (int, error) {
	idx := indexOf(header, colName)
	v, err := atoi(row[idx])
	if err != nil {
		return 0, fmt.Errorf("row %d %s: %w", rowNum, colName, err)
	}
	return v, nil
}

// getFloat fetches and parses the float value for the given column, wrapping any parse error
// with a consistent "row N <col>: <err>" message used by tests.
func getFloat(row, header []string, colName string, rowNum int) (float64, error) {
	idx := indexOf(header, colName)
	v, err := atof(row[idx])
	if err != nil {
		return 0, fmt.Errorf("row %d %s: %w", rowNum, colName, err)
	}
	return v, nil
}
