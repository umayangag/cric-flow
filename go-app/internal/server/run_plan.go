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
	mu      sync.Mutex
	running *planRun
}

// planRun is one plan's registration. Behind a pointer so it has an identity a release
// can compare against: function values are not comparable in Go.
type planRun struct{ cancel context.CancelFunc }

// set registers the plan starting now and returns the release func that deregisters
// it. Call release in a defer when the plan goroutine exits.
//
// Release clears this registration and no other. `clear()` used to null whatever was
// stored, so a second plan that started, was refused as "already running" and exited
// deregistered the plan that was actually running — leaving it with no way to be
// stopped (GO-06). A plan already registered therefore keeps its place, and the
// refused plan's release is a no-op.
func (p *planCancel) set(cancel context.CancelFunc) func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running != nil {
		slog.Warn("run plan: a plan is already registered as stoppable; keeping it")
		return func() {}
	}
	registration := &planRun{cancel: cancel}
	p.running = registration
	return func() { p.release(registration) }
}

func (p *planCancel) release(registration *planRun) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running == registration {
		p.running = nil
	}
}

// stop cancels the running plan and reports whether there was one.
func (p *planCancel) stop() bool {
	p.mu.Lock()
	registration := p.running
	p.running = nil
	p.mu.Unlock()
	if registration == nil {
		return false
	}
	registration.cancel()
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
	releaseLane := a.SetJobCancel(lane, cancel)
	defer releaseLane()

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
	releasePlan := a.planCancel.set(cancel)

	a.RunBackgroundJob(func() {
		defer cancel()
		defer releasePlan()

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
	})
}

// StartImportPlan begins the acquire-and-import plan in the background.
//
// Separate from StartRunPlan because its steps are skipped by what is on disk rather
// than by what a previous run finished, and because it carries a StepRequest: the
// import step's own options (placeholder fielding rows) have to survive being run by
// a plan rather than by its handler.
func (a *App) StartImportPlan(steps []pipelinesvc.Step, skip map[string]string, req StepRequest) {
	planCtx, cancel := context.WithCancel(a.JobContext())
	releasePlan := a.planCancel.set(cancel)

	a.RunBackgroundJob(func() {
		defer cancel()
		defer releasePlan()

		executor := a.newPlanExecutor()
		executor.Run = func(ctx context.Context, step pipelinesvc.Step) error {
			return a.runPlanStepWith(ctx, step, req)
		}
		if err := executor.ExecuteSkipping(planCtx, runplan.PlanImport, steps, skip); err != nil {
			slog.Error("import plan: stopped", slog.Any("err", err))
			return
		}
		slog.Info("import plan: completed")
	})
}

// StopRunPlan cancels the plan in flight and reports whether there was one.
func (a *App) StopRunPlan() bool { return a.planCancel.stop() }
