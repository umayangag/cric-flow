package exportdataset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	
	"github.com/umayangag/cric-flow/go-app/internal/config"
	exq "github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)


// Runner orchestrates the export-dataset workflow behind interfaces for testability.
type Runner struct {
	Bat    BattingExporter
	Bow    BowlingExporter
	Field  FieldingExporter
	Extras ExtrasExporter
	Win    WinExporter
}

// NewRunner constructs a Runner with only filesystem dependency (backward compatible during migration).
func NewRunner() *Runner { return &Runner{} }

// perFormatExporter represents a model type that supports per-format (non-legacy) export.
type perFormatExporter struct {
	name     string
	exporter interface {
		ExportFormat(context.Context, string, io.Writer) error
	}
	enabled bool
}

// perFormatExporters returns the fielding/extras/win exporters for per-format loops.
func (r *Runner) perFormatExporters() []perFormatExporter {
	return []perFormatExporter{
		{"fielding", r.Field, r.Field != nil},
		{"extras", r.Extras, r.Extras != nil},
		{"win", r.Win, r.Win != nil},
	}
}

// addPerFormatExportGoroutines adds goroutines to g that export fielding/extras/win per-format CSVs.
func (r *Runner) addPerFormatExportGoroutines(
	parentCtx context.Context,
	g *errgroup.Group,
	outDir string,
	format string,
) {
	for _, fe := range r.perFormatExporters() {
		if !fe.enabled {
			continue
		}
		fe := fe
		g.Go(func() error {
			return r.writeUsing(
				outDir,
				fmt.Sprintf("%s_encoded_%s.csv", fe.name, format),
				func(w io.Writer) error { return fe.exporter.ExportFormat(parentCtx, format, w) },
			)
		})
	}
}

// NewRunnerWithServices constructs a Runner with filesystem and export services.
// Field, Extras, Win can be nil to skip their export (e.g. backward compatibility).
func NewRunnerWithServices(
	bat BattingExporter,
	bow BowlingExporter,
	field FieldingExporter,
	extras ExtrasExporter,
	win WinExporter,
) *Runner {
	return &Runner{Bat: bat, Bow: bow, Field: field, Extras: extras, Win: win}
}

