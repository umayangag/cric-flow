package server

import (
	"context"
	"log/slog"
	"sync"

	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/services/runplan"
)

// planCancel holds the cancel func for the run plan in flight.
//
// Separate from the per-lane job cancels: cancelling a plan must stop the plan, not
// just the step it happens to be on (ops plan R-1). Cancelling only the step would
// stop that step and then start the next one, which is not what "stop" means.
type planCancel struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (p *planCancel) set(cancel context.CancelFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancel = cancel
}

func (p *planCancel) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancel = nil
}

// stop cancels the running plan and reports whether there was one.
func (p *planCancel) stop() bool {
	p.mu.Lock()
	cancel := p.cancel
	p.cancel = nil
	p.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// newPlanExecutor builds the executor with the application's real dependencies.
//
// The gate is opsstatus.CanRunPipelineStep — the same rules the single-step endpoint
// applies. A plan with its own ordering logic would be a second answer to "may this
// step run?", and the two would disagree the first time either changed.
func (a *App) newPlanExecutor() *runplan.Executor {
	return &runplan.Executor{
		Store: runplan.TrackingStore{},
		Run:   a.runPlanStep,
		Gate:  opsstatus.CanRunPipelineStep,
	}
}

// runPlanStep runs one step to completion, recording it in run history exactly as a
// single-step run would.
//
// It is synchronous, unlike the HTTP handlers: the executor's whole job is to wait for
// one step before starting the next. The work itself comes from stepJob, so a plan
// runs the same code path a single-step trigger does rather than a parallel one.
func (a *App) runPlanStep(ctx context.Context, step pipelinesvc.Step) error {
	return a.runPlanStepWith(ctx, step, StepRequest{})
}

// runPlanStepWith runs one step with the plan's own request options.
func (a *App) runPlanStepWith(ctx context.Context, step pipelinesvc.Step, req StepRequest) error {
	job := a.stepJob(step, req)

	lane := pipelinesvc.Steps().LaneForCommand(job.Command)
	stepCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Registered per-lane as well, so /ops/pipeline/stop without a lane stops the
	// step in flight and not only the plan around it.
	a.SetJobCancel(lane, cancel)
	defer a.ClearJobCancel(lane)

	slog.Info("run plan: step starting", slog.String("step", step.ID))
	return pipeline.RunJob(stepCtx, job.Command, job.Args, job.Timeout, job.Run)
}

// StartRunPlan begins a plan in the background and returns once it has started.
//
// Background because a full pipeline outlives any request, and persisted because the
// browser that started it may be closed long before it ends.
//
// `prior` resumes a stopped run, skipping the steps it completed. Nil runs every step
// — which is what a fresh plan means, and what an earlier version got wrong by
// skipping anything that had ever succeeded on this box.
func (a *App) StartRunPlan(plan string, steps []pipelinesvc.Step, prior *runplan.State) {
	planCtx, cancel := context.WithCancel(a.JobContext())
	a.planCancel.set(cancel)

	go func() {
		defer cancel()
		defer a.planCancel.clear()

		executor := a.newPlanExecutor()
		var err error
		if prior != nil {
			err = executor.Resume(planCtx, plan, steps, *prior)
		} else {
			err = executor.Execute(planCtx, plan, steps)
		}
		if err != nil {
			slog.Error("run plan: stopped", slog.String("plan", plan), slog.Any("err", err))
			return
		}
		slog.Info("run plan: completed", slog.String("plan", plan))
	}()
}

// StartImportPlan begins the acquire-and-import plan in the background.
//
// Separate from StartRunPlan because its steps are skipped by what is on disk rather
// than by what a previous run finished, and because it carries a StepRequest: the
// import step's own options (placeholder fielding rows) have to survive being run by
// a plan rather than by its handler.
func (a *App) StartImportPlan(steps []pipelinesvc.Step, skip map[string]string, req StepRequest) {
	planCtx, cancel := context.WithCancel(a.JobContext())
	a.planCancel.set(cancel)

	go func() {
		defer cancel()
		defer a.planCancel.clear()

		executor := a.newPlanExecutor()
		executor.Run = func(ctx context.Context, step pipelinesvc.Step) error {
			return a.runPlanStepWith(ctx, step, req)
		}
		if err := executor.ExecuteSkipping(planCtx, runplan.PlanImport, steps, skip); err != nil {
			slog.Error("import plan: stopped", slog.Any("err", err))
			return
		}
		slog.Info("import plan: completed")
	}()
}

// StopRunPlan cancels the plan in flight and reports whether there was one.
func (a *App) StopRunPlan() bool { return a.planCancel.stop() }
