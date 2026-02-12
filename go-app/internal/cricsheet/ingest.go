package cricsheet

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Options controls optional behaviors for Cricsheet import.
type Options struct {
	PlaceholdersWeather  bool
	PlaceholdersFielding bool
	WeatherEnqueue       bool
}

// ImportDir reads all .json files in dir and imports them into the DB concurrently.
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

	var count int64
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	for _, f := range files {
		f := f // capture
		g.Go(func() error {
			slog.Info("importing match file", slog.String("file", filepath.Base(f)))
			if err := ImportMatchFile(ctx, f, opts); err != nil {
				return fmt.Errorf("file %s: %w", filepath.Base(f), err)
			}
			atomic.AddInt64(&count, 1)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return int(count), err
	}
	return int(count), nil
}

// ImportMatchFile parses a single Cricsheet JSON file and upserts stats into DB.
func ImportMatchFile(ctx context.Context, path string, opts *Options) error {
	cache := db.GetGlobalCache()
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := fh.Close(); err != nil {
			slog.Warn("close file failed", slog.String("path", path), slog.Any("err", err))
		}
	}()
	m, err := Parse(fh)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	info := m.Info
	dateISO := info.MatchDate()
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
	formatID, err := cache.GetFormatID(ctx, formatCode)
	if err != nil {
		return fmt.Errorf("lookup format_id for %s: %w", formatCode, err)
	}
	if err := cricDB.EnsureMatchWithFormat(ctx, mid, formatID, dateISO, info.MatchType); err != nil {
		return fmt.Errorf("ensure match with format: %w", err)
	}
	venueName := strings.TrimSpace(firstNonEmpty(info.Venue, info.City))
	var venueID *int64
	if venueName != "" {
		if id, e := cache.GetVenueID(ctx, venueName); e == nil {
			venueID = &id
		}
	}
	var seasonID *int64
	if s := strings.TrimSpace(string(info.Season)); s != "" {
		if id, e := cache.GetSeasonID(ctx, s); e == nil {
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
	var winnerID *int64
	if info.Outcome != nil {
		winner = strings.TrimSpace(info.Outcome.Winner)
		if winner != "" {
			if id, e := cricDB.GetOrCreateOpposition(ctx, winner); e == nil {
				winnerID = &id
            } else {
				return fmt.Errorf("get/create opposition for winner '%s': %w", winner, e)
			}
		}
	}

	ballsPerOver := info.BallsPerOver
	if ballsPerOver <= 0 {
		ballsPerOver = 6
	}
	// Optional: insert placeholder weather rows once per match
	if opts != nil && opts.PlaceholdersWeather {
		if err := cricDB.Exec(
			ctx,
			`INSERT INTO weather_data(match_id, session) VALUES ($1,$2) ON CONFLICT (match_id, session) DO NOTHING`,
			mid,
			"inning1",
		); err != nil {
			return fmt.Errorf("insert weather placeholder 1: %w", err)
		}
		if err := cricDB.Exec(
			ctx,
			`INSERT INTO weather_data(match_id, session) VALUES ($1,$2) ON CONFLICT (match_id, session) DO NOTHING`,
			mid,
			"inning2",
		); err != nil {
			return fmt.Errorf("insert weather placeholder 2: %w", err)
		}
	}
	playersSeen := map[string]bool{}
	for i, inng := range m.Innings {
		inningNo := i + 1
		batTeam := strings.TrimSpace(inng.Team)
		oppTeam := otherTeam(batTeam, teamA, teamB)
		var oppositionID *int64
		if oppTeam != "" {
			if id, e := cache.GetOppositionID(ctx, oppTeam); e == nil {
				oppositionID = &id
			}
		}
		// totals and aggregates
		runs, wkts, balls, extras := 0, 0, 0, 0
		batAgg := map[string]*batRow{}
		dismissals := map[string]string{}
		bowlAgg := map[string]*bowlRow{}
		overTotalsByBowler := map[string]map[int]int{}
		var fieldingEvents []db.FieldingEvent
		for _, over := range inng.Overs {
			overNo := over.Over
			perBowler := map[string]int{}
			for ballIndex, d := range over.Deliveries {
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
						bowlNumber := ballIndex + 1
						desc := w.Kind
						if w.Fielders != nil && len(*w.Fielders) > 0 {
							desc = desc + " " + strings.Join(*w.Fielders, ", ")
						}
						dismissals[w.PlayerOut] = strings.TrimSpace(desc)
						// Emit fielding_event rows for fielding-related dismissals
						kindLower := strings.ToLower(strings.TrimSpace(w.Kind))
						var eventKind, assistRole string
						isCaught := false
						switch kindLower {
						case "caught":
							eventKind = "caught"
							isCaught = true
						case "run out", "runout", "run_out":
							eventKind = "run_out"
							assistRole = "assist"
						case "stumped":
							eventKind = "stumped"
							assistRole = "keeper"
						default:
							continue // Not a fielding dismissal we are tracking
						}

						var fNames []string
						if w.Fielders != nil {
							fNames = append(fNames, *w.Fielders...)
						}
						// Special case for caught and bowled: fielder is the bowler.
						if isCaught && len(fNames) == 0 && d.Bowler != "" {
							fNames = []string{d.Bowler}
						}

						if len(fNames) > 0 {
							batterID, err := cache.GetPlayerID(ctx, w.PlayerOut)
							if err != nil {
								slog.Warn(
									"get/create player failed",
									slog.String("name", w.PlayerOut),
									slog.Any("err", err),
								)
								continue
							}
							var bowlerID *int64
							// Bowler is only associated with 'caught' dismissals.
							if isCaught && d.Bowler != "" {
								bid, err := cache.GetPlayerID(ctx, d.Bowler)
								if err != nil {
									slog.Warn(
										"get/create player failed",
										slog.String("name", d.Bowler),
										slog.Any("err", err),
									)
									continue
								}
								bowlerID = &bid
							}
							for _, fn := range fNames {
								fid, err := cache.GetPlayerID(ctx, fn)
								if err != nil {
									slog.Warn("get/create player failed", slog.String("name", fn), slog.Any("err", err))
									continue
								}
								fieldingEvents = append(fieldingEvents, db.FieldingEvent{
									MatchID:     mid,
									Innings:     inningNo,
									Over:        overNo,
									Ball:        bowlNumber,
									BatterOutID: &batterID,
									FielderID:   &fid,
									BowlerID:    bowlerID,
									Kind:        eventKind,
									AssistRole:  assistRole,
									IsDirectHit: false,
									Notes:       nil,
								})
							}
						}
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
		if err := insertFieldingEventsBatchFn(ctx, fieldingEvents); err != nil {
			slog.Warn("insert fielding_events failed", slog.Int64("match_id", mid), slog.Any("err", err))
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
			Balls:             &balls,
			BattingSession:    &batTeam,
			BowlingSession:    &oppTeam,
			Extras:            &extras,
			Inning:            &inningNo,
			MatchDate:         &dateISO,
			MatchNumber:       matchNumber,
			OppositionID:      oppositionID,
			Overs:             &oversFloat,
			RPO:               &rpo,
			Result:            winnerID,
			Score:             &runs,
			SeasonID:          seasonID,
			Target:            target,
			Toss:              &toss,
			VenueID:           venueID,
			Wickets:           &wkts,
			OriginalMatchType: &info.MatchType,
		}
		if err := cricDB.UpdateMatchDetails(ctx, mid, upd); err != nil {
			slog.Warn("update match_details failed", slog.Int64("match_id", mid), slog.Any("err", err))
		}
		order := make([]string, 0, len(batAgg))
		for name, b := range batAgg {
			b.Name = name
			order = append(order, name)
		}
		order = battingOrderFromInnings(inng, order)
		pos := 1
		var batBatch []db.Batting
		for _, name := range order {
			b := batAgg[name]
			pid, _ := cache.GetPlayerID(ctx, name)
			sr := strikeRate(b.Runs, b.Balls)
			desc := dismissals[name]
			if desc == "" {
				desc = "not out"
			}
			mins := 0
			batBatch = append(batBatch, db.Batting{
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
		if err := cricDB.UpsertBattingBatch(ctx, batBatch); err != nil {
			slog.Warn("upsert batting batch failed", slog.Int64("match_id", mid), slog.Any("err", err))
		}

		var bowlBatch []db.Bowling
		for name, s := range bowlAgg {
			maidens := maidenCount(overTotalsByBowler[name])
			o := oversFromBalls(s.Balls, ballsPerOver)
			econ := float32(0)
			if s.Balls > 0 {
				econ = float32(float64(s.Runs) / float64(s.Balls) * float64(ballsPerOver))
			}
			pid, _ := cache.GetPlayerID(ctx, name)
			bowlBatch = append(bowlBatch, db.Bowling{
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
		if err := cricDB.UpsertBowlingBatch(ctx, bowlBatch); err != nil {
			slog.Warn("upsert bowling batch failed", slog.Int64("match_id", mid), slog.Any("err", err))
		}
	}
	// Optional: insert placeholder fielding rows for all players seen in the match
	if opts != nil && opts.PlaceholdersFielding {
		zero := 0
		var fieldingBatch []db.Fielding
		for name := range playersSeen {
			pid, _ := cache.GetPlayerID(ctx, name)
			fieldingBatch = append(fieldingBatch, db.Fielding{
				MatchID:        mid,
				PlayerID:       pid,
				Catches:        &zero,
				RunOuts:        &zero,
				DroppedCatches: &zero,
				MissedRunOuts:  &zero,
			})
		}
		if err = cricDB.UpsertFieldingBatch(ctx, fieldingBatch); err != nil {
			slog.Error("failed to insert placeholder fielding batch", slog.Int64("match_id", mid), slog.Any("err", err))
			return err
		}
	}
	// Enqueue async weather job (non-blocking)
	if opts != nil && opts.WeatherEnqueue {
		if err := weatherClient.EnqueueJob(ctx, mid, info.City, info.Venue, len(m.Innings)); err != nil {
			slog.Error("failed to enqueue weather job", slog.Int64("match_id", mid), slog.Any("err", err))
			return err
		}
	}
	// Recompute fielding aggregates from emitted events for this match
	if err = recomputeFn(ctx, mid); err != nil {
		slog.Error("failed to recompute fielding aggregates", slog.Int64("match_id", mid), slog.Any("err", err))
		return err
	}

	if err = EmitBallEvents(ctx, m, int(formatID), mid); err != nil {
		slog.Error("failed to emit ball_event rows", slog.Int64("match_id", mid), slog.Any("err", err))
		return err
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
