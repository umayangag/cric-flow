package etlimporter

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/fsx"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Stats summarizes the ingestion results.
type Stats struct {
	Files       int
	BattingRows int
	BowlingRows int
}

// Service coordinates reading curated CSVs (via FS), parsing, and optionally upserting via Repo.
// It is deterministic and testable; no logging here.
type Service struct {
	FS   fsx.FS
	Repo db.EtlRepo
}

func NewService(fs fsx.FS, repo db.EtlRepo) *Service { return &Service{FS: fs, Repo: repo} }

// IngestDir enumerates files by pattern in dir, parses them, and when apply==true upserts via Repo.
// conc is reserved for future use; it must be >=1 (validated by the caller/CLI). For simplicity and
// determinism we currently process sequentially.
func (s *Service) IngestDir(ctx context.Context, dir, pattern string, apply bool, conc int) (Stats, error) {
	if s == nil || s.FS == nil || s.Repo == nil {
		return Stats{}, errors.New("nil service or dependency")
	}
	if strings.TrimSpace(dir) == "" {
		return Stats{}, errors.New("input directory required")
	}
	if conc < 1 {
		return Stats{}, errors.New("concurrency must be >= 1")
	}
	matches, err := s.FS.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return Stats{}, err
	}
	st := Stats{Files: len(matches)}
	var allBat []db.EtlBattingRow
	var allBowl []db.EtlBowlingRow
	for _, p := range matches {
		b, rerr := s.FS.ReadFile(ctx, p)
		if rerr != nil {
			return Stats{}, rerr
		}
		// Decide parser by header
		bat, berr := ParseBattingCSV(bytes.NewReader(b))
		if berr == nil {
			st.BattingRows += len(bat)
			allBat = append(allBat, bat...)
			continue
		}
		bowl, werr := ParseBowlingCSV(bytes.NewReader(b))
		if werr == nil {
			st.BowlingRows += len(bowl)
			allBowl = append(allBowl, bowl...)
			continue
		}
		// If neither parsed, return first error (batting) for diagnosability
return Stats{}, fmt.Errorf("failed to parse as batting (%w) or bowling (%w)", berr, werr)
	}
	if apply {
		if len(allBat) > 0 {
			if err := s.Repo.UpsertBatting(ctx, allBat); err != nil {
				return Stats{}, err
			}
		}
		if len(allBowl) > 0 {
			if err := s.Repo.UpsertBowling(ctx, allBowl); err != nil {
				return Stats{}, err
			}
		}
	}
	return st, nil
}
