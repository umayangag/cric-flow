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

	cli "github.com/umayangag/cric-flow/go-app/internal/cli/exportdataset"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	exq "github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

// BattingExporter is the minimal interface Runner needs for batting exports.
type BattingExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportLegacy(ctx context.Context, w io.Writer) error
	ExportInference(ctx context.Context, format string, w io.Writer) error
	ExportFormat(ctx context.Context, format string, w io.Writer) error
}

// BowlingExporter is the minimal interface Runner needs for bowling exports.
type BowlingExporter interface {
	ExportUnified(ctx context.Context, w io.Writer) error
	ExportLegacy(ctx context.Context, w io.Writer) error
	ExportInference(ctx context.Context, format string, w io.Writer) error
	ExportFormat(ctx context.Context, format string, w io.Writer) error
}

// Runner orchestrates the export-dataset workflow behind interfaces for testability.
type Runner struct {
	Bat BattingExporter
	Bow BowlingExporter
}

// NewRunner constructs a Runner with only filesystem dependency (backward compatible during migration).
func NewRunner() *Runner { return &Runner{} }

// NewRunnerWithServices constructs a Runner with filesystem and export services.
func NewRunnerWithServices(bat BattingExporter, bow BowlingExporter) *Runner {
	return &Runner{Bat: bat, Bow: bow}
}

// Run executes the export based on CLI options provided by the caller.
// For now it validates options and prepares the output directory; service orchestration will follow in subsequent phases.
func (r *Runner) Run(ctx context.Context, opts cli.Options) error {
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
			g.Go(func() error {
				name := "batting_encoded_all.csv"
				err := r.writeUsing(opts.OutDir, name, func(w io.Writer) error { return r.Bat.ExportUnified(parentCtx, w) })
				if err != nil {
					slog.Error("exportdataset.Runner.Run export failed", "file", name, slog.Any("err", err))
					return err
				}
				return nil
			})
			g.Go(func() error {
				name := "bowling_encoded_all.csv"
				err := r.writeUsing(opts.OutDir, name, func(w io.Writer) error { return r.Bow.ExportUnified(parentCtx, w) })
				if err != nil {
					slog.Error("exportdataset.Runner.Run export failed", "file", name, slog.Any("err", err))
					return err
				}
				return nil
			})
			if err := g.Wait(); err != nil {
				slog.Error("exportdataset.Runner.Run unified export failed", slog.Any("err", err))
				return err
			}
			// When formats are requested (e.g. SplitByFormat), also write per-format CSVs
			// so per-format training (train_batting --all-formats, train_bowling --all-formats) has inputs.
			for _, f := range formats {
				if f == "" || !safeFormatForFilename(f) {
					continue
				}
				f := f
				g.Go(func() error {
					bat := fmt.Sprintf("batting_encoded_%s.csv", f)
					err := r.writeUsing(opts.OutDir, bat, func(w io.Writer) error { return r.Bat.ExportFormat(parentCtx, f, w) })
					if err != nil {
						slog.Error("exportdataset.Runner.Run export failed", "file", bat, slog.Any("err", err))
						return err
					}
					return nil
				})
				g.Go(func() error {
					bow := fmt.Sprintf("bowling_encoded_%s.csv", f)
					err := r.writeUsing(opts.OutDir, bow, func(w io.Writer) error { return r.Bow.ExportFormat(parentCtx, f, w) })
					if err != nil {
						slog.Error("exportdataset.Runner.Run export failed", "file", bow, slog.Any("err", err))
						return err
					}
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				slog.Error("exportdataset.Runner.Run per-format export failed", slog.Any("err", err))
				return err
			}
			return nil
		}

		for _, f := range formats {
			f := f // capture
			if f == "" {
				if opts.InferenceOnly {
					// Legacy note: inference-only requires explicit formats; skip combined.
					continue
				}
				g.Go(func() error {
					err := r.writeUsing(opts.OutDir, "batting_encoded.csv", func(w io.Writer) error { return r.Bat.ExportLegacy(parentCtx, w) })
					if err != nil {
						slog.Error("exportdataset.Runner.Run export failed", "file", "batting_encoded.csv", slog.Any("err", err))
						return err
					}
					return nil
				})
				g.Go(func() error {
					err := r.writeUsing(opts.OutDir, "bowling_encoded.csv", func(w io.Writer) error { return r.Bow.ExportLegacy(parentCtx, w) })
					if err != nil {
						slog.Error("exportdataset.Runner.Run export failed", "file", "bowling_encoded.csv", slog.Any("err", err))
						return err
					}
					return nil
				})
				continue
			}
			if !safeFormatForFilename(f) {
				continue
			}
			if opts.InferenceOnly {
				g.Go(func() error {
					bat := fmt.Sprintf("batting_infer_%s.csv", f)
					err := r.writeUsing(opts.OutDir, bat, func(w io.Writer) error { return r.Bat.ExportInference(parentCtx, f, w) })
					if err != nil {
						slog.Error("exportdataset.Runner.Run export failed", "file", bat, slog.Any("err", err))
						return err
					}
					return nil
				})
				g.Go(func() error {
					bow := fmt.Sprintf("bowling_infer_%s.csv", f)
					err := r.writeUsing(opts.OutDir, bow, func(w io.Writer) error { return r.Bow.ExportInference(parentCtx, f, w) })
					if err != nil {
						slog.Error("exportdataset.Runner.Run export failed", "file", bow, slog.Any("err", err))
						return err
					}
					return nil
				})
				continue
			}
			// Per-format training exports (non-inference)
			g.Go(func() error {
				bat := fmt.Sprintf("batting_encoded_%s.csv", f)
				err := r.writeUsing(opts.OutDir, bat, func(w io.Writer) error { return r.Bat.ExportFormat(parentCtx, f, w) })
				if err != nil {
					slog.Error("exportdataset.Runner.Run export failed", "file", bat, slog.Any("err", err))
					return err
				}
				return nil
			})
			g.Go(func() error {
				bow := fmt.Sprintf("bowling_encoded_%s.csv", f)
				err := r.writeUsing(opts.OutDir, bow, func(w io.Writer) error { return r.Bow.ExportFormat(parentCtx, f, w) })
				if err != nil {
					slog.Error("exportdataset.Runner.Run export failed", "file", bow, slog.Any("err", err))
					return err
				}
				return nil
			})
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