// Run executes the export based on CLI options provided by the caller.
// For now it validates options and prepares the output directory; service orchestration will follow in subsequent phases.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	if r == nil {
		err := errors.New("nil runner")
		slog.Error("exportdataset.Runner.Run failed", slog.Any("err", err))
		return err
	}
	if opts.OutDir == "" {
		err := errors.New("output directory is required")
		slog.Error("exportdataset.Runner.Run failed", slog.Any("err", err))
		return err
	}
	if err := os.MkdirAll(opts.OutDir, fs.FileMode(0o755)); err != nil {
		// Fallback when cwd (or requested path) is not writable (e.g. API in Docker without GO_APP_OUTPUT_DIR).
		if isPermissionDenied(err) {
			fallback := filepath.Join(os.TempDir(), "cric-export", "go-app")
			if fallbackErr := os.MkdirAll(fallback, fs.FileMode(0o755)); fallbackErr == nil {
				slog.Info("exportdataset.Runner.Run using temp fallback (requested dir not writable)",
					slog.String("requested", opts.OutDir), slog.String("fallback", fallback))
				opts.OutDir = fallback
			} else {
				slog.Error("exportdataset.Runner.Run mkdir failed", slog.String("out_dir", opts.OutDir), slog.Any("err", err))
				return fmt.Errorf("mkdir %s: %w", opts.OutDir, err)
			}
		} else {
			slog.Error("exportdataset.Runner.Run mkdir failed", slog.String("out_dir", opts.OutDir), slog.Any("err", err))
			return fmt.Errorf("mkdir %s: %w", opts.OutDir, err)
		}
	}
	// If services are injected, orchestrate exports here. This path is only active
	// when Bat and Bow are non-nil. The existing main currently constructs the
	// runner without services, so behavior remains unchanged until wiring is added.
	if r.Bat != nil && r.Bow != nil {
		// Inject the exporter sequence flag into context so lower layers can gate joins.
		ctx = exq.WithSeqEnabled(ctx, opts.EnableSeq)
		// Use parent context for export calls so one failing goroutine does not cancel the
		// others (errgroup cancels its context on first error, which would cause cascading
		// "context canceled" in all in-flight exports).
		parentCtx := ctx
		formats := ResolveFormats(opts, config.Load())

		var g errgroup.Group

		if opts.Unified {
			slog.Info("pipeline: export-dataset exporting unified CSVs", slog.String("out_dir", opts.OutDir))
			type unifiedExporter struct {
				name     string
				exporter interface {
					ExportUnified(context.Context, io.Writer) error
				}
				enabled bool
			}
			exporters := []unifiedExporter{
				{"batting", r.Bat, r.Bat != nil},
				{"bowling", r.Bow, r.Bow != nil},
				{"fielding", r.Field, r.Field != nil},
				{"extras", r.Extras, r.Extras != nil},
				{"win", r.Win, r.Win != nil},
			}
			for _, exp := range exporters {
				if !exp.enabled {
					continue
				}
				e := exp
				g.Go(func() error {
					filename := fmt.Sprintf("%s_encoded_all.csv", e.name)
					slog.Info("pipeline: export-dataset exporting " + filename)
					return r.writeUsing(
						opts.OutDir,
						filename,
						func(w io.Writer) error { return e.exporter.ExportUnified(parentCtx, w) },
					)
				})
			}
			if err := g.Wait(); err != nil {
				slog.Error("exportdataset.Runner.Run unified export failed", slog.Any("err", err))
				return err
			}
			// When formats are requested (e.g. SplitByFormat), also write per-format CSVs
			// so per-format training (train_batting --all-formats, train_bowling --all-formats) has inputs.
			slog.Info("pipeline: export-dataset exporting per-format CSVs", slog.Any("formats", formats))
			for _, f := range formats {
				if f == "" || !safeFormatForFilename(f) {
					continue
				}
				f := f
				g.Go(func() error {
					return r.writeUsing(
						opts.OutDir,
						fmt.Sprintf("batting_encoded_%s.csv", f),
						func(w io.Writer) error { return r.Bat.ExportFormat(parentCtx, f, w) },
					)
				})
				g.Go(func() error {
					return r.writeUsing(
						opts.OutDir,
						fmt.Sprintf("bowling_encoded_%s.csv", f),
						func(w io.Writer) error { return r.Bow.ExportFormat(parentCtx, f, w) },
					)
				})
				r.addPerFormatExportGoroutines(parentCtx, &g, opts.OutDir, f)
			}
			if err := g.Wait(); err != nil {
				slog.Error("exportdataset.Runner.Run per-format export failed", slog.Any("err", err))
				return err
			}
			return nil
		}

		slog.Info("pipeline: export-dataset exporting legacy/per-format CSVs", slog.Any("formats", formats))
		for _, f := range formats {
			f := f // capture
			if f == "" {
				if opts.InferenceOnly {
					// Legacy note: inference-only requires explicit formats; skip combined.
					continue
				}
				g.Go(func() error {
					return r.writeUsing(
						opts.OutDir,
						"batting_encoded.csv",
						func(w io.Writer) error { return r.Bat.ExportLegacy(parentCtx, w) },
					)
				})
				g.Go(func() error {
					return r.writeUsing(
						opts.OutDir,
						"bowling_encoded.csv",
						func(w io.Writer) error { return r.Bow.ExportLegacy(parentCtx, w) },
					)
				})
				continue
			}
			if !safeFormatForFilename(f) {
				continue
			}
			if opts.InferenceOnly {
				g.Go(func() error {
					return r.writeUsing(
						opts.OutDir,
						fmt.Sprintf("batting_infer_%s.csv", f),
						func(w io.Writer) error { return r.Bat.ExportInference(parentCtx, f, w) },
					)
				})
				g.Go(func() error {
					return r.writeUsing(
						opts.OutDir,
						fmt.Sprintf("bowling_infer_%s.csv", f),
						func(w io.Writer) error { return r.Bow.ExportInference(parentCtx, f, w) },
					)
				})
				continue
			}
			// Per-format training exports (non-inference)
			g.Go(func() error {
				return r.writeUsing(
					opts.OutDir,
					fmt.Sprintf("batting_encoded_%s.csv", f),
					func(w io.Writer) error { return r.Bat.ExportFormat(parentCtx, f, w) },
				)
			})
			g.Go(func() error {
				return r.writeUsing(
					opts.OutDir,
					fmt.Sprintf("bowling_encoded_%s.csv", f),
					func(w io.Writer) error { return r.Bow.ExportFormat(parentCtx, f, w) },
				)
			})
			r.addPerFormatExportGoroutines(parentCtx, &g, opts.OutDir, f)
		}
		if err := g.Wait(); err != nil {
			slog.Error("exportdataset.Runner.Run format export failed", slog.Any("err", err))
			return err
		}
		return nil
	}
	return nil
}

func (r *Runner) writeUsing(outDir, name string, fn func(w io.Writer) error) error {
	slog.Info("pipeline: export-dataset writing", slog.String("file", name))
	var buf bytes.Buffer
	if err := fn(&buf); err != nil {
		slog.Error("exportdataset.writeUsing export failed", slog.String("name", name), slog.Any("err", err))
		return err
	}
	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, buf.Bytes(), fs.FileMode(0o644)); err != nil {
		slog.Error("exportdataset.writeUsing write failed", slog.String("path", path), slog.Any("err", err))
		return err
	}
	return nil
}

// safeFormatForFilename returns true if s is safe to use in an export filename (no path
// components or special chars), to prevent path traversal when writing per-format CSVs.
func safeFormatForFilename(s string) bool {
	if len(s) == 0 || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

// isPermissionDenied returns true if err indicates a permission denied (e.g. mkdir in a read-only dir).
func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrPermission) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "permission denied")
}
