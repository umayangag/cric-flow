package opsstatus

import (
	"context"
	"log/slog"
	"strings"

	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// stepGate answers "may this step start, and is it finished?" for one step, using
// the run history in data_migrations. It exists so BuildPipelineSection and
// CanRunPipelineStep share one set of rules instead of two that drift apart.
type stepGate struct {
	ctx        context.Context
	registry   *pipelinesvc.Registry
	inProgress map[string]bool
}

// newStepGate loads the in-flight commands once so a whole-pipeline pass does not
// re-query for every step.
func newStepGate(ctx context.Context) *stepGate {
	inProgress, err := tracking.InProgressByCommand(ctx)
	if err != nil {
		slog.Warn("pipeline: InProgressByCommand failed", "err", err)
		inProgress = map[string]bool{}
	}
	return &stepGate{ctx: ctx, registry: pipelinesvc.Steps(), inProgress: inProgress}
}

// running reports whether the step currently has a run in flight.
func (g *stepGate) running(step pipelinesvc.Step) bool { return g.inProgress[step.Command] }

// completed reports whether the step's most recent run succeeded.
//
// Most recent, not "ever": a step whose latest run failed or was cancelled has not
// produced the output the steps after it consume, whatever it managed weeks ago.
func (g *stepGate) completed(step pipelinesvc.Step) bool {
	done, err := tracking.LastRunSucceededForCommand(g.ctx, step.Command)
	if err != nil {
		slog.Warn("pipeline: LastRunSucceededForCommand failed",
			"step", step.ID, "command", step.Command, "err", err)
		return false
	}
	return done
}

// unmetRequirement returns the first prerequisite step that has not completed, if any.
func (g *stepGate) unmetRequirement(step pipelinesvc.Step) (pipelinesvc.Step, bool) {
	for _, id := range step.Requires {
		req, ok := g.registry.ByID(id)
		if !ok {
			continue
		}
		if !g.completed(req) {
			return req, true
		}
	}
	return pipelinesvc.Step{}, false
}

// runnable reports whether the step may be started right now.
func (g *stepGate) runnable(step pipelinesvc.Step) bool {
	if g.running(step) {
		return false
	}
	_, unmet := g.unmetRequirement(step)
	return !unmet
}

// BuildPipelineSection returns a map with "steps" (per-step running, runnable,
// completed) for /ops/status, in pipeline order.
//
// It reports the pipeline surface only. Acquisition steps share the registry so their
// lane and label cannot drift, but they have no place in an ordering that runs import
// through auto-tune — "order" is what the UI renders the graph from, and a fetch step
// in it would be a stage that is not one.
func BuildPipelineSection(ctx context.Context) map[string]any {
	gate := newStepGate(ctx)
	graph := gate.registry.OnSurface(pipelinesvc.SurfacePipeline)
	steps := make(map[string]any, len(graph))
	order := make([]string, 0, len(graph))
	for _, step := range graph {
		steps[step.ID] = map[string]any{
			"running":   gate.running(step),
			"runnable":  gate.runnable(step),
			"completed": gate.completed(step),
			"optional":  step.Optional,
		}
		order = append(order, step.ID)
	}
	// "order" lets the UI render the pipeline without hard-coding the sequence.
	return map[string]any{"steps": steps, "order": order}
}

// CanRunPipelineStep returns whether the step can be started and, when it cannot,
// a message an operator can act on.
func CanRunPipelineStep(ctx context.Context, stepID string) (ok bool, errMsg string) {
	registry := pipelinesvc.Steps()
	step, known := registry.ByID(stepID)
	if !known {
		return false, "unknown step"
	}

	running, err := tracking.HasInProgressForCommand(ctx, step.Command)
	if err != nil {
		return false, "could not verify if step is running"
	}
	if running {
		return false, "this step is already running"
	}

	unmet := make([]string, 0, len(step.Requires))
	for _, id := range step.Requires {
		req, found := registry.ByID(id)
		if !found {
			continue
		}
		done, err := tracking.LastRunSucceededForCommand(ctx, req.Command)
		if err != nil {
			return false, "could not verify previous step"
		}
		if !done {
			unmet = append(unmet, req.Label)
		}
	}
	switch len(unmet) {
	case 0:
		return true, ""
	case 1:
		if len(step.Requires) == 1 {
			return false, "complete the previous step (" + unmet[0] + ") first"
		}
		return false, "complete " + unmet[0] + " first"
	default:
		return false, "complete " + strings.Join(unmet, ", ") + " first"
	}
}
