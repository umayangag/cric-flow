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

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
	"github.com/umayangag/cric-flow/go-app/internal/resources"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	"github.com/umayangag/cric-flow/go-app/internal/teamlineage"
	"github.com/umayangag/cric-flow/go-app/internal/venues"
	"github.com/umayangag/cric-flow/go-app/internal/wicketkinds"
)

// Options controls optional behaviors for Cricsheet import.
type Options struct {
	PlaceholdersFielding bool
	FailFast             bool
	// WicketKinds is the vocabulary that says what each wicket kind is to the scorecard
	// (configs/wicket_kinds.json). ImportDir loads it once for the run; a single-file
	// import with none set loads it itself.
	WicketKinds *wicketkinds.Vocabulary
}

// wicketKinds is the vocabulary an import classifies wickets by: the one it was given,
// else the committed file.
func (o *Options) wicketKinds() (wicketkinds.Vocabulary, error) {
	if o != nil && o.WicketKinds != nil {
		return *o.WicketKinds, nil
	}
	return wicketkinds.Load()
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
	// The process-global EntityCache never expires on its own, and go-app's API server
	// can run this more than once across its own lifetime (the pipeline's import step);
	// a dev-destroy or re-migrate between two such runs would otherwise leave the second
	// one resolving names against ids the schema no longer holds (IMPORT-16). Every run
	// starts from nothing memoised, before any file is read.
	db.GetGlobalCache().Clear()
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
		return 0, fmt.Errorf(
			"%w in %s (set %s or inputs.cricsheet_dir to the directory holding the *.json match files)",
			ErrNoMatchFiles,
			dir,
			dataset.DirEnvVar,
		)
	}

	slog.Info("pipeline: cricsheet import scanning complete",
		slog.String("dir", dir),
		slog.Int("files_found", len(files)),
		slog.Int("concurrency", concurrency))

	// The wicket vocabulary is read once for the run, not once per file, and a run that
	// cannot read it does not start: without it no wicket can be classified.
	vocabulary, err := opts.wicketKinds()
	if err != nil {
		slog.Error("cricsheet.ImportDir could not read the wicket kinds vocabulary",
			slog.String("dir", dir),
			slog.Any("err", err))
		return 0, fmt.Errorf("read wicket kinds: %w", err)
	}
	shared := *opts
	shared.WicketKinds = &vocabulary
	opts = &shared

	var count int64
	var failedMu sync.Mutex
	var failedFiles []string
	// Which spelling of a name each player ends up displayed under is decided across the
	// whole import, not per file, so it cannot depend on which goroutine finished first.
	names := newDisplayNames()
	// errgroup's context is cancelled as soon as Wait returns, success or not, so the
	// flush below has to run on the caller's context rather than the workers'.
	parentCtx := ctx
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
			if err := importMatchFile(ctx, f, opts, names); err != nil {
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

	waitErr := g.Wait()
	if waitErr != nil {
		slog.Error(
			"cricsheet.ImportDir wait failed",
			slog.String("dir", dir),
			slog.Int64("imported", count),
			slog.Any("err", waitErr),
		)
	} else {
		resources.RecordWorkerMemorySample(resources.KindImport, concurrency)
	}
	// Settlement runs whether every file landed or not, because an abort does not undo
	// what already committed. The lookup rows -- players, clubs -- are written on the pool
	// rather than inside the per-file transaction, so a run that stopped on file 9,000
	// leaves 9,000 matches and every identity they touched behind it, unsettled. Returning
	// early there did not leave the archive untouched; it left it half-written and looking
	// finished (IMPORT-07).
	settleErr := settleImport(parentCtx, dir, names)
	if len(failedFiles) > 0 {
		slog.Warn("cricsheet.ImportDir finished with skipped files",
			slog.String("dir", dir),
			slog.Int64("imported", count),
			slog.Int("skipped", len(failedFiles)),
			slog.Any("skipped_files", failedFiles))
	}
	return int(count), errors.Join(waitErr, settleErr)
}

// settleImport finishes an import: the display names it collected across the files, then
// the club lineage. Both run even when one fails, because they settle different things and
// neither depends on the other; the errors are joined so a failure in one cannot hide the
// other the way an early return did.
//
// Settling names over a partial import is safe, not a compromise. The rule picks the
// spelling from the latest match date seen, and UpdatePlayerDisplayNames will not move a
// name backwards -- it writes only where the new name_as_of is at or after the stored one.
// So the worst a half-read directory can do is choose the name a smaller import would have
// chosen. Not settling is the lossy option: the names already committed are whichever
// file's goroutine created the row first, which is not reproducible.
//
// A nil collector means there are no names to settle -- the single-file path, which has
// one spelling and nothing to weigh it against -- and only the lineage runs.
func settleImport(ctx context.Context, dir string, names *displayNames) error {
	var settleNamesErr error
	if names != nil {
		if err := cricDB.UpdatePlayerDisplayNames(ctx, names.rows()); err != nil {
			slog.Error("cricsheet: could not settle player display names",
				slog.String("dir", dir),
				slog.Int("players", len(names.rows())),
				slog.Any("err", err))
			settleNamesErr = fmt.Errorf("settle player display names: %w", err)
		}
	}
	return errors.Join(settleNamesErr, applyTeamLineage(ctx, dir))
}

// applyTeamLineage links every superseded team row to the club's current row, from the
// reviewed mapping in configs/team_lineage.json.
//
// A mapping that exists but does not parse or does not validate fails the import: that is
// a bug in committed data, and importing 22,734 files against a broken mapping only buries
// it. A mapping that is simply *absent* does not fail -- a deployment may legitimately have
// none, and losing the whole dataset over a search path would be the worse trade -- but it
// warns, and the counts below are what make its absence visible in a run's log.
//
// The log names all three outcomes separately -- linked, written by this run, and absent
// from this dataset -- because a single "rows changed: 0" was the same answer for a mapping
// already applied, a dataset that stops before a rename, and a pass that never ran.
func applyTeamLineage(ctx context.Context, dir string) error {
	mapping, err := teamlineage.Load()
	if err != nil {
		slog.Error("cricsheet.ImportDir could not read the team lineage mapping",
			slog.String("dir", dir),
			slog.Any("err", err))
		return fmt.Errorf("read team lineage: %w", err)
	}
	if len(mapping.Renames) == 0 {
		return nil
	}
	renames := make([]db.TeamRename, 0, len(mapping.Renames))
	for _, r := range mapping.Renames {
		renames = append(renames, db.TeamRename{FromName: r.From, ToName: r.To, Gender: r.Gender})
	}
	report, err := cricDB.ApplyTeamLineage(ctx, renames)
	if err != nil {
		slog.Error("cricsheet: could not apply the team lineage mapping",
			slog.String("dir", dir),
			slog.Any("err", err))
		return fmt.Errorf("apply team lineage: %w", err)
	}
	slog.Info("cricsheet: team lineage applied",
		slog.Int("renames", report.Requested()),
		slog.Int("linked", report.Count(db.TeamLineageLinked)),
		slog.Int("linked_by_this_run", report.Changed()),
		slog.Any("absent_from_this_dataset", report.InState(db.TeamLineageAbsent)))
	// A rename still unlinked after the pass that exists to link it should be
	// unreachable: both rows are in the archive, so the statement matched them. Saying
	// nothing is what made the original defect invisible, so the impossible case is loud.
	if unlinked := report.InState(db.TeamLineageUnlinked); len(unlinked) > 0 {
		slog.Error("cricsheet: team lineage left renamed clubs unlinked",
			slog.String("dir", dir),
			slog.Any("unlinked_renames", unlinked))
	}
	return nil
}

// ImportMatchFile parses a single Cricsheet JSON file and upserts stats into DB.
//
// It applies the club lineage afterwards but does not settle display names, and the
// asymmetry is the point. Lineage is a closed reviewed list that says nothing about this
// import: one file can create the opposition row that completes a rename -- lookup rows are
// written on the pool, not in the file's transaction -- and linking it is idempotent, so
// running it keeps the single-file path from leaving an archive the directory path would
// not. Display-name settlement is the opposite: a computation over the files read, and one
// file has no other spelling to weigh its own against, which is why the collector is nil
// here (IMPORT-07). Lineage runs even when the file failed, for the same reason it runs
// after an aborted directory import.
func ImportMatchFile(ctx context.Context, path string, opts *Options) error {
	importErr := importMatchFile(ctx, path, opts, nil)
	return errors.Join(importErr, settleImport(ctx, filepath.Dir(path), nil))
}

// importMatchFile is ImportMatchFile with the import-wide display-name collector, which
// only a directory import has. A single-file import stores the name that file gives and
// has nothing to reconcile it against.
func importMatchFile(ctx context.Context, path string, opts *Options, names *displayNames) error {
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
	vocabulary, err := opts.wicketKinds()
	if err != nil {
		slog.Error("cricsheet.ImportMatchFile could not read the wicket kinds vocabulary",
			slog.String("file", path),
			slog.Any("err", err))
		return fmt.Errorf("read wicket kinds: %w", err)
	}
	info := m.Info
	dateISO := info.MatchDate()
	// Every name below is resolved through this: it carries the file's person registry
	// and the match's gender, which are the two things that turn a name into an identity.
	identity := &matchIdentity{
		entities: cache,
		registry: info.Registry.PersonIDsByName(),
		gender:   strings.TrimSpace(info.Gender),
		dateISO:  dateISO,
		path:     path,
		names:    names,
	}
	teamA, teamB := "Team A", "Team B"
	if len(info.Teams) >= 1 {
		teamA = info.Teams[0]
	}
	if len(info.Teams) >= 2 {
		teamB = info.Teams[1]
	}
	mid := MatchIDFromSource(SourceRef(path), dateISO, teamA, teamB)
	// The level is read before the format because the format depends on it: a "T20"
	// between national sides is a T20I only by its team_type. A file that names no level
	// is refused, not guessed at, for the same reason the twelve-team list was retired.
	competitionLevel, err := formats.ParseCompetitionLevel(info.TeamType)
	if err != nil {
		slog.Error("cricsheet: competition level unreadable",
			slog.String("file", path),
			slog.String("team_type", info.TeamType),
			slog.String("match_type", info.MatchType),
			slog.String("teams", fmt.Sprintf("%v", info.Teams)),
			slog.Any("err", err))
		return fmt.Errorf("competition level of %s: %w", path, err)
	}
	formatCode := DetectFormat(info.MatchType, competitionLevel)
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
	// A match file with no venue has no venue. The city it names is where a ground is, not
	// a ground, and standing it in built venue rows named after cities that carried real
	// familiarity and scoring history under a name no XI ever played at (IMPORT-08).
	// match.venue_id is nullable, so the honest record of an unnamed ground is no ground,
	// and that is also the answer for a name that carries no identity once folded.
	venueName := strings.TrimSpace(info.Venue)
	venueCity := strings.TrimSpace(info.City)
	var venueID *int64
	if venues.NormalizeName(venueName) != "" {
		id, e := cache.GetVenueID(ctx, venueName, venueCity)
		if e != nil {
			slog.Error("lookup venue_id failed",
				slog.String("file", path),
				slog.String("venue", venueName),
				slog.String("match_date", dateISO),
				slog.Any("err", e))
			return fmt.Errorf("lookup venue_id for %q: %w", venueName, e)
		}
		venueID = &id
	}
	// A season the file names is a season the match belongs to. Failing the lookup used
	// to leave season_id NULL in silence, which reads downstream as a match played in no
	// season at all -- indistinguishable from a file that names none (IMPORT-13).
	var seasonID *int64
	if s := strings.TrimSpace(string(info.Season)); s != "" {
		id, e := cache.GetSeasonID(ctx, s)
		if e != nil {
			slog.Error("lookup season_id failed",
				slog.String("file", path),
				slog.String("season", s),
				slog.String("match_date", dateISO),
				slog.Any("err", e))
			return fmt.Errorf("lookup season_id for %q: %w", s, e)
		}
		seasonID = &id
	}
	matchNumber := (*int)(nil)
	if info.Event != nil && info.Event.MatchNumber != nil {
		matchNumber = info.Event.MatchNumber
	}
	toss := ""
	if info.Toss != nil {
		toss = info.Toss.Winner
	}
	// The winner is whichever side the outcome says the match went to, tie-breakers
	// included (Outcome.WinningTeam); the result and method are stored beside it verbatim.
	var winnerID *int64
	if winner := info.Outcome.WinningTeam(); winner != "" {
		if id, e := identity.OppositionID(ctx, winner); e == nil {
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

	ballsPerOver := info.BallsPerOver
	if ballsPerOver <= 0 {
		ballsPerOver = 6
	}
	// The toss winner is a side the file names, and it is one of the two playing. The
	// lookup failing used to leave toss_winner_opposition_id NULL without a word, which
	// reads as a match with no toss -- and the toss is a feature (IMPORT-13).
	var tossWinnerOppositionID *int64
	if toss != "" {
		id, e := identity.OppositionID(ctx, toss)
		if e != nil {
			slog.Error("get/create opposition for toss winner failed",
				slog.String("file", path),
				slog.Int64("match_id", mid),
				slog.String("match_date", dateISO),
				slog.String("toss_winner", toss),
				slog.String("teams", fmt.Sprintf("%s vs %s", teamA, teamB)),
				slog.Any("err", e))
			return fmt.Errorf("get/create opposition for toss winner %q: %w", toss, e)
		}
		tossWinnerOppositionID = &id
	}
	scheduledOvers := scheduledOversFromFormatOrInfo(formatCode, info.Overs)
	eventName, eventStage, eventGroup := "", "", ""
	if info.Event != nil {
		eventName = info.Event.Name
		eventStage = strings.TrimSpace(info.Event.Stage)
		eventGroup = strings.TrimSpace(string(info.Event.Group))
	}
	matchInsert := &db.MatchInsert{
		MatchID:                   mid,
		FormatID:                  formatID,
		MatchDate:                 dateISO,
		OriginalMatchType:         info.MatchType,
		CompetitionLevel:          competitionLevel,
		MatchTypeNumber:           info.MatchTypeNumber,
		VenueID:                   venueID,
		SeasonID:                  seasonID,
		TossWinnerOppositionID:    tossWinnerOppositionID,
		TossDecision:              strPtrFromToss(info.Toss),
		OutcomeWinnerOppositionID: winnerID,
		OutcomeByRuns:             outcomeByRuns(info.Outcome),
		OutcomeByWickets:          outcomeByWickets(info.Outcome),
		Result:                    outcomeResult(info.Outcome),
		ResultMethod:              outcomeMethod(info.Outcome),
		EventName:                 strPtrNonEmpty(eventName),
		EventStage:                strPtrNonEmpty(eventStage),
		EventGroup:                strPtrNonEmpty(eventGroup),
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
	playedInnings := m.PlayedInnings()
	if skipped := len(m.Innings) - len(playedInnings); skipped > 0 {
		slog.Info("cricsheet: super over left out of the innings record",
			slog.String("file", path),
			slog.Int64("match_id", mid),
			slog.String("match_date", dateISO),
			slog.Int("super_over_innings", skipped))
	}
	for i, inng := range playedInnings {
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
		battingTeamOppositionID, err := identity.OppositionID(ctx, batTeam)
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
		bowlingTeamOppositionID, err := identity.OppositionID(ctx, oppTeam)
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
		overLegalBallsByBowler := map[string]map[int]int{}
		var fieldingEvents []db.FieldingEvent
		for _, over := range inng.Overs {
			overNo := over.Over
			perBowler := map[string]int{}
			perBowlerLegalBalls := map[string]int{}
			for ballIndex, d := range over.Deliveries {
				tr := d.Runs.Total
				runs += tr
				extras += d.Runs.Extras
				// The bowler's runs are not the delivery's total: byes, leg-byes and penalty
				// runs go to the innings but not to him. Until IMPORT-04 was fixed the whole
				// total was charged, and the runs, economy of every bowler who bowled to a
				// fumbling keeper carried the keeper's misses.
				bowlerRuns := d.RunsConcededByBowler()
				// The bowler's count, not the batter's: a no-ball is faced but is not one of
				// his six, and a wide is neither (IMPORT-05).
				legal := d.IsLegal()
				// A maiden is the same "runs charged to the bowler" as his Runs column
				// (bowlerRuns): byes, leg-byes and penalty runs stay off it, as IMPORT-04
				// established. The bug was gating this accumulation on `legal` -- a wide or
				// no-ball is illegal, so bowlerRuns off one (which does include it; only
				// byes/leg-byes/penalty are excluded) never reached the over's tally, and an
				// over that only conceded a wide read as scoreless (IMPORT-14). Every delivery
				// counts here, legal or not; the legal-ball count below is what tells
				// maidenCount a complete over from a partial one, so an innings cut short
				// mid-over is never mistaken for a maiden either.
				perBowler[d.Bowler] += bowlerRuns
				if legal {
					balls++
					perBowlerLegalBalls[d.Bowler]++
				}
				// What the delivery's wickets are to the scorecard: a run out is a wicket
				// lost and not the bowler's, a batter retired hurt is neither (IMPORT-06).
				wicketTally, err := d.TallyWickets(vocabulary)
				if err != nil {
					slog.Error("wicket kind not in the vocabulary",
						slog.String("file", path),
						slog.Int64("match_id", mid),
						slog.Int("inning", inningNo),
						slog.Int("over", overNo),
						slog.Int("ball", ballIndex+1),
						slog.Any("err", err))
					return fmt.Errorf("classify wickets: %w", err)
				}
				wkts += wicketTally.Dismissals
				if d.Wickets != nil && len(*d.Wickets) > 0 {
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

						// A "caught" dismissal with no named fielder is a catch by a substitute
						// the source could not name (469 in the archive): it is credited to
						// nobody, as the batting and rating paths already do. "Caught and
						// bowled" is its own kind and never reaches here, so falling back to
						// the bowler would credit him with a catch he did not take -- which is
						// what this did until the two rating sources were compared (P-3).
						var fNames []string
						if w.Fielders != nil {
							fNames = append(fNames, *w.Fielders...)
						}

						if len(fNames) > 0 {
							batterID, err := identity.PlayerID(ctx, w.PlayerOut)
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
								bid, err := identity.PlayerID(ctx, d.Bowler)
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
								fid, err := identity.PlayerID(ctx, fn)
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
					if d.FacedByBatter() {
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
					b.Runs += bowlerRuns
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
					b.Wickets += wicketTally.CreditedToBowler
					b.Wides += d.Extras.Wides
					b.NoBalls += d.Extras.NoBalls
				}
			}
			for bowler, t := range perBowler {
				if overTotalsByBowler[bowler] == nil {
					overTotalsByBowler[bowler] = map[int]int{}
				}
				overTotalsByBowler[bowler][overNo] += t
			}
			for bowler, n := range perBowlerLegalBalls {
				if overLegalBallsByBowler[bowler] == nil {
					overLegalBallsByBowler[bowler] = map[int]int{}
				}
				overLegalBallsByBowler[bowler][overNo] += n
			}
		}
		allFieldingEvents = append(allFieldingEvents, fieldingEvents)
		oversFloat := oversFromBalls(balls, ballsPerOver)
		rpo := float32(0)
		if balls > 0 {
			rpo = float32(float64(runs) / float64(balls) * float64(ballsPerOver))
		}
		targetRuns, targetOvers := chaseTarget(inng, inningNo, formatCode, playedInnings)
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
			TargetRuns:              targetRuns,
			TargetOvers:             targetOvers,
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
			pid, err := identity.PlayerID(ctx, name)
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
			maidens := maidenCount(overTotalsByBowler[name], overLegalBallsByBowler[name], ballsPerOver)
			o := oversFromBalls(s.Balls, ballsPerOver)
			econ := float32(0)
			if s.Balls > 0 {
				econ = float32(float64(s.Runs) / float64(s.Balls) * float64(ballsPerOver))
			}
			pid, err := identity.PlayerID(ctx, name)
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
	// Resolve the squads each side picked (requires cache; done before tx).
	matchPlayerRows, err := buildMatchPlayerRows(ctx, identity, m, mid, path, dateISO)
	if err != nil {
		return err
	}
	// Build ball event rows (requires cache; done before tx)
	ballEvents, err := BuildBallEventRows(ctx, identity, vocabulary, m, int(formatID), mid)
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
			pid, err := identity.PlayerID(ctx, name)
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
		// First, and before anything is written: a re-import replaces the match rather
		// than merging into whatever an earlier import of the same id left behind.
		if err := db.DeleteMatchFactsTx(ctx, tx, mid); err != nil {
			slog.Error("delete existing match rows failed",
				slog.String("file", matchCtx.file),
				slog.Int64("match_id", mid),
				slog.String("match_date", matchCtx.date),
				slog.String("teams", matchCtx.teams),
				slog.Any("err", err))
			return fmt.Errorf("delete existing match rows: %w", err)
		}
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
		if err := db.ReplaceMatchPlayersTx(ctx, tx, mid, matchPlayerRows); err != nil {
			slog.Error("replace match_player failed",
				slog.String("file", matchCtx.file),
				slog.Int64("match_id", mid),
				slog.String("match_date", matchCtx.date),
				slog.String("teams", matchCtx.teams),
				slog.Int("squad_size", len(matchPlayerRows)),
				slog.Any("err", err))
			return fmt.Errorf("replace match_player: %w", err)
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
		if len(ballEvents.Deliveries) > 0 {
			if err := db.InsertBallEventsTx(ctx, tx, ballEvents.Deliveries); err != nil {
				slog.Error("insert ball_event failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("rows_count", len(ballEvents.Deliveries)),
					slog.Any("err", err))
				return fmt.Errorf("insert ball events: %w", err)
			}
			if err := db.InsertBallEventWicketsTx(ctx, tx, ballEvents.Wickets); err != nil {
				slog.Error("insert ball_event_wicket failed",
					slog.String("file", matchCtx.file),
					slog.Int64("match_id", mid),
					slog.String("match_date", matchCtx.date),
					slog.Int("rows_count", len(ballEvents.Wickets)),
					slog.Any("err", err))
				return fmt.Errorf("insert ball event wickets: %w", err)
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
	// Only now: the file's rows are in the archive, so the spellings it used are spellings
	// the archive holds and may settle the display name (IMPORT-13).
	identity.SettleObservedNames()
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

// chaseTarget is what the batting side had to make, and in how many overs, for one innings:
// the runs to win and the over limit the chase was given, or nil for an innings that was
// not a chase.
//
// The archive's own figure comes first. Cricsheet writes `innings[].target` as {runs, overs}
// on the innings being chased, and those are the real numbers: `runs` is the score that wins
// (one more than the innings defended, or the Duckworth-Lewis figure when rain revised it),
// and `overs` is the limit that chase was given, which a rain revision also cuts. 18,264 of
// the 22,905 files carry one; on 983 of them the runs differ from first + 1 and on 1,541 the
// overs differ from the scheduled allotment, so deriving either from the first innings throws
// away every revised target in the archive. Only 971 of those matches name "D/L" in
// `info.outcome.method` -- that field says how the *result* was reached, not whether a target
// was revised -- so the revised target cannot be recovered from what is already stored.
//
// Where the file names no target, first + 1 is the arithmetic of a chase and is used for a
// limited-overs second innings, with no over limit because the archive states none (the
// match's scheduled_overs_per_innings already holds the allotment). A multi-day innings gets
// nothing: no file in the archive puts a target on one, and until IMPORT-11 every second
// innings of a Test or a first-class match was given the first innings' total as its target,
// which is not a target and is not even the lead.
func chaseTarget(inng Innings, inningNo int, formatCode string, played []Innings) (*int, *float32) {
	if inng.Target != nil {
		runs := inng.Target.Runs
		overs := float32(inng.Target.Overs)
		return &runs, &overs
	}
	if inningNo != 2 || formatCode == formats.CodeTest || len(played) == 0 {
		return nil, nil
	}
	runs := inningsRuns(played[0]) + 1
	return &runs, nil
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

// maidenCount reports how many of a bowler's overs were maidens: zero runs charged to the
// over (overRuns), and a complete over of legal balls (overLegalBalls == ballsPerOver) --
// not a partial one, such as an innings that ends mid-over. An over the bowler did not
// finish is never a maiden, whatever it conceded (IMPORT-14).
func maidenCount(overRuns map[int]int, overLegalBalls map[int]int, ballsPerOver int) int {
	c := 0
	for overNo, t := range overRuns {
		if t == 0 && overLegalBalls[overNo] == ballsPerOver {
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

// outcomeResult is Cricsheet's result verbatim ("draw", "no result", "tie"), nil for a
// match that was won outright, which Cricsheet writes with no result at all.
func outcomeResult(o *Outcome) *string {
	if o == nil {
		return nil
	}
	return strPtrNonEmpty(strings.TrimSpace(o.Result))
}

// outcomeMethod is the rule that adjusted or awarded the result ("D/L", "VJD",
// "Awarded", ...), nil where the archive names none.
func outcomeMethod(o *Outcome) *string {
	if o == nil {
		return nil
	}
	return strPtrNonEmpty(strings.TrimSpace(o.Method))
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
