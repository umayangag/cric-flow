package teamselect

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// LoadFromCSV parses a simple CSV pool with header:
// name,is_bowler,is_keeper,bat_score,bowl_score
// Values are trimmed; booleans accept 1/0,true/false,yes/no (case-insensitive).
func LoadFromCSV(r io.Reader) ([]Player, error) {
	if r == nil {
		return nil, errors.New("nil reader")
	}
	cr := csv.NewReader(bufio.NewReader(r))
	cr.TrimLeadingSpace = true
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, errors.New("empty csv")
	}
	h := normalizeHeader(recs[0])
	need := []string{"name", "is_bowler", "is_keeper", "bat_score", "bowl_score"}
	if !hasHeader(h, need) {
		return nil, errors.New("unexpected header")
	}
	var out []Player
	for i := 1; i < len(recs); i++ {
		row := recs[i]
		if len(row) < len(h) {
			return nil, fieldErr(i, "columns")
		}
		name := strings.TrimSpace(row[idx(h, "name")])
		if name == "" {
			return nil, fieldErr(i, "name")
		}
		isBow := parseBool(row[idx(h, "is_bowler")])
		isKeep := parseBool(row[idx(h, "is_keeper")])
		bat, err := parseFloat(row[idx(h, "bat_score")])
		if err != nil {
			return nil, fieldErr(i, "bat_score")
		}
		bowl, err := parseFloat(row[idx(h, "bowl_score")])
		if err != nil {
			return nil, fieldErr(i, "bowl_score")
		}
		out = append(out, Player{Name: name, IsBowler: isBow, IsKeeper: isKeep, BatScore: bat, BowlScore: bowl})
	}
	return out, nil
}

// TeamSelectRepo is consumed via the db package; LoadFromDB adapts its DTOs to Player.
func LoadFromDB(ctx context.Context, repo db.TeamSelectRepo, matchID int64, format, season string) ([]Player, error) {
	if repo == nil {
		return nil, errors.New("nil repo")
	}
	if matchID <= 0 || strings.TrimSpace(format) == "" || strings.TrimSpace(season) == "" {
		return nil, errors.New("invalid args")
	}
	dtos, err := repo.LoadPool(ctx, matchID, format, season)
	if err != nil {
		return nil, err
	}
	out := make([]Player, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, Player{
			Name: d.Name, IsBowler: d.IsBowler, IsKeeper: d.IsKeeper,
			BatScore: d.BatScore, BowlScore: d.BowlScore,
		})
	}
	return out, nil
}

// --- helpers ---

func normalizeHeader(h []string) []string {
	out := make([]string, len(h))
	for i, v := range h {
		out[i] = strings.ToLower(strings.TrimSpace(v))
	}
	return out
}

func hasHeader(h []string, need []string) bool {
	for _, n := range need {
		if idx(h, n) < 0 {
			return false
		}
	}
	return true
}

func idx(h []string, name string) int {
	for i, v := range h {
		if v == name {
			return i
		}
	}
	return -1
}

func parseBool(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	return v == "1" || v == "true" || v == "yes"
}

func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

func fieldErr(i int, col string) error {
	return errors.New("row " + strconv.Itoa(i) + ": invalid " + col)
}
