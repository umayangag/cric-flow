package exportdataset

import (
	"context"
	"fmt"
	"io"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// BowlingExporter defines bowling export operations.
type BowlingExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportLegacy(ctx context.Context, w io.Writer) error
	ExportInference(ctx context.Context, format string, w io.Writer) error
}

// BowlingService implements BowlingExporter using a DatasetRepo.
type BowlingService struct {
	Repo db.DatasetRepo
}

func NewBowlingService(r db.DatasetRepo) *BowlingService { return &BowlingService{Repo: r} }

func (s *BowlingService) ExportUnified(ctx context.Context, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.BowlingUnifiedRows(ctx)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func (s *BowlingService) ExportLegacy(ctx context.Context, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.BowlingLegacyRows(ctx)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func (s *BowlingService) ExportInference(ctx context.Context, format string, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.BowlingInferenceRows(ctx, format)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

// writeCSV is shared from batting.go in the same package.
