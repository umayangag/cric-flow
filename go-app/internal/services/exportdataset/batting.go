package exportdataset

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// BattingExporter defines batting export operations.
type BattingExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportInference(ctx context.Context, format string, w io.Writer) error
	ExportFormat(ctx context.Context, format string, w io.Writer) error
}

// BattingService implements BattingExporter using a DatasetRepo.
type BattingService struct {
	Repo db.DatasetRepo
}

func NewBattingService(r db.DatasetRepo) *BattingService { return &BattingService{Repo: r} }

func (s *BattingService) ExportUnified(ctx context.Context, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.BattingUnifiedRows(ctx)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func (s *BattingService) ExportInference(ctx context.Context, format string, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.BattingInferenceRows(ctx, format)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func (s *BattingService) ExportFormat(ctx context.Context, format string, w io.Writer) error {
	if s == nil || s.Repo == nil {
		return fmt.Errorf("nil service or repo")
	}
	rows, err := s.Repo.BattingFormatRows(ctx, format)
	if err != nil {
		return err
	}
	return writeCSV(w, rows)
}

func writeCSV(w io.Writer, rows [][]string) error {
	cw := csv.NewWriter(w)
	for _, r := range rows {
		if err := cw.Write(r); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
