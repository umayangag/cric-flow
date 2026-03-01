package exportdataset

import (
	"context"
	"fmt"
	"io"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// ExtrasExporter defines extras export operations (unified and per-format only).
type ExtrasExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportFormat(ctx context.Context, format string, w io.Writer) error
}

// ExtrasService implements ExtrasExporter using a DatasetRepo.
type ExtrasService struct {
	Repo db.DatasetRepo
}

func NewExtrasService(r db.DatasetRepo) *ExtrasService { return &ExtrasService{Repo: r} }

func (s *ExtrasService) ExportUnified(ctx context.Context, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.ExtrasUnifiedRows(ctx)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func (s *ExtrasService) ExportFormat(ctx context.Context, format string, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.ExtrasFormatRows(ctx, format)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}
