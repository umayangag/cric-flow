package seqcalc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
)

func seqcalcConcurrency() int {
	if v := os.Getenv("SEQCALC_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			return n
		}
	}
	return 1
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

// Run executes calculators with the given params concurrently.
// Concurrency is controlled by SEQCALC_CONCURRENCY env (default 1) to avoid OOM from
// multiple concurrent ball_event scans. Increase on machines with ample memory.
func Run(ctx context.Context, calcs []Calculator, params Params, dry bool) error {
	g, ctx := errgroup.WithContext(ctx)
	limit := seqcalcConcurrency()
	g.SetLimit(limit)

	for _, c := range calcs {
		calc := c // capture for goroutine
		name := calc.Name()
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return nil
			}
			slog.Info("seqcalc.calculator.start", slog.String("calculator", string(name)), slog.String("format", params.FormatCode))
			err := calc.Compute(ctx, params, dry)
			if err != nil {
				// Log immediately so we see DB cancel/timeout in go-app logs (Postgres may show "terminating parallel worker").
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					slog.Error("seqcalc.calculator.cancelled_or_timeout",
						slog.String("calculator", string(name)),
						slog.String("format", params.FormatCode),
						slog.Any("err", err),
					)
				} else {
					slog.Error("seqcalc.calculator.failed",
						slog.String("calculator", string(name)),
						slog.String("format", params.FormatCode),
						slog.Any("err", err),
					)
				}
				return err
			}
			slog.Info("seqcalc.calculator.done", slog.String("calculator", string(name)), slog.String("format", params.FormatCode))
			return nil
		})
	}
	err := g.Wait()
	if err != nil {
		slog.Error("seqcalc.run.finished_with_error", slog.String("format", params.FormatCode), slog.Any("err", err))
	}
	return err
}
