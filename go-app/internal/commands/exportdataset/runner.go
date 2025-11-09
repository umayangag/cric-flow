package exportdataset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/fsx"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// BattingExporter is the minimal interface Runner needs for batting exports.
type BattingExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportLegacy(ctx context.Context, w io.Writer) error
	ExportInference(ctx context.Context, format string, w io.Writer) error
}

// BowlingExporter is the minimal interface Runner needs for bowling exports.
type BowlingExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportLegacy(ctx context.Context, w io.Writer) error
	ExportInference(ctx context.Context, format string, w io.Writer) error
}

// Runner orchestrates the export-dataset workflow behind interfaces for testability.
type Runner struct {
	FS  fsx.FS
	Bat BattingExporter
	Bow BowlingExporter
}

// NewRunner constructs a Runner with only filesystem dependency (backward compatible during migration).
func NewRunner(files fsx.FS) *Runner { return &Runner{FS: files} }

// NewRunnerWithServices constructs a Runner with filesystem and export services.
func NewRunnerWithServices(files fsx.FS, bat BattingExporter, bow BowlingExporter) *Runner {
	return &Runner{FS: files, Bat: bat, Bow: bow}
}

// Run executes the export based on CLI options provided by the caller.
// For now it validates options and prepares the output directory; service orchestration will follow in subsequent phases.
func (r *Runner) Run(ctx context.Context, opts cli.Options) error {
	if r == nil {
		return errors.New("nil runner")
	}
	if opts.OutDir == "" {
		return errors.New("output directory is required")
	}
	if r.FS == nil {
		return errors.New("missing FS dependency")
	}
	if err := r.FS.MkdirAll(opts.OutDir, fs.FileMode(0o755)); err != nil {
		return fmt.Errorf("mkdir %s: %w", opts.OutDir, err)
	}
	// If services are injected, orchestrate exports here. This path is only active
	// when Bat and Bow are non-nil. The existing main currently constructs the
	// runner without services, so behavior remains unchanged until wiring is added.
	if r.Bat != nil && r.Bow != nil {
		formats := ResolveFormats(opts, config.Load())
		if opts.Unified {
			if err := r.writeUsing(opts.OutDir, "batting_encoded_all.csv", func(w io.Writer) error { return r.Bat.ExportUnified(ctx, w) }); err != nil { return err }
			if err := r.writeUsing(opts.OutDir, "bowling_encoded_all.csv", func(w io.Writer) error { return r.Bow.ExportUnified(ctx, w) }); err != nil { return err }
			return nil
		}
		for _, f := range formats {
			if f == "" {
				if opts.InferenceOnly {
					// Legacy note: inference-only requires explicit formats; skip combined.
					continue
				}
				if err := r.writeUsing(opts.OutDir, "batting_encoded.csv", func(w io.Writer) error { return r.Bat.ExportLegacy(ctx, w) }); err != nil { return err }
				if err := r.writeUsing(opts.OutDir, "bowling_encoded.csv", func(w io.Writer) error { return r.Bow.ExportLegacy(ctx, w) }); err != nil { return err }
				continue
			}
			if opts.InferenceOnly {
				bat := fmt.Sprintf("batting_infer_%s.csv", f)
				bow := fmt.Sprintf("bowling_infer_%s.csv", f)
				if err := r.writeUsing(opts.OutDir, bat, func(w io.Writer) error { return r.Bat.ExportInference(ctx, f, w) }); err != nil { return err }
				if err := r.writeUsing(opts.OutDir, bow, func(w io.Writer) error { return r.Bow.ExportInference(ctx, f, w) }); err != nil { return err }
				continue
			}
			// Per-format training exports (non-inference) remain in cmd for now.
		}
	}
	return nil
}

func (r *Runner) writeUsing(outDir, name string, fn func(w io.Writer) error) error {
	var buf bytes.Buffer
	if err := fn(&buf); err != nil { return err }
	path := filepath.Join(outDir, name)
	if r.FS == nil { return errors.New("missing FS dependency") }
	return r.FS.WriteFile(context.Background(), path, buf.Bytes(), fs.FileMode(0o644))
}
