package cricsheet

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Options controls optional behaviors for Cricsheet import.
type Options struct {
	PlaceholdersWeather  bool
	PlaceholdersFielding bool
	WeatherEnqueue       bool
}

// ImportDir reads all .json files in dir and imports them into the DB.
func ImportDir(ctx context.Context, dir string, opts *Options) (int, error) {
	if opts == nil {
		opts = &Options{}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".json") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	count := 0
	for _, f := range files {
		if err := ImportMatchFile(ctx, f, opts); err != nil {
			log.Printf("warn: import %s failed: %v", filepath.Base(f), err)
			continue
		}
		count++
		fmt.Printf("import %s\n", filepath.Base(f))
	}
	return count, nil
}

// ImportMatchFile parses a single Cricsheet JSON file and upserts stats into DB.
func ImportMatchFile(ctx context.Context, path string, opts *Options) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	m, err := Parse(fh)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	info := m.Info
	dateISO := "1970-01-01"
	if len(info.Dates) > 0 {
		dateISO = info.Dates[0]
	}
	teamA, teamB := "Team A", "Team B"
	if len(info.Teams) >= 1 {
		teamA = info.Teams[0]
	}
	if len(info.Teams) >= 2 {
		teamB = info.Teams[1]
	}
	mid := StableMatchID(dateISO, teamA, teamB)
	cfg := config.Load()
	formatCode := DetectFormat(info.MatchType, info.Teams, cfg)
	if formatCode == "" {
		return fmt.Errorf("unsupported match_type: %s", info.MatchType)
	}
	formatID, err := cricDB.GetMatchFormatIDByCode(ctx, formatCode)
	if err != nil {
		return fmt.Errorf("lookup format_id for %s: %w", formatCode, err)
	}
	if err := cricDB.EnsureMatchWithFormat(ctx, mid, formatID); err != nil {
		return fmt.Errorf("ensure match with format: %w", err)
	}
	venueName := strings.TrimSpace(firstNonEmpty(info.Venue, info.City))
	var venueID *int64
	if venueName != "" {
		if id, e := cricDB.GetOrCreateVenue(ctx, venueName); e == nil {
			venueID = &id
		}
	}
	var seasonID *int64
	if s := strings.TrimSpace(string(info.Season)); s != "" {
		if id, e := cricDB.GetOrCreateSeason(ctx, s); e == nil {
			seasonID = &id
		}
	}
	matchNumber := (*int)(nil)
	if info.Event != nil && info.Event.MatchNumber != nil {
		matchNumber = info.Event.MatchNumber
	}
	toss := ""
	if info.Toss != nil {
		toss = info.Toss.Winner
	}
	winner := ""
	if info.Outcome != nil {
		winner = info.Outcome.Winner
	}
	_ = winner // currently unused, but kept for possible result mapping

	ballsPerOver := info.BallsPerOver
	if ballsPerOver <= 0 {
		ballsPerOver = 6
	}
	// Optional: insert placeholder weather rows once per match
	if opts != nil && opts.PlaceholdersWeather {
		_ = cricDB.Exec(
			ctx,
			`INSERT INTO weather_data(match_id, session) VALUES ($1,$2) ON CONFLICT (match_id, session) DO NOTHING`,
			mid,
			"inning1",
		)
		_ = cricDB.Exec(
			ctx,
			`INSERT INTO weather_data(match_id, session) VALUES ($1,$2) ON CONFLICT (match_id, session) DO NOTHING`,
			mid,
			"inning2",
		)
	}
	playersSeen := map[string]bool{}
	for i, inng := range m.Innings {
		inningNo := i + 1
		batTeam := strings.TrimSpace(inng.Team)
		oppTeam := otherTeam(batTeam, teamA, teamB)
		var oppositionID *int64
		if oppTeam != "" {
			if id, e := cricDB.GetOrCreateOpposition(ctx, oppTeam); e == nil {
				oppositionID = &id
			}
		}
		// totals and aggregates
		runs, wkts, balls, extras := 0, 0, 0, 0
		batAgg := map[string]*batRow{}
		dismissals := map[string]string{}
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
				if d.Wickets != nil && len(*d.Wickets) > 0 {
					wkts += len(*d.Wickets)
					for _, w := range *d.Wickets {
						desc := w.Kind
						if w.Fielders != nil && len(*w.Fielders) > 0 {
							desc = desc + " " + strings.Join(*w.Fielders, ", ")
						}
						dismissals[w.PlayerOut] = strings.TrimSpace(desc)
					}
				}
				if d.Batter != "" {
					playersSeen[d.Batter] = true
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
					playersSeen[d.NonStriker] = true
					_ = ensureBat(batAgg, d.NonStriker)
				}
				if d.Bowler != "" {
					playersSeen[d.Bowler] = true
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
					if d.Wickets != nil && len(*d.Wickets) > 0 {
						b.Wickets += len(*d.Wickets)
					}
					b.Wides += d.Extras["wides"]
					b.NoBalls += d.Extras["noballs"]
				}
			}
			for bowler, t := range perBowler {
				if overTotalsByBowler[bowler] == nil {
					overTotalsByBowler[bowler] = map[int]int{}
				}
				overTotalsByBowler[bowler][overNo] += t
			}
		}
		oversFloat := oversFromBalls(balls, ballsPerOver)
		rpo := float32(0)
		if balls > 0 {
			rpo = float32(float64(runs) / float64(balls) * float64(ballsPerOver))
		}
		target := (*int)(nil)
		if inningNo == 2 && len(m.Innings) >= 1 {
			firRuns := inningsRuns(m.Innings[0])
			target = &firRuns
		}
		upd := &db.MatchInfoUpdate{
			Score:        &runs,
			Wickets:      &wkts,
			Overs:        &oversFloat,
			Balls:        &balls,
			RPO:          &rpo,
			Target:       target,
			Inning:       &inningNo,
			OppositionID: oppositionID,
			Date:         &dateISO,
			VenueID:      venueID,
			Extras:       &extras,
			Toss:         &toss,
			SeasonID:     seasonID,
			MatchNumber:  matchNumber,
		}
		if err := cricDB.UpdateMatchDetails(ctx, mid, upd); err != nil {
			log.Printf("warn: update match_details failed for match_id=%d: %v", mid, err)
		}
		order := make([]string, 0, len(batAgg))
		for name, b := range batAgg {
			b.Name = name
			order = append(order, name)
		}
		order = battingOrderFromInnings(inng, order)
		pos := 1
		for _, name := range order {
			b := batAgg[name]
			pid, _ := cricDB.GetOrCreateByName(ctx, name)
			sr := strikeRate(b.Runs, b.Balls)
			desc := dismissals[name]
			if desc == "" {
				desc = "not out"
			}
			mins := 0
			_ = cricDB.UpsertBatting(ctx, &db.Batting{
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
			})
			pos++
		}
		for name, s := range bowlAgg {
			maidens := maidenCount(overTotalsByBowler[name])
			o := oversFromBalls(s.Balls, ballsPerOver)
			econ := float32(0)
			if s.Balls > 0 {
				econ = float32(float64(s.Runs) / float64(s.Balls) * float64(ballsPerOver))
			}
			pid, _ := cricDB.GetOrCreateByName(ctx, name)
			_ = cricDB.UpsertBowling(ctx, &db.Bowling{
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
			})
		}
	}
	// Optional: insert placeholder fielding rows for all players seen in the match
	if opts != nil && opts.PlaceholdersFielding {
		zero := 0
		for name := range playersSeen {
			pid, _ := cricDB.GetOrCreateByName(ctx, name)
			_ = cricDB.UpsertFielding(ctx, &db.Fielding{
				MatchID:        mid,
				PlayerID:       pid,
				Catches:        &zero,
				RunOuts:        &zero,
				DroppedCatches: &zero,
				MissedRunOuts:  &zero,
			})
		}
	}
	// Enqueue async weather job (non-blocking)
	if opts != nil && opts.WeatherEnqueue {
		_ = weatherClient.EnqueueJob(ctx, mid, info.City, info.Venue, len(m.Innings))
	}
	return nil
}

