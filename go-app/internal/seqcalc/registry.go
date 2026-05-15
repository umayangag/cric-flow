package seqcalc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/umayangag/cric-flow/go-app/internal/resources"
)

func seqcalcConcurrency() int {
	return resources.GetLimit(resources.KindSeqCalc)
}

// Registry holds available calculators keyed by target name.
type Registry struct {
	calcs map[Target]Calculator
}

// NewRegistry creates a registry with provided calculators.
func NewRegistry(calcs ...Calculator) *Registry {
	r := &Registry{calcs: make(map[Target]Calculator)}
	for _, c := range calcs {
		if c == nil {
			continue
		}
		r.calcs[c.Name()] = c
	}
	return r
}

// AllTargets returns a sorted list of known targets (excluding "all").
func (r *Registry) AllTargets() []Target {
	keys := make([]Target, 0, len(r.calcs))
	for k := range r.calcs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// ResolveTargets parses a comma separated list of target names into calculators.
// Special token "all" expands to all known calculators.
func (r *Registry) ResolveTargets(spec string) ([]Calculator, error) {
	if strings.TrimSpace(spec) == "" || strings.EqualFold(spec, string(TargetAll)) {
		// all
		ts := r.AllTargets()
		out := make([]Calculator, 0, len(ts))
		for _, t := range ts {
			out = append(out, r.calcs[t])
		}
		return out, nil
	}
	names := strings.Split(spec, ",")
	out := make([]Calculator, 0, len(names))
	for _, n := range names {
		name := Target(strings.TrimSpace(strings.ToLower(n)))
		if name == "" {
			continue
		}
		// compare against keys case-insensitively
		var found Calculator
		for k, c := range r.calcs {
			if strings.EqualFold(string(k), string(name)) {
				found = c
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("unknown target: %s", n)
		}
		out = append(out, found)
	}
	return out, nil
}

// DryRun prints planned computations for the given calculators and params.
func DryRun(w io.Writer, calcs []Calculator, params Params) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if len(calcs) == 0 {
		return errors.New("no calculators to run")
	}
	fmt.Fprintf(w, "precompute (dry-run) format=%s targets=", params.FormatCode)
	for i, c := range calcs {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprint(w, c.Name())
	}
	fmt.Fprintln(w)
	return nil
}

// Run executes calculators with the given params. When concurrency is 1, runs them sequentially
// in the current goroutine to avoid spawning one goroutine per calculator (reduces num_goroutine
// and keeps memory footprint lower). When concurrency > 1, uses errgroup with SetLimit(limit).
func Run(ctx context.Context, calcs []Calculator, params Params, dry bool) error {
	limit := seqcalcConcurrency()
	slog.Info(
		"seqcalc: starting run",
		slog.Int("calculators", len(calcs)),
		slog.String("format", params.FormatCode),
		slog.Int("concurrency", limit),
	)
	resources.LogMemoryAndGoroutines(
		"seqcalc: memory and goroutines at start",
		slog.String("format", params.FormatCode),
		slog.Int("concurrency", limit),
	)

	if limit <= 1 {
		return runSequential(ctx, calcs, params, dry)
	}
	return runConcurrent(ctx, calcs, params, dry, limit)
}

// seqcalcWorkItem is one (params, calculator) unit for the multi-format pool.
type seqcalcWorkItem struct {
	params Params
	calc   Calculator
}

// RunMultiFormat runs all calculators for all params (e.g. one Params per format) using a single
// shared worker pool. When one format has fewer calculators or finishes early, workers take
// (format, calculator) work from others instead of sitting idle.
func RunMultiFormat(ctx context.Context, calcs []Calculator, paramsList []Params, dry bool, limit int) error {
	if len(paramsList) == 0 || len(calcs) == 0 {
		return nil
	}
	if limit < 1 {
		limit = seqcalcConcurrency()
	}
	if limit < 1 {
		limit = 1
	}
	totalWork := len(paramsList) * len(calcs)
	slog.Info(
		"seqcalc: starting multi-format run",
		slog.Int("formats", len(paramsList)),
		slog.Int("calculators", len(calcs)),
		slog.Int("work_items", totalWork),
		slog.Int("concurrency", limit),
	)
	resources.LogMemoryAndGoroutines(
		"seqcalc: memory and goroutines at start (multi-format)",
		slog.Int("concurrency", limit),
	)

	workCh := make(chan seqcalcWorkItem, totalWork)
	for _, p := range paramsList {
		for _, c := range calcs {
			workCh <- seqcalcWorkItem{params: p, calc: c}
		}
	}
	close(workCh)

	g, gCtx := errgroup.WithContext(ctx)
	for i := 0; i < limit; i++ {
		g.Go(func() error {
			for item := range workCh {
				if gCtx.Err() != nil {
					return nil
				}
				name := item.calc.Name()
				slog.Info(
					"seqcalc.calculator.start",
					slog.String("calculator", string(name)),
					slog.String("format", item.params.FormatCode),
				)
				err := item.calc.Compute(gCtx, item.params, dry)
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						slog.Error(
							"seqcalc.calculator.cancelled_or_timeout",
							slog.String("calculator", string(name)),
							slog.String("format", item.params.FormatCode),
							slog.Any("err", err),
						)
					} else {
						slog.Error(
							"seqcalc.calculator.failed",
							slog.String("calculator", string(name)),
							slog.String("format", item.params.FormatCode),
							slog.Any("err", err),
						)
					}
					return err
				}
				slog.Info(
					"seqcalc.calculator.done",
					slog.String("calculator", string(name)),
					slog.String("format", item.params.FormatCode),
				)
			}
			return nil
		})
	}
	err := g.Wait()
	resources.LogMemoryAndGoroutines(
		"seqcalc: memory and goroutines at end (multi-format)",
		slog.Int("concurrency", limit),
	)
	resources.RecordWorkerMemorySample(resources.KindSeqCalc, limit)
	if err != nil {
		slog.Error("seqcalc.run.multi_format_finished_with_error", slog.Any("err", err))
	}
	return err
}

