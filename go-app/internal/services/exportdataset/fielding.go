package exportdataset

import (
	"context"
	"fmt"
	"io"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// FieldingExporter defines fielding export operations (unified and per-format only).
type FieldingExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportFormat(ctx context.Context, format string, w io.Writer) error
}

// FieldingService implements FieldingExporter using a DatasetRepo.
type FieldingService struct {
	Repo db.DatasetRepo
}

func NewFieldingService(r db.DatasetRepo) *FieldingService { return &FieldingService{Repo: r} }

func (s *FieldingService) ExportUnified(ctx context.Context, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.FieldingUnifiedRows(ctx)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func (s *FieldingService) ExportFormat(ctx context.Context, format string, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.FieldingFormatRows(ctx, format)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}
