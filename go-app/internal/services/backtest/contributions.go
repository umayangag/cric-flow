package backtest

import (
	"encoding/csv"
	"os"
	"strconv"

	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// BuildContributionRows builds contribution rows from player results for CSV export.
// keeperMap maps player_id -> true for wicketkeepers.
func BuildContributionRows(
	players []PlayerResult,
	keeperMap map[int64]bool,
	format string,
	batDiv, wicketDiv, econBase, fieldDiv float64,
) []ContributionRow {
	out := make([]ContributionRow, 0, len(players))
	for _, p := range players {
		pred := p.Predicted
		act := p.Actual
		if pred == nil || act == nil {
			continue
		}
		runs := pred["runs"]
		wickets := pred["wickets"]
		econ := pred["economy"]
		catches := pred["catches"]
		runOuts := pred["run_outs"]
		actualRuns := act["runs"]

		batScore := teamselect.NormalizeBatScore(runs, batDiv)
		bowlScore := teamselect.NormalizeBowlScore(wickets, econ, wicketDiv, econBase)
		fieldScore := teamselect.NormalizeFieldScore(catches, runOuts, fieldDiv)

		isKeeper := 0
		if keeperMap[p.PlayerID] {
			isKeeper = 1
		}
		target := 0.0
		if batDiv > 0 {
			target = actualRuns / batDiv
		}
		out = append(out, ContributionRow{
			BatScore:   batScore,
			BowlScore:  bowlScore,
			FieldScore: fieldScore,
			IsKeeper:   isKeeper,
			Format:     format,
			Target:     target,
		})
	}
	return out
}

// WriteContributionsCSV writes contribution rows to a CSV file at the given path.
func WriteContributionsCSV(path string, rows []ContributionRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	header := []string{"bat_score", "bowl_score", "field_score", "is_keeper", "format", "target"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		record := []string{
			strconv.FormatFloat(r.BatScore, 'f', -1, 64),
			strconv.FormatFloat(r.BowlScore, 'f', -1, 64),
			strconv.FormatFloat(r.FieldScore, 'f', -1, 64),
			strconv.Itoa(r.IsKeeper),
			r.Format,
			strconv.FormatFloat(r.Target, 'f', -1, 64),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
