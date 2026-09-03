// Package runplan defines the named sequences of pipeline steps and the state of a
// run through one.
//
// `make up-all` and `make full-pipeline` have existed in the Makefile for as long as
// the pipeline has; from the ops console the same thing is eleven manual clicks with
// waiting in between (ops plan, gap 3). This package is the server-side half of
// closing that asymmetry.
//
// It is deliberately free of execution concerns — no database, no HTTP, no goroutines
// — so the question "what does `full` mean, and where has this run got to?" can be
// answered and tested without any of them.
package runplan

import (
	"fmt"
	"sort"
	"strings"

	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// Named plans. The step lists are derived from the registry rather than written out,
// so a step added there joins the plans that should contain it instead of being
// quietly left out — the drift F-1 removed from six other places.
const (
	// PlanFull is the whole pipeline: acquire nothing, but import through training.
	PlanFull = "full"
	// PlanRetrainOnly re-trains against data already imported, and serves the result.
	//
	// The counterpart to PlanFull for the common case: new matches are already in the
	// database and what is stale is the model, not the data. There is no separate
	// "tune" plan any more -- the grid runs inside retrain and records its choice in
	// the run manifest, so searching and training are one step and cannot be run in
	// the order that throws the artifacts away.
	PlanRetrainOnly = "retrain-only"
	// PlanImport acquires a dataset and loads it: fetch, extract, import.
	//
	// This is what the Import action runs (consumer plan W6-2). It is a plan rather
	// than three things an operator does in two tabs, and it is the run-plan executor
	// rather than a second chaining mechanism, so it gets per-step live state, Stop
	// and run history without any of them being written twice.
	PlanImport = "import"
	// PlanRefresh is the scheduled cadence (A-5): fetch, extract, import, retrain,
	// reload — new matches all the way through to the run being served.
	//
	// It exists because the two halves being separate plans is what lets them come
	// apart. On the box this was written on the `import` plan had run and no retrain
	// followed it: the database held matches to 2026-09-01 while the served run's
	// ratings ran through 2026-08-25, nine days old against H-11's fourteen-day
	// limit. Nothing was broken; the second half was simply never triggered. One plan
	// removes the gap, because reload is reached only by walking through retrain.
	//
	// Reload publishing only on success is the executor's ordinary behaviour, not a
	// property of this plan: a failed step stops the walk, so a retrain that fails —
	// including on the data-quality gate, which the automated path never accepts on
	// an operator's behalf — leaves `current` pointing where it already pointed.
	PlanRefresh = "refresh"
)

// StepStatus is where one step of a plan has got to.
type StepStatus string

const (
	// StatusPending means the step has not started.
	StatusPending StepStatus = "PENDING"
	// StatusRunning means the step is in flight.
	StatusRunning StepStatus = "RUNNING"
	// StatusCompleted means the step finished successfully.
	StatusCompleted StepStatus = "COMPLETED"
	// StatusFailed means the step failed; the plan stops here.
	StatusFailed StepStatus = "FAILED"
	// StatusCancelled means the operator stopped the plan.
	StatusCancelled StepStatus = "CANCELLED"
	// StatusSkipped means the step was already complete when the plan reached it.
	StatusSkipped StepStatus = "SKIPPED"
)

// Terminal reports whether a step will not change again within this plan run.
func (s StepStatus) Terminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusSkipped:
		return true
	default:
		return false
	}
}

// StepState is one step's place in a plan.
type StepState struct {
	StepID string     `json:"step_id"`
	Label  string     `json:"label"`
	Status StepStatus `json:"status"`
	// StartedAt and FinishedAt are RFC3339, empty until they happen.
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	// Error is the failure message, actionable where ml-service supplied one.
	Error string `json:"error,omitempty"`
	// Note says why a step was skipped.
	//
	// A skipped step that renders as "done" is the silent-success failure this
	// codebase has met three times, so a skip has to say what it decided and on what
	// evidence: "the dataset directory already holds all_json.zip from this source
	// (21,253 match files)" is a claim an operator can check, and "SKIPPED" alone is
	// not.
	Note string `json:"note,omitempty"`
	// MigrationID links to the step's own row in run history.
	MigrationID int `json:"migration_id,omitempty"`
}

// State is a plan run: which plan, which steps, and where it has got to.
//
// It is JSON because it lives in `data_migrations.metadata` — the run-history table
// already holds a row per run, already has a jsonb column, and a plan is a run. A
// separate table would have been a second place to look for the same thing.
type State struct {
	Plan  string      `json:"plan"`
	Steps []StepState `json:"steps"`
	// StartedAt is when the plan began, RFC3339.
	StartedAt string `json:"started_at,omitempty"`
	// FinishedAt is when it stopped, for any reason.
	FinishedAt string `json:"finished_at,omitempty"`
}

// planSteps returns the ordered step IDs for a named plan.
//
// Derived from the registry each time rather than stored: the pipeline's shape is the
// registry's business, and a hardcoded list here would be the seventh copy.
func planSteps(name string) ([]string, error) {
	registry := pipelinesvc.Steps()
	graph := registry.OnSurface(pipelinesvc.SurfacePipeline)

	include := func(keep func(pipelinesvc.Step) bool) []string {
		out := make([]string, 0, len(graph))
		for _, step := range graph {
			if keep(step) {
				out = append(out, step.ID)
			}
		}
		return out
	}

	switch strings.TrimSpace(strings.ToLower(name)) {
	case PlanImport:
		// Named explicitly rather than filtered out of the graph: fetch and extract
		// are on SurfaceData and carry no Requires, so no predicate over the pipeline
		// surface can produce this order. Stating it is also the only place the
		// dependency "import needs data on disk" is written down.
		return []string{"fetch", "extract", "import"}, nil
	case PlanFull:
		// Optional steps are excluded by definition: they are offered but never
		// implied by the steps before them, and a "run everything" that silently
		// included evaluate would spend the whole harness on a run nobody asked to
		// measure.
		return include(func(s pipelinesvc.Step) bool { return !s.Optional }), nil
	case PlanRetrainOnly:
		return include(func(s pipelinesvc.Step) bool { return !s.Optional && s.ID != "import" }), nil
	case PlanRefresh:
		// Composed from the two plans it is the concatenation of rather than written
		// out. A fourth literal list of steps here would be the one that keeps its
		// old shape after the registry changes.
		acquire, err := planSteps(PlanImport)
		if err != nil {
			return nil, err
		}
		train, err := planSteps(PlanRetrainOnly)
		if err != nil {
			return nil, err
		}
		return append(acquire, train...), nil
	default:
		return nil, fmt.Errorf("unknown plan %q; known plans: %s", name, strings.Join(Names(), ", "))
	}
}

