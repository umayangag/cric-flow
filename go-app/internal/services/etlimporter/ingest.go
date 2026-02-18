package etlimporter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/resources"
)

// Stats summarizes the ingestion results.
type Stats struct {
	Files       int
	BattingRows int
	BowlingRows int
}

// Service coordinates reading curated CSVs (via FS), parsing, and optionally upserting via Repository.
// It is deterministic and testable; no logging here.
type Service struct {
	Repository db.EtlRepository
}

func NewService(repository db.EtlRepository) *Service {
	return &Service{
		Repository: repository,
	}
}

// IngestDir enumerates files by pattern in dir, parses them concurrently, and when apply==true
// upserts per file to avoid loading all data into memory. conc limits parallel workers; 0 uses resource-aware limit.
func (s *Service) IngestDir(ctx context.Context, dir, pattern string, apply bool, conc int) (Stats, error) {
	if s == nil || s.Repository == nil {
		return Stats{}, errors.New("nil service or dependency")
	}
	if strings.TrimSpace(dir) == "" {
		return Stats{}, errors.New("input directory required")
	}
	if conc <= 0 {
		conc = resources.GetLimit(resources.KindImport)
	}
	if conc < 1 {
		conc = 1
	}
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return Stats{}, err
	}
	st := Stats{Files: len(matches)}
	slog.Info(
		"starting ETL ingestion",
		slog.Int("files", st.Files),
		slog.String("dir", dir),
		slog.String("pattern", pattern),
		slog.Bool("apply", apply),
		slog.Int("concurrency", conc),
	)

	var mu sync.Mutex
	totalBat, totalBowl := 0, 0
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(conc)

	for _, p := range matches {
		p := p
		g.Go(func() error {
			if err := gCtx.Err(); err != nil {
				return nil
			}
			slog.Info("processing ETL file", slog.String("path", p))
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			bat, berr := ParseBattingCSV(bytes.NewReader(b))
			if berr == nil {
				mu.Lock()
				totalBat += len(bat)
				mu.Unlock()
				if apply && len(bat) > 0 {
					if err := s.Repository.UpsertBatting(gCtx, bat); err != nil {
						return fmt.Errorf("upsert batting %s: %w", filepath.Base(p), err)
					}
				}
				return nil
			}
			bowl, werr := ParseBowlingCSV(bytes.NewReader(b))
			if werr == nil {
				mu.Lock()
				totalBowl += len(bowl)
				mu.Unlock()
				if apply && len(bowl) > 0 {
					if err := s.Repository.UpsertBowling(gCtx, bowl); err != nil {
						return fmt.Errorf("upsert bowling %s: %w", filepath.Base(p), err)
					}
				}
				return nil
			}
			return fmt.Errorf("failed to parse %s as batting (%w) or bowling (%w)", filepath.Base(p), berr, werr)
		})
	}

	if err := g.Wait(); err != nil {
		return st, err
	}
	st.BattingRows = totalBat
	st.BowlingRows = totalBowl
	slog.Info(
		"ETL ingestion finished",
		slog.Int("files", st.Files),
		slog.Int("batting_rows", st.BattingRows),
		slog.Int("bowling_rows", st.BowlingRows),
	)
	return st, nil
}
