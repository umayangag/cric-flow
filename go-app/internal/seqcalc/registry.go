package seqcalc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
)

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
func Run(ctx context.Context, calcs []Calculator, params Params, dry bool) error {
	g, ctx := errgroup.WithContext(ctx)
	// Limit concurrency to avoid overloading the DB with too many simultaneous heavy queries
	// if we have many calculators. Most of these perform significant scans.
	limit := runtime.NumCPU()
	if limit > 4 {
		limit = 4 // Cap at 4 to be conservative with DB connections and I/O
	}
	g.SetLimit(limit)

	for _, c := range calcs {
		c := c // capture
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return nil
			}
			return c.Compute(ctx, params, dry)
		})
	}
	return g.Wait()
}
