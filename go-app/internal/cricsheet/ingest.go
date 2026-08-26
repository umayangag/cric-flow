package cricsheet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/resources"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
)

// Options controls optional behaviors for Cricsheet import.
type Options struct {
	PlaceholdersFielding bool
	FailFast             bool
}

// ErrNoMatchFiles is returned when the dataset directory holds nothing to import.
//
// This is a failure, not a no-op. An import that reads the wrong directory finds no
// files and would otherwise complete "successfully" with zero rows — the silent
// success that made a mis-defaulted data directory invisible for as long as it was.
var ErrNoMatchFiles = errors.New("no Cricsheet match files found")

// ImportDir reads all .json files in dir and imports them into the DB concurrently.
// It does not recurse: only files directly in dir are read.
// concurrency limits parallel file imports; 0 or negative uses resource-aware limit (memory/CPU).
// Each file's DB writes run in a single transaction (all-or-nothing per file).
func ImportDir(ctx context.Context, dir string, opts *Options, concurrency int) (int, error) {
	if opts == nil {
		opts = &Options{}
	}
	if concurrency <= 0 {
		concurrency = resources.GetLimit(resources.KindImport)
	}
	if concurrency < 1 {
		concurrency = 1
	}
	files, err := dataset.MatchFiles(dir)
	if err != nil {
		slog.Error("cricsheet.ImportDir ReadDir failed", slog.String("dir", dir), slog.Any("err", err))
		return 0, err
	}
	if len(files) == 0 {
		slog.Error("cricsheet.ImportDir found no match files", slog.String("dir", dir))
		return 0, fmt.Errorf("%w in %s (set %s or inputs.cricsheet_dir to the directory holding the *.json match files)",
			ErrNoMatchFiles, dir, dataset.DirEnvVar)
	}

	slog.Info("pipeline: cricsheet import scanning complete",
		slog.String("dir", dir),
		slog.Int("files_found", len(files)),
		slog.Int("concurrency", concurrency))

	var count int64
	var failedMu sync.Mutex
	var failedFiles []string
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, f := range files {
		f := f // capture
		g.Go(func() error {
			// Skip without logging if another file already failed (ctx cancelled) and FailFast.
			if err := ctx.Err(); err != nil {
				return nil
			}
			slog.Info("importing match file", slog.String("file", filepath.Base(f)))
			if err := ImportMatchFile(ctx, f, opts); err != nil {
				if opts.FailFast {
					slog.Error("import failed, stopping",
						slog.String("file", filepath.Base(f)),
						slog.String("path", f),
						slog.Any("err", err))
					return fmt.Errorf("file %s: %w", filepath.Base(f), err)
				}
				slog.Warn("import failed, skipping",
					slog.String("file", filepath.Base(f)),
					slog.String("path", f),
					slog.Any("err", err))
				failedMu.Lock()
				failedFiles = append(failedFiles, filepath.Base(f))
				failedMu.Unlock()
				return nil
			}
			atomic.AddInt64(&count, 1)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		slog.Error(
			"cricsheet.ImportDir wait failed",
			slog.String("dir", dir),
			slog.Int64("imported", count),
			slog.Any("err", err),
		)
		return int(count), err
	}
	resources.RecordWorkerMemorySample(resources.KindImport, concurrency)
	if len(failedFiles) > 0 {
		slog.Warn("cricsheet.ImportDir finished with skipped files",
			slog.String("dir", dir),
			slog.Int64("imported", count),
			slog.Int("skipped", len(failedFiles)),
			slog.Any("skipped_files", failedFiles))
	}
	return int(count), nil
}

// ImportMatchFile parses a single Cricsheet JSON file and upserts stats into DB.
func ImportMatchFile(ctx context.Context, path string, opts *Options) error {
	cache := db.GetGlobalCache()
	fh, err := os.Open(path)
	if err != nil {
		slog.Error("cricsheet.ImportMatchFile open failed", slog.String("path", path), slog.Any("err", err))
		return err
	}
	defer func() {
		if err := fh.Close(); err != nil {
			slog.Warn("close file failed", slog.String("path", path), slog.Any("err", err))
		}
	}()
	m, err := Parse(fh)
	if err != nil {
		slog.Error("parse failed",
			slog.String("file", path),
			slog.Any("err", err))
		return fmt.Errorf("parse %s: %w", path, err)
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
		slog.Error("unsupported match_type",
			slog.String("file", path),
			slog.String("match_type", info.MatchType),
			slog.String("teams", fmt.Sprintf("%v", info.Teams)))
		return fmt.Errorf("unsupported match_type: %s", info.MatchType)
	}
	formatID, err := cache.GetFormatID(ctx, formatCode)
	if err != nil {
		slog.Error("lookup format_id failed",
			slog.String("file", path),
			slog.String("format", formatCode),
			slog.String("match_date", dateISO),
			slog.String("teams", fmt.Sprintf("%s vs %s", teamA, teamB)),
			slog.Any("err", err))
		return fmt.Errorf("lookup format_id for %s: %w", formatCode, err)
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
			if id, e := cache.GetOppositionID(ctx, winner); e == nil {
				winnerID = &id
			} else {
				slog.Error("get/create opposition for winner failed",
					slog.String("file", path),
					slog.Int64("match_id", mid),
					slog.String("winner", winner),
					slog.String("teams", fmt.Sprintf("%s vs %s", teamA, teamB)),
					slog.Any("err", e))
				return fmt.Errorf("get/create opposition for winner '%s': %w", winner, e)
			}
		}
	}

	ballsPerOver := info.BallsPerOver
	if ballsPerOver <= 0 {
		ballsPerOver = 6
	}
	var tossWinnerOppositionID *int64
	if toss != "" {
		if id, e := cache.GetOppositionID(ctx, toss); e == nil {
			tossWinnerOppositionID = &id
		}
	}
	scheduledOvers := scheduledOversFromFormatOrInfo(formatCode, info.Overs)
	eventName := ""
	if info.Event != nil {
		eventName = info.Event.Name
	}
	matchInsert := &db.MatchInsert{
		MatchID:                   mid,
		FormatID:                  formatID,
		MatchDate:                 dateISO,
		OriginalMatchType:         info.MatchType,
		VenueID:                   venueID,
		SeasonID:                  seasonID,
		TossWinnerOppositionID:    tossWinnerOppositionID,
		TossDecision:              strPtrFromToss(info.Toss),
		OutcomeWinnerOppositionID: winnerID,
		OutcomeByRuns:             outcomeByRuns(info.Outcome),
		OutcomeByWickets:          outcomeByWickets(info.Outcome),
		EventName:                 strPtrNonEmpty(eventName),
		MatchNumber:               matchNumber,
		Gender:                    strPtrNonEmpty(info.Gender),
		BallsPerOver:              ballsPerOver,
		ScheduledOversPerInnings:  scheduledOvers,
	}
	playersSeen := map[string]bool{}
	var allFieldingEvents [][]db.FieldingEvent
	var allMatchInnings []*db.MatchInningInsert
	var allBatBatches [][]db.Batting
	var allBowlBatches [][]db.Bowling
	for i, inng := range m.Innings {
		inningNo := i + 1
		batTeam := strings.TrimSpace(inng.Team)
		oppTeam := otherTeam(batTeam, teamA, teamB)
		// Validate inning team names match match teams to avoid creating opposition rows with empty name
		if batTeam == "" {
			slog.Error("inning has no team name",
				slog.String("file", path),
				slog.Int64("match_id", mid),
				slog.String("match_date", dateISO),
				slog.Int("inning", inningNo))
			return fmt.Errorf("inning %d has no team name", inningNo)
		}
		if oppTeam == "" {
			slog.Error("inning team does not match match teams",
				slog.String("file", path),
				slog.Int64("match_id", mid),
				slog.String("match_date", dateISO),
				slog.Int("inning", inningNo),
				slog.String("inning_team", batTeam),
				slog.String("match_teams", fmt.Sprintf("%q and %q", teamA, teamB)))
			return fmt.Errorf("inning %d team %q does not match match teams %q and %q", inningNo, batTeam, teamA, teamB)
		}
		battingTeamOppositionID, err := cache.GetOppositionID(ctx, batTeam)
		if err != nil {
			slog.Error("get/create opposition for batting team failed",
				slog.String("file", path),
				slog.Int64("match_id", mid),
				slog.String("match_date", dateISO),
				slog.Int("inning", inningNo),
				slog.String("batting_team", batTeam),
				slog.Any("err", err))
			return fmt.Errorf("get/create opposition for batting team %q: %w", batTeam, err)
		}
		bowlingTeamOppositionID, err := cache.GetOppositionID(ctx, oppTeam)
		if err != nil {
			slog.Error("get/create opposition for bowling team failed",
				slog.String("file", path),
				slog.Int64("match_id", mid),
				slog.String("match_date", dateISO),
				slog.Int("inning", inningNo),
				slog.String("bowling_team", oppTeam),
				slog.Any("err", err))
			return fmt.Errorf("get/create opposition for bowling team %q: %w", oppTeam, err)
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
								slog.Error("get/create player for batter (fielding_event) failed",
									slog.String("file", path),
									slog.Int64("match_id", mid),
									slog.Int("inning", inningNo),
									slog.Int("over", overNo),
									slog.String("batter", w.PlayerOut),
									slog.String("kind", kindLower),
									slog.Any("err", err))
								return fmt.Errorf("get/create player for batter %q: %w", w.PlayerOut, err)
							}
							var bowlerID *int64
							// Bowler is only associated with 'caught' dismissals.
							if isCaught && d.Bowler != "" {
								bid, err := cache.GetPlayerID(ctx, d.Bowler)
								if err != nil {
									slog.Error("get/create player for bowler (fielding_event) failed",
										slog.String("file", path),
										slog.Int64("match_id", mid),
										slog.Int("inning", inningNo),
										slog.String("bowler", d.Bowler),
										slog.String("batter_out", w.PlayerOut),
										slog.Any("err", err))
									return fmt.Errorf("get/create player for bowler %q: %w", d.Bowler, err)
								}
								bowlerID = &bid
							}
							for _, fn := range fNames {
								fid, err := cache.GetPlayerID(ctx, fn)
								if err != nil {
									slog.Error("get/create player for fielder failed",
										slog.String("file", path),
										slog.Int64("match_id", mid),
										slog.Int("inning", inningNo),
										slog.Int("over", overNo),
										slog.String("fielder", fn),
										slog.String("batter_out", w.PlayerOut),
										slog.String("kind", kindLower),
										slog.Any("err", err))
									return fmt.Errorf("get/create player for fielder %q: %w", fn, err)
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
		allFieldingEvents = append(allFieldingEvents, fieldingEvents)
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
		mi := &db.MatchInningInsert{
			MatchID:                 mid,
			InningNumber:            inningNo,
			BattingTeamOppositionID: battingTeamOppositionID,
			BowlingTeamOppositionID: bowlingTeamOppositionID,
			RunsScored:              runs,
			WicketsLost:             wkts,
			OversBowled:             oversFloat,
			BallsBowled:             balls,
			RunRate:                 &rpo,
			TargetRuns:              target,
			Extras:                  extras,
			WinnerOppositionID:      winnerID,
		}
		allMatchInnings = append(allMatchInnings, mi)
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
			pid, err := cache.GetPlayerID(ctx, name)
			if err != nil {
				slog.Error("get/create player for batting failed",
					slog.String("file", path),
					slog.Int64("match_id", mid),
					slog.String("match_date", dateISO),
					slog.Int("inning", inningNo),
					slog.String("batting_team", batTeam),
					slog.String("player", name),
					slog.Any("err", err))
				return fmt.Errorf("get/create player %q for batting: %w", name, err)
			}
			sr := strikeRate(b.Runs, b.Balls)
			desc := dismissals[name]
			if desc == "" {
				desc = "not out"
			}
			mins := 0
			batBatch = append(batBatch, db.Batting{
				MatchID:         mid,
				InningNumber:    inningNo,
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
		allBatBatches = append(allBatBatches, batBatch)

		var bowlBatch []db.Bowling
		for name, s := range bowlAgg {
			maidens := maidenCount(overTotalsByBowler[name])
			o := oversFromBalls(s.Balls, ballsPerOver)
			econ := float32(0)
			if s.Balls > 0 {
				econ = float32(float64(s.Runs) / float64(s.Balls) * float64(ballsPerOver))
			}
			pid, err := cache.GetPlayerID(ctx, name)
			if err != nil {
				slog.Error("get/create player for bowling failed",
					slog.String("file", path),
					slog.Int64("match_id", mid),
					slog.String("match_date", dateISO),
					slog.Int("inning", inningNo),
					slog.String("bowling_team", oppTeam),
					slog.String("player", name),
					slog.Any("err", err))
				return fmt.Errorf("get/create player %q for bowling: %w", name, err)
			}
			bowlBatch = append(bowlBatch, db.Bowling{
				MatchID:      mid,
				InningNumber: inningNo,
				PlayerID:     pid,
				Overs:        &o,
				Balls:        &s.Balls,
				Maidens:      &maidens,
				Runs:         &s.Runs,
				Wickets:      &s.Wickets,
				Dots:         &s.Dots,
				Fours:        &s.Fours,
				Sixes:        &s.Sixes,
				Econ:         &econ,
				Wides:        &s.Wides,
				NoBalls:      &s.NoBalls,
			})
		}
		allBowlBatches = append(allBowlBatches, bowlBatch)
	}
	// Build ball event rows (requires cache; done before tx)
	ballEventRows, err := BuildBallEventRows(ctx, m, int(formatID), mid)
	if err != nil {
		slog.Error("failed to build ball_event rows",
			slog.String("file", path),
			slog.Int64("match_id", mid),
			slog.String("match_date", dateISO),
			slog.String("teams", fmt.Sprintf("%s vs %s", teamA, teamB)),
			slog.Int("innings_count", len(m.Innings)),
			slog.Any("err", err))
		return fmt.Errorf("build ball events: %w", err)
	}
	// Build fielding placeholder batch if needed
	var fieldingBatch []db.Fielding
	if opts != nil && opts.PlaceholdersFielding {
		zero := 0
		for name := range playersSeen {
			pid, err := cache.GetPlayerID(ctx, name)
			if err != nil {
				slog.Error("get/create player for fielding placeholder failed",
					slog.String("file", path),
					slog.Int64("match_id", mid),
					slog.String("match_date", dateISO),
					slog.String("teams", fmt.Sprintf("%s vs %s", teamA, teamB)),
					slog.String("player", name),
					slog.Any("err", err))
				return fmt.Errorf("get/create player %q for fielding placeholder: %w", name, err)
			}
			fieldingBatch = append(fieldingBatch, db.Fielding{
				MatchID:        mid,
				PlayerID:       pid,
				Catches:        &zero,
				RunOuts:        &zero,
				DroppedCatches: &zero,
				MissedRunOuts:  &zero,
			})
		}
	}
	// Run all match-specific DB writes in a single transaction (all-or-nothing).
	// Note: Dimension entity creation (players, venues, seasons, oppositions via cache.Get*)
	// happens above, outside this transaction. If the tx rolls back, those new dimension rows
	// remain and can become orphaned. Full atomicity would require refactoring the cache/DB
	// to accept a transaction for all writes.
	runTx := db.RunInTx
	if runInTxFn != nil {
		runTx = runInTxFn
	}
	matchCtx := struct {
		file, date, teams string
	}{path, dateISO, fmt.Sprintf("%s vs %s", teamA, teamB)}
	if err := runTx(ctx, func(ctx context.Context, tx db.CopyFromTx) error {
		if err := db.UpsertMatchTx(ctx, tx, matchInsert); err != nil {
			slog.Error("upsert match failed",
				slog.String("file", matchCtx.file),
				slog.Int64("match_id", mid),
				slog.String("match_date", matchCtx.date),
				slog.String("teams", matchCtx.teams),
				slog.String("match_type", info.MatchType),
				slog.Any("err", err))
			return fmt.Errorf("upsert match: %w", err)
		}
		for i := range allMatchInnings {
			inn := allMatchInnings[i]
			if err := db.InsertFieldingEventsBatchTx(ctx, tx, allFieldingEvents[i]); err != nil {
				slog.Error("insert fielding_events failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("inning", inn.InningNumber),
					slog.Int("events_count", len(allFieldingEvents[i])),
					slog.Any("err", err))
				return fmt.Errorf("insert fielding_events: %w", err)
			}
			if err := db.UpsertMatchInningTx(ctx, tx, inn); err != nil {
				slog.Error("upsert match_inning failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("inning", inn.InningNumber),
					slog.Int("runs_scored", inn.RunsScored),
					slog.Int("wickets_lost", inn.WicketsLost),
					slog.Any("err", err))
				return fmt.Errorf("upsert match_inning: %w", err)
			}
			batBatch := allBatBatches[i]
			if err := db.UpsertBattingBatchTx(ctx, tx, batBatch); err != nil {
				firstPlayer := ""
				if len(batBatch) > 0 && batBatch[0].PlayerID != 0 {
					firstPlayer = fmt.Sprintf("player_id=%d", batBatch[0].PlayerID)
				}
				slog.Error("upsert batting batch failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("inning", inn.InningNumber),
					slog.Int("batch_size", len(batBatch)),
					slog.String("first_row", firstPlayer),
					slog.Any("err", err))
				return fmt.Errorf("upsert batting batch: %w", err)
			}
			bowlBatch := allBowlBatches[i]
			if err := db.UpsertBowlingBatchTx(ctx, tx, bowlBatch); err != nil {
				firstPlayer := ""
				if len(bowlBatch) > 0 && bowlBatch[0].PlayerID != 0 {
					firstPlayer = fmt.Sprintf("player_id=%d", bowlBatch[0].PlayerID)
				}
				slog.Error("upsert bowling batch failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("inning", inn.InningNumber),
					slog.Int("batch_size", len(bowlBatch)),
					slog.String("first_row", firstPlayer),
					slog.Any("err", err))
				return fmt.Errorf("upsert bowling batch: %w", err)
			}
		}
		if len(fieldingBatch) > 0 {
			if err := db.UpsertFieldingBatchTx(ctx, tx, fieldingBatch); err != nil {
				slog.Error("upsert fielding batch failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("batch_size", len(fieldingBatch)),
					slog.Any("err", err))
				return fmt.Errorf("upsert fielding batch: %w", err)
			}
		}
		if err := db.RecomputeFieldingAggregatesTx(ctx, tx, mid); err != nil {
			slog.Error("recompute fielding aggregates failed",
				slog.String("file", matchCtx.file),
				slog.Int64("match_id", mid),
				slog.String("match_date", matchCtx.date),
				slog.String("teams", matchCtx.teams),
				slog.Any("err", err))
			return fmt.Errorf("recompute fielding aggregates: %w", err)
		}
		if len(ballEventRows) > 0 {
			if err := db.InsertBallEventsTx(ctx, tx, ballEventRows); err != nil {
				slog.Error("insert ball_event failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("rows_count", len(ballEventRows)),
					slog.Any("err", err))
				return fmt.Errorf("insert ball events: %w", err)
			}
		}
		return nil
	}); err != nil {
		slog.Error(
			"cricsheet.ImportMatchFile transaction failed",
			slog.String("path", path),
			slog.Int64("match_id", mid),
			slog.Any("err", err),
		)
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

func strPtrNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func strPtrFromToss(t *Toss) *string {
	if t == nil || t.Decision == "" {
		return nil
	}
	return &t.Decision
}

func outcomeByRuns(o *Outcome) *int {
	if o == nil || o.By == nil {
		return nil
	}
	return o.By.Runs
}

func outcomeByWickets(o *Outcome) *int {
	if o == nil || o.By == nil {
		return nil
	}
	return o.By.Wickets
}

func scheduledOversFromFormatOrInfo(formatCode string, infoOvers int) *int {
	if infoOvers > 0 {
		return &infoOvers
	}
	switch formatCode {
	case "T20", "T20I":
		v := 20
		return &v
	case "ODI":
		v := 50
		return &v
	default:
		return nil
	}
}

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
