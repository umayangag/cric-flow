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
		runs, err := atoi(row[indexOf(h, "runs")])
		if err != nil {
			return nil, fmt.Errorf("row %d runs: %w", i, err)
		}
		balls, err := atoi(row[indexOf(h, "balls")])
		if err != nil {
			return nil, fmt.Errorf("row %d balls: %w", i, err)
		}
		fours, err := atoi(row[indexOf(h, "fours")])
		if err != nil {
			return nil, fmt.Errorf("row %d fours: %w", i, err)
		}
		sixes, err := atoi(row[indexOf(h, "sixes")])
		if err != nil {
			return nil, fmt.Errorf("row %d sixes: %w", i, err)
		}
		pos, err := atoi(row[indexOf(h, "position")])
		if err != nil {
			return nil, fmt.Errorf("row %d position: %w", i, err)
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
		overse, err := atof(row[indexOf(h, "overs")])
		if err != nil {
			return nil, fmt.Errorf("row %d overs: %w", i, err)
		}
		balls, err := atoi(row[indexOf(h, "balls")])
		if err != nil {
			return nil, fmt.Errorf("row %d balls: %w", i, err)
		}
		maidens, err := atoi(row[indexOf(h, "maidens")])
		if err != nil {
			return nil, fmt.Errorf("row %d maidens: %w", i, err)
		}
		runs, err := atoi(row[indexOf(h, "runs")])
		if err != nil {
			return nil, fmt.Errorf("row %d runs: %w", i, err)
		}
		wickets, err := atoi(row[indexOf(h, "wickets")])
		if err != nil {
			return nil, fmt.Errorf("row %d wickets: %w", i, err)
		}
		econ, err := atof(row[indexOf(h, "economy")])
		if err != nil {
			return nil, fmt.Errorf("row %d economy: %w", i, err)
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
