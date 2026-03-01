package exportdataset

import (
	"context"
	"fmt"
	"io"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// WinExporter defines win export operations (unified and per-format only).
type WinExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportFormat(ctx context.Context, format string, w io.Writer) error
}

// WinService implements WinExporter using a DatasetRepo.
type WinService struct {
	Repo db.DatasetRepo
}

func NewWinService(r db.DatasetRepo) *WinService { return &WinService{Repo: r} }

func (s *WinService) ExportUnified(ctx context.Context, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.WinUnifiedRows(ctx)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func (s *WinService) ExportFormat(ctx context.Context, format string, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.WinFormatRows(ctx, format)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}
