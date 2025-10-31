package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/umayangag/cric-app/go-app/internal/cricsheet"
	"github.com/umayangag/cric-app/go-app/internal/db"
)

func main() {
	var (
		dataDir   = flag.String("dir", "../data", "Directory containing Cricsheet .json files")
		phWeather = flag.Bool("placeholders-weather", false, "Insert placeholder weather rows per match")
		phField   = flag.Bool("placeholders-fielding", false, "Insert zeroed fielding rows for all players seen")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	// Apply migrations to ensure schema is ready
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	opts := &cricsheet.Options{PlaceholdersWeather: *phWeather, PlaceholdersFielding: *phField}
	n, err := cricsheet.ImportDir(ctx, *dataDir, opts)
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}
 log.Printf("cricsheet-importer finished: %d files imported", n)
}
/* stray legacy code removed
        inningNo := i + 1
		batTeam := strings.TrimSpace(inng.Team)
		oppTeam := otherTeam(batTeam, teamA, teamB)
		var oppositionID *int64
		if oppTeam != "" {
			if id, e := db.GetOrCreateOpposition(ctx, oppTeam); e == nil {
				oppositionID = &id
			}
		}

		// Totals
		runs, wkts, balls, extras := 0, 0, 0, 0
		// Track per-batter aggregates and dismissals
		batAgg := map[string]*batRow{}
		dismissals := map[string]string{}
		// Track per-bowler aggregates and over totals for maidens
		bowlAgg := map[string]*bowlRow{}
		overTotalsByBowler := map[string]map[int]int{}

		for _, over := range inng.Overs {
			overNo := over.Over
			perBowler := map[string]int{}
			for _, d := range over.Deliveries {
				tr := d.Runs.Total
				runs += tr
				extras += d.Runs.Extras
				wides := d.Extras["wides"]
				noballs := d.Extras["noballs"]
				legal := (wides == 0 && noballs == 0)
				if legal {
					balls++
					perBowler[d.Bowler] += tr
				}
				if len(d.Wickets) > 0 {
					wkts += len(d.Wickets)
					for _, w := range d.Wickets {
						desc := w.Kind
						if len(w.Fielders) > 0 {
							desc = desc + " " + strings.Join(w.Fielders, ", ")
						}
						dismissals[w.PlayerOut] = strings.TrimSpace(desc)
					}
				}

				// batting
				if d.Batter != "" {
					br := d.Runs.Batter
					b := ensureBat(batAgg, d.Batter)
					b.Runs += br
					if legal {
						b.Balls++
					}
					if br == 4 {
						b.Fours++
					}
					if br == 6 {
						b.Sixes++
					}
				}
				if d.NonStriker != "" {
					_ = ensureBat(batAgg, d.NonStriker)
				}
				// bowling
				if d.Bowler != "" {
					b := ensureBowl(bowlAgg, d.Bowler)
					b.Runs += tr
					if legal {
						b.Balls++
						if tr == 0 {
							b.Dots++
						}
						if d.Runs.Batter == 4 {
							b.Fours++
						}
						if d.Runs.Batter == 6 {
							b.Sixes++
						}
					}
					b.Wickets += len(d.Wickets)
					b.Wides += d.Extras["wides"]
					b.NoBalls += d.Extras["noballs"]
				}
			}
			for bowler, t := range perBowler {
				if overTotalsByBowler[bowler] == nil { overTotalsByBowler[bowler] = map[int]int{} }
				overTotalsByBowler[bowler][overNo] += t
			}
		}

		// compute and upsert match_details for this innings perspective
		oversFloat := oversFromBalls(balls, ballsPerOver)
		rpo := float32(0)
		if balls > 0 { rpo = float32(float64(runs) / float64(balls) * float64(ballsPerOver)) }
		target := (*int)(nil)
		if inningNo == 2 && len(m.Innings) >= 1 {
			firRuns := inningsRuns(m.Innings[0])
			target = &firRuns
		}
		upd := &db.MatchInfoUpdate{
			Score:          &runs,
			Wickets:        &wkts,
			Overs:          &oversFloat,
			Balls:          &balls,
			RPO:            &rpo,
			Target:         target,
			Inning:         &inningNo,
			OppositionID:   oppositionID,
			Date:           &dateISO,
			VenueID:        venueID,
			Extras:         &extras,
			Toss:           &toss,
			SeasonID:       seasonID,
			MatchNumber:    matchNumber,
		}
		if err := db.UpdateMatchDetails(ctx, mid, upd); err != nil {
			log.Printf("warn: update match_details failed for match_id=%d: %v", mid, err)
		}

		// upsert batting rows (batting order inferred by first appearance)
		order := make([]string, 0, len(batAgg))
		for name, b := range batAgg { b.Name = name; order = append(order, name); _ = b }
		// preserve appearance by re-walking deliveries order in JSON
		order = battingOrderFromInnings(inng, order)
		pos := 1
		for _, name := range order {
			b := batAgg[name]
			pid, _ := db.GetOrCreateByName(ctx, name)
			sr := strikeRate(b.Runs, b.Balls)
			desc := dismissals[name]
			if desc == "" { desc = "not out" }
			mins := 0
			if err := db.UpsertBatting(ctx, &db.Batting{
				MatchID:         mid,
				PlayerID:        pid,
				Description:     strPtr(desc),
				Runs:            &b.Runs,
				Balls:           &b.Balls,
				Minutes:         &mins,
				Fours:           &b.Fours,
				Sixes:           &b.Sixes,
				StrikeRate:      &sr,
				BattingPosition: &pos,
			}); err != nil {
				log.Printf("warn: upsert batting failed: %v", err)
			}
			pos++
		}

		// finalize maiden overs and econ and upsert bowling
		for name, s := range bowlAgg {
			maidens := maidenCount(overTotalsByBowler[name])
			o := oversFromBalls(s.Balls, ballsPerOver)
			econ := float32(0)
			if s.Balls > 0 { econ = float32(float64(s.Runs) / float64(s.Balls) * float64(ballsPerOver)) }
			pid, _ := db.GetOrCreateByName(ctx, name)
			if err := db.UpsertBowling(ctx, &db.Bowling{
				MatchID:  mid,
				PlayerID: pid,
				Overs:    &o,
				Balls:    &s.Balls,
				Maidens:  &maidens,
				Runs:     &s.Runs,
				Wickets:  &s.Wickets,
				Dots:     &s.Dots,
				Fours:    &s.Fours,
				Sixes:    &s.Sixes,
				Econ:     &econ,
				Wides:    &s.Wides,
				NoBalls:  &s.NoBalls,
			}); err != nil {
				log.Printf("warn: upsert bowling failed: %v", err)
			}
		}
	}

	return nil
}

type batRow struct {
	Name        string
	Runs        int
	Balls       int
	Fours       int
	Sixes       int
}

type bowlRow struct {
	Balls   int
	Maidens int
	Runs    int
	Wickets int
	Dots    int
	Fours   int
	Sixes   int
	Wides   int
	NoBalls int
}

func ensureBat(m map[string]*batRow, name string) *batRow {
	if v, ok := m[name]; ok { return v }
	br := &batRow{}
	m[name] = br
	return br
}

func ensureBowl(m map[string]*bowlRow, name string) *bowlRow {
	if v, ok := m[name]; ok { return v }
	br := &bowlRow{}
	m[name] = br
	return br
}

func inningRunsBalls(inng cricsheet.Innings) (runs int, balls int, extras int) {
	for _, over := range inng.Overs {
		for _, d := range over.Deliveries {
			runs += d.Runs.Total
			extras += d.Runs.Extras
			wides := d.Extras["wides"]
			noballs := d.Extras["noballs"]
			if wides == 0 && noballs == 0 { balls++ }
		}
	}
	return
}

func inningsRuns(inng cricsheet.Innings) int {
	r, _, _ := inningRunsBalls(inng)
	return r
}

func oversFromBalls(balls int, bpo int) float32 {
	if bpo <= 0 { bpo = 6 }
	ov := balls / bpo
	rem := balls % bpo
	return float32(float64(ov) + float64(rem)/10.0)
}

func maidenCount(overMap map[int]int) int {
	c := 0
	for _, t := range overMap { if t == 0 { c++ } }
	return c
}

func strikeRate(runs, balls int) float32 {
	if balls <= 0 { return 0 }
	return float32(float64(runs) / float64(balls) * 100.0)
}

func strPtr(s string) *string { return &s }

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" { return v }
	}
	return ""
}

func otherTeam(bat string, a, b string) string {
	if bat == a { return b }
	if bat == b { return a }
	// fallback: if unknown, return the one that is not empty
	if bat == "" { return strings.TrimSpace(firstNonEmpty(a, b)) }
	return ""
}

*/