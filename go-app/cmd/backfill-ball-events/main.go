package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// options holds CLI flags for backfilling ball_event rows.
type options struct {
	format      string
	season      string
	matchID     int64
	concurrency int
	dryRun      bool
	inDir       string
}

func parseFlags(args []string) (*options, error) {
	fs := flag.NewFlagSet("backfill-ball-events", flag.ContinueOnError)
	fs.SetOutput(new(noopWriter)) // keep tests quiet
	var (
		format  = fs.String("format", "", "Match format code to backfill (e.g., T20)")
		season  = fs.String("season", "", "Optional season filter (e.g., 2019)")
		matchID = fs.Int64("match-id", 0, "Optional specific stable match ID to backfill")
		conc    = fs.Int("concurrency", 1, "Concurrency level (currently only 1 is supported deterministically)")
		dry     = fs.Bool("dry-run", false, "Plan only; do not write to DB")
		inDirF  = fs.String(
			"in",
			"",
			"Input directory of Cricsheet JSONs (default from CRICSHEET_DIR or data/cricsheet)",
		)
	)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(*format) == "" {
		return nil, errors.New("-format is required (e.g., -format=T20)")
	}
	// Normalize common aliases
	fmtNorm := strings.ToUpper(strings.TrimSpace(*format))
	if fmtNorm == "T20I" {
		fmtNorm = "T20" // treat T20I as T20 for backfill scope in this sub-plan
	}
	inDir := strings.TrimSpace(*inDirF)
	if inDir == "" {
		inDir = os.Getenv("CRICSHEET_DIR")
	}
	if inDir == "" {
		inDir = filepath.Join("..", "..", "data", "cricsheet")
	}
	return &options{
		format:      fmtNorm,
		season:      strings.TrimSpace(*season),
		matchID:     *matchID,
		concurrency: *conc,
		dryRun:      *dry,
		inDir:       inDir,
	}, nil
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:]); err != nil {
		log.Fatalf("backfill failed: %v", err)
	}
}

func run(ctx context.Context, args []string) error {
	opts, err := parseFlags(args)
	if err != nil {
		return err
	}
	if opts.concurrency != 1 {
		log.Printf(
			"[warn] only concurrency=1 is supported currently; proceeding sequentially (requested %d)",
			opts.concurrency,
		)
	}

	// Connect DB
	if _, err := db.Connect(ctx); err != nil {
		return fmt.Errorf("db connect: %w", err)
	}

	formatID, err := db.GetMatchFormatIDByCode(ctx, opts.format)
	if err != nil {
		return fmt.Errorf("lookup format_id for %s: %w", opts.format, err)
	}

	// Discover input files
	entries, err := os.ReadDir(opts.inDir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", opts.inDir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".json") {
			files = append(files, filepath.Join(opts.inDir, name))
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		log.Printf("no files found in %s", opts.inDir)
		return nil
	}

	cfg := config.Load()
	start := time.Now()
	total := 0
	for _, f := range files {
		m, meta, perr := parseMatchFile(f)
		if perr != nil {
			log.Printf("[error] parse failed %s: %v", filepath.Base(f), perr)
			continue
		}
		fmtCode := cricsheet.DetectFormat(meta.matchType, meta.teams, cfg)
		if strings.ToUpper(fmtCode) != opts.format {
			continue
		}
		if opts.season != "" && strings.TrimSpace(string(meta.season)) != opts.season {
			continue
		}
		stableID := cricsheet.StableMatchID(meta.dateISO, meta.teamA, meta.teamB)
		if opts.matchID > 0 && stableID != opts.matchID {
			continue
		}
		if opts.dryRun {
			log.Printf("[dry] would backfill match_id=%d from %s", stableID, filepath.Base(f))
			total++
			continue
		}
		if err := db.EnsureMatchWithFormat(ctx, stableID, formatID); err != nil {
			log.Printf("[error] ensure match failed id=%d: %v", stableID, err)
			continue
		}
		if err := cricsheet.EmitBallEvents(ctx, m, int(formatID), stableID); err != nil {
			log.Printf("[error] emit failed for id=%d: %v", stableID, err)
			continue
		}
		log.Printf("[ok] backfilled ball_event for id=%d from %s", stableID, filepath.Base(f))
		total++
	}
	log.Printf("done. matches=%d elapsed=%s", total, time.Since(start).Round(time.Millisecond))
	return nil
}

type matchMeta struct {
	dateISO   string
	teams     []string
	teamA     string
	teamB     string
	matchType string
	season    cricsheet.Season
}

func parseMatchFile(path string) (*cricsheet.Match, matchMeta, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, matchMeta{}, err
	}
	defer fh.Close()
	m, err := cricsheet.Parse(fh)
	if err != nil {
		return nil, matchMeta{}, err
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
	return m, matchMeta{
		dateISO:   dateISO,
		teams:     info.Teams,
		teamA:     teamA,
		teamB:     teamB,
		matchType: info.MatchType,
		season:    info.Season,
	}, nil
}