// Names returns the known plan names, sorted, for error messages and the API.
func Names() []string {
	names := []string{PlanFull, PlanRetrainOnly, PlanImport, PlanRefresh}
	sort.Strings(names)
	return names
}

// Describe returns the ordered steps of a named plan.
//
// A named plan's own sequence is honoured as written, unlike a caller-supplied step
// list: the plan *is* the ordering decision. That matters for PlanImport, whose steps
// run fetch → extract → import — an order the registry cannot express, because
// acquisition sits on a different surface and carries no Requires, and which
// re-sorting into registry order would turn into "import, then download the data it
// just imported".
func Describe(name string) ([]pipelinesvc.Step, error) {
	ids, err := planSteps(name)
	if err != nil {
		return nil, err
	}
	registry := pipelinesvc.Steps()
	steps := make([]pipelinesvc.Step, 0, len(ids))
	for _, id := range ids {
		step, known := registry.ByID(id)
		if !known {
			return nil, fmt.Errorf("plan %q names unknown step %q", name, id)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// Resolve turns a named plan or an explicit step list into ordered steps.
//
// An explicit list is reordered into registry order rather than run as given: the
// registry's order is the dependency order, and honouring a caller's arbitrary
// sequence would mean running reload before retrain because someone typed it that
// way — which `CanRunPipelineStep` would then refuse, one step in, having already run
// the others.
func Resolve(name string, stepIDs []string) ([]pipelinesvc.Step, error) {
	if len(stepIDs) > 0 {
		if strings.TrimSpace(name) != "" {
			return nil, fmt.Errorf("give a plan name or an explicit step list, not both")
		}
		return stepsByID(stepIDs)
	}
	return Describe(name)
}

// stepsByID validates a *caller-supplied* step list and returns it in registry order.
//
// The surface check belongs here rather than in Describe: it guards against a request
// asking to run acquisition as if it were a pipeline stage, which is a mistake worth
// naming. A named plan that includes acquisition is not that mistake — it is a plan
// that has decided where acquisition goes.
func stepsByID(ids []string) ([]pipelinesvc.Step, error) {
	registry := pipelinesvc.Steps()
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		step, known := registry.ByID(trimmed)
		if !known {
			return nil, fmt.Errorf("unknown step %q", trimmed)
		}
		if step.EffectiveSurface() != pipelinesvc.SurfacePipeline {
			return nil, fmt.Errorf("%s is a dataset step, not a pipeline stage", step.Label)
		}
		wanted[step.ID] = true
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("a plan needs at least one step")
	}

	out := make([]pipelinesvc.Step, 0, len(wanted))
	for _, step := range registry.OnSurface(pipelinesvc.SurfacePipeline) {
		if wanted[step.ID] {
			out = append(out, step)
		}
	}
	return out, nil
}

// NewState builds the initial state of a plan run.
func NewState(plan string, steps []pipelinesvc.Step, startedAt string) State {
	states := make([]StepState, 0, len(steps))
	for _, step := range steps {
		states = append(states, StepState{StepID: step.ID, Label: step.Label, Status: StatusPending})
	}
	return State{Plan: plan, Steps: states, StartedAt: startedAt}
}

// Clone returns a deep copy.
//
// The executor mutates its state as it walks the plan, so anything handed to a Store
// must be a copy: an implementation that retained what it was given — an in-memory
// store, a cache, a test double — would watch its records change underneath it, and
// the snapshot it thought it had of "step 2 running" would silently become the final
// state. Marshalling immediately, as the production store does, hides that; it should
// not be a requirement.
func (s State) Clone() State {
	out := s
	out.Steps = append([]StepState(nil), s.Steps...)
	return out
}

// Find returns a pointer to a step's state, or nil.
func (s *State) Find(stepID string) *StepState {
	for i := range s.Steps {
		if s.Steps[i].StepID == stepID {
			return &s.Steps[i]
		}
	}
	return nil
}

// FirstIncomplete returns the first step that has not completed or been skipped.
//
// This is what makes a plan resumable from where it stopped rather than from the top:
// re-running an eleven-step pipeline because step nine failed is how an operator
// learns not to use the button.
func (s *State) FirstIncomplete() (StepState, bool) {
	for _, step := range s.Steps {
		if step.Status != StatusCompleted && step.Status != StatusSkipped {
			return step, true
		}
	}
	return StepState{}, false
}

// Done reports whether every step has reached a terminal status.
func (s *State) Done() bool {
	for _, step := range s.Steps {
		if !step.Status.Terminal() {
			return false
		}
	}
	return true
}

// Failed returns the first failed step, if the plan stopped on one.
func (s *State) Failed() (StepState, bool) {
	for _, step := range s.Steps {
		if step.Status == StatusFailed {
			return step, true
		}
	}
	return StepState{}, false
}