func runSequential(ctx context.Context, calcs []Calculator, params Params, dry bool) error {
	for _, calc := range calcs {
		if err := ctx.Err(); err != nil {
			return nil
		}
		name := calc.Name()
		slog.Info(
			"seqcalc.calculator.start",
			slog.String("calculator", string(name)),
			slog.String("format", params.FormatCode),
		)
		err := calc.Compute(ctx, params, dry)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				slog.Error(
					"seqcalc.calculator.cancelled_or_timeout",
					slog.String("calculator", string(name)),
					slog.String("format", params.FormatCode),
					slog.Any("err", err),
				)
			} else {
				slog.Error(
					"seqcalc.calculator.failed",
					slog.String("calculator", string(name)),
					slog.String("format", params.FormatCode),
					slog.Any("err", err),
				)
			}
			return err
		}
		slog.Info(
			"seqcalc.calculator.done",
			slog.String("calculator", string(name)),
			slog.String("format", params.FormatCode),
		)
	}
	resources.LogMemoryAndGoroutines("seqcalc: memory and goroutines at end", slog.String("format", params.FormatCode))
	resources.RecordWorkerMemorySample(resources.KindSeqCalc, 1) // sequential run = 1 worker
	return nil
}

func runConcurrent(ctx context.Context, calcs []Calculator, params Params, dry bool, limit int) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for _, c := range calcs {
		calc := c
		name := calc.Name()
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return nil
			}
			slog.Info(
				"seqcalc.calculator.start",
				slog.String("calculator", string(name)),
				slog.String("format", params.FormatCode),
			)
			err := calc.Compute(ctx, params, dry)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					slog.Error(
						"seqcalc.calculator.cancelled_or_timeout",
						slog.String("calculator", string(name)),
						slog.String("format", params.FormatCode),
						slog.Any("err", err),
					)
				} else {
					slog.Error(
						"seqcalc.calculator.failed",
						slog.String("calculator", string(name)),
						slog.String("format", params.FormatCode),
						slog.Any("err", err),
					)
				}
				return err
			}
			slog.Info(
				"seqcalc.calculator.done",
				slog.String("calculator", string(name)),
				slog.String("format", params.FormatCode),
			)
			return nil
		})
	}
	err := g.Wait()
	resources.LogMemoryAndGoroutines("seqcalc: memory and goroutines at end", slog.String("format", params.FormatCode))
	resources.RecordWorkerMemorySample(resources.KindSeqCalc, limit)
	if err != nil {
		slog.Error("seqcalc.run.finished_with_error", slog.String("format", params.FormatCode), slog.Any("err", err))
	}
	return err
}