// helpers reused from cmd implementation

type batRow struct {
	Name  string
	Runs  int
	Balls int
	Fours int
	Sixes int
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
	if v, ok := m[name]; ok {
		return v
	}
	br := &batRow{}
	m[name] = br
	return br
}

func ensureBowl(m map[string]*bowlRow, name string) *bowlRow {
	if v, ok := m[name]; ok {
		return v
	}
	br := &bowlRow{}
	m[name] = br
	return br
}

func inningsRuns(inng Innings) int {
	r := 0
	for _, over := range inng.Overs {
		for _, d := range over.Deliveries {
			r += d.Runs.Total
		}
	}
	return r
}

func oversFromBalls(balls int, bpo int) float32 {
	if bpo <= 0 {
		bpo = 6
	}
	ov := balls / bpo
	rem := balls % bpo
	return float32(float64(ov) + float64(rem)/10.0)
}

func maidenCount(overMap map[int]int) int {
	c := 0
	for _, t := range overMap {
		if t == 0 {
			c++
		}
	}
	return c
}

func strikeRate(runs, balls int) float32 {
	if balls <= 0 {
		return 0
	}
	return float32(float64(runs) / float64(balls) * 100.0)
}

func strPtr(s string) *string { return &s }

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func otherTeam(bat string, a, b string) string {
	if bat == a {
		return b
	}
	if bat == b {
		return a
	}
	if bat == "" {
		return strings.TrimSpace(firstNonEmpty(a, b))
	}
	return ""
}

// battingOrderFromInnings derives a batting order from an innings by using the
// first-seen sequence of players as they appear in deliveries (either as
// batter or non-striker). Any provided names not seen in the deliveries are
// appended at the end in deterministic alphabetical order.
func battingOrderFromInnings(inng Innings, names []string) []string {
	// Map of player -> first index when they appear in the innings
	seen := make(map[string]int, len(names))
	next := 0
	for _, over := range inng.Overs {
		for _, d := range over.Deliveries {
			if b := strings.TrimSpace(d.Batter); b != "" {
				if _, ok := seen[b]; !ok {
					seen[b] = next
					next++
				}
			}
			if ns := strings.TrimSpace(d.NonStriker); ns != "" {
				if _, ok := seen[ns]; !ok {
					seen[ns] = next
					next++
				}
			}
		}
	}
	out := make([]string, len(names))
	copy(out, names)
	sort.SliceStable(out, func(i, j int) bool {
		iPos, iOK := seen[out[i]]
		jPos, jOK := seen[out[j]]
		if iOK && jOK {
			return iPos < jPos
		}
		if iOK != jOK {
			return iOK // seen players come before unseen
		}
		// Neither seen: fall back to case-insensitive name order for stability
		ii := strings.ToLower(out[i])
		jj := strings.ToLower(out[j])
		if ii == jj { // tie-break by original
			return out[i] < out[j]
		}
		return ii < jj
	})
	return out
}
