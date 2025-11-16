package seqcalc

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func TestAggregateTransitions_SimpleInnings(t *testing.T) {
	// One innings with opener change and a wicket causing a change
	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	seq := []bevent{
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    1,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 101, Valid: true},
			RunsBatter: 0,
			RunsTotal:  0,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    2,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 101, Valid: true},
			RunsBatter: 1,
			RunsTotal:  1,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
		// striker change after over strike rotation (simulate change): 101 -> 102
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    3,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 102, Valid: true},
			RunsBatter: 0,
			RunsTotal:  0,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    4,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 102, Valid: true},
			RunsBatter: 4,
			RunsTotal:  4,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
		// wicket: striker 102 out, new batter 103 next ball
		{
			MatchID:     1,
			Innings:     1,
			BallSeq:     5,
			Phase:       "powerplay",
			StrikerID:   sql.NullInt64{Int64: 102, Valid: true},
			RunsBatter:  0,
			RunsTotal:   0,
			WicketKind:  sql.NullString{String: "bowled", Valid: true},
			PlayerOutID: sql.NullInt64{Int64: 102, Valid: true},
			AsOf:        asOf,
			FormatID:    fmtID,
		},
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    6,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 103, Valid: true},
			RunsBatter: 6,
			RunsTotal:  6,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
	}
	rows := aggregateTransitions(seq)
	if len(rows) != 2 {
		// Expected transitions: 101->102, then 102->103
		t.Fatalf("expected 2 transition rows, got %d: %#v", len(rows), rows)
	}
	// build a map for assertions
	key := func(prev, bat int64) string { return fmt.Sprintf("%d->%d", prev, bat) }
	m := map[string]db.BatTransitionRow{}
	for _, r := range rows {
		m[key(r.PrevBatterID, r.BatterID)] = r
	}
	if got := m[key(101, 102)]; true {
		if got.Balls != 3 {
			t.Fatalf("101->102 Balls unexpected: %+v", got)
		}
		if got.Runs != 4 {
			t.Fatalf("101->102 Runs unexpected: %+v", got)
		}
		if got.Fours != 1 {
			t.Fatalf("101->102 Fours unexpected: %+v", got)
		}
		if got.Dismissals != 1 {
			t.Fatalf("101->102 Dismissals unexpected: %+v", got)
		}
	}
	if got := m[key(102, 103)]; true {
		if got.Balls != 1 {
			t.Fatalf("102->103 Balls unexpected: %+v", got)
		}
		if got.Dismissals != 0 {
			t.Fatalf("102->103 Dismissals unexpected: %+v", got)
		}
		if got.Sixes != 1 {
			t.Fatalf("102->103 Sixes unexpected: %+v", got)
		}
		if got.Runs != 6 {
			t.Fatalf("102->103 Runs unexpected: %+v", got)
		}
	}
}

func TestAggregateTransitions_NoAsOfSkips(t *testing.T) {
	// Ensure that missing striker_id rows are ignored and do not break aggregation.
	asOf := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	fmtID := 3
	seq := []bevent{
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    1,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Valid: false},
			RunsBatter: 0,
			RunsTotal:  0,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    2,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 201, Valid: true},
			RunsBatter: 0,
			RunsTotal:  0,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    3,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 202, Valid: true},
			RunsBatter: 1,
			RunsTotal:  1,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
		{
			MatchID:    1,
			Innings:    1,
			BallSeq:    4,
			Phase:      "powerplay",
			StrikerID:  sql.NullInt64{Int64: 202, Valid: true},
			RunsBatter: 6,
			RunsTotal:  6,
			AsOf:       asOf,
			FormatID:   fmtID,
		},
	}
	rows := aggregateTransitions(seq)
	if len(rows) != 1 {
		t.Fatalf("expected 1 transition row, got %d", len(rows))
	}
	if rows[0].PrevBatterID != 201 || rows[0].BatterID != 202 || rows[0].Runs != 7 || rows[0].Fours != 0 ||
		rows[0].Sixes != 1 {
		t.Fatalf("unexpected aggregation: %+v", rows[0])
	}
}
