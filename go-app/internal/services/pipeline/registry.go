package pipeline

// Lane names the resource a step contends for. Two steps in the same lane never run
// at once; steps in different lanes may overlap.
//
// The lanes are a deliberate decision, not an accident of how the code grew (ops plan
// F-2): acquisition must not block training. Downloading a Cricsheet archive touches
// only the staging directory and can take ten minutes over a slow link; stalling a
// training run behind it would be a worse system, not a safer one.
type Lane string

const (
	// LaneCompute is the database-and-artifacts lane: import, precompute, export and
	// every training step. They read and write the same tables and model files, so
	// exactly one may run at a time.
	LaneCompute Lane = "compute"

	// LaneData is the dataset-acquisition lane: fetching and extracting archives into
	// staging. It runs concurrently with LaneCompute by design.
	LaneData Lane = "data"
)

// knownLanes is every lane a step may declare. Named so the stop endpoint can
// validate ?lane= against the same list the registry uses rather than a copy.
var knownLanes = []Lane{LaneCompute, LaneData}

// IsKnownLane reports whether the lane is one the registry uses.
func IsKnownLane(lane Lane) bool {
	for _, l := range knownLanes {
		if l == lane {
			return true
		}
	}
	return false
}

// LaneNames returns the lane names, for error messages and the API surface.
func LaneNames() []string {
	out := make([]string, 0, len(knownLanes))
	for _, l := range knownLanes {
		out = append(out, string(l))
	}
	return out
}

// Surface names where a step is offered in the UI. It exists because the registry is
// the single list of steps (ops plan F-1) but not every step belongs on the pipeline
// graph: acquisition is a precondition for the pipeline, not a stage of it, and it
// has no place in an ordering that runs import through auto-tune.
//
// Keeping acquisition in the same registry is what stops the lane, the label and the
// busy-check from drifting into a second table — the failure F-1 and F-2 both fixed.
type Surface string

const (
	// SurfacePipeline is the ordered import-to-train graph. It is the zero value, so
	// a step only names a surface when it is not an ordinary pipeline stage.
	SurfacePipeline Surface = "pipeline"

	// SurfaceData is the dataset acquisition surface: fetch and extract. These steps
	// are triggered from /ops/data/*, not from /ops/pipeline/run/{step}.
	SurfaceData Surface = "data"
)

// Step is the single authoritative definition of one pipeline step.
//
// Before this type existed the same eleven steps were spelled out in six places —
// two switch statements in the run handler, three maps in this package and two more
// in opsstatus — and a step added to one of them stayed invisible to the others.
// That is how train_combination_meta reached main with a working handler that the UI
// never offered. Everything that needs to know about steps now derives from Registry,
// so a step can only be added in one place.
type Step struct {
	// ID is the stable identifier used by POST /ops/pipeline/run/{id}, by
	// /ops/status and by the frontend. It never changes once released.
	ID string

	// Command is the value written to data_migrations.command for a run of this step.
	Command string

	// Label is the human-readable name shown in the UI and in error messages.
	Label string

	// Model is the ML model name used for the tuned-parameter lookup, or "" when
	// the step trains no single model.
	Model string

	// MLEndpoint is the ml-service path segment for POST /admin/train/{endpoint},
	// or "" when go-app executes the step itself.
	MLEndpoint string

	// Requires lists the step IDs that must have completed successfully before
	// this step may start. Empty means the step has no predecessor.
	Requires []string

	// Optional marks a step that is outside the default happy path — it is offered
	// but never implied by the steps before it.
	Optional bool

	// Prerequisite describes a precondition that step ordering cannot express, in
	// terms the operator can act on. Empty when ordering says everything.
	Prerequisite string

	// Lane is the resource this step contends for. The zero value means LaneCompute,
	// so a step only names a lane when it is not the ordinary pipeline one.
	Lane Lane

	// Surface is where the step is offered. The zero value means SurfacePipeline.
	Surface Surface
}

// EffectiveLane returns the step's lane, defaulting to LaneCompute.
func (s Step) EffectiveLane() Lane {
	if s.Lane == "" {
		return LaneCompute
	}
	return s.Lane
}

// EffectiveSurface returns the step's surface, defaulting to SurfacePipeline.
func (s Step) EffectiveSurface() Surface {
	if s.Surface == "" {
		return SurfacePipeline
	}
	return s.Surface
}

// IsTraining reports whether the step trains a model whose hyper-parameters are
// looked up from the tuned-params table.
func (s Step) IsTraining() bool { return s.Model != "" }

// RunsOnMLService reports whether the step is executed by ml-service rather than go-app.
func (s Step) RunsOnMLService() bool { return s.MLEndpoint != "" }

// Registry is an ordered, read-only collection of pipeline steps with lookups by
// ID and by data_migrations command. Construct it with NewRegistry; the package
// level Steps() returns the one the application uses.
type Registry struct {
	steps     []Step
	byID      map[string]Step
	byCommand map[string]Step
}

// NewRegistry indexes the given steps. The order of steps is preserved and is the
// order the pipeline is presented in.
func NewRegistry(steps ...Step) *Registry {
	r := &Registry{
		steps:     append([]Step(nil), steps...),
		byID:      make(map[string]Step, len(steps)),
		byCommand: make(map[string]Step, len(steps)),
	}
	for _, s := range r.steps {
		r.byID[s.ID] = s
		r.byCommand[s.Command] = s
	}
	return r
}

// All returns the steps in pipeline order.
func (r *Registry) All() []Step { return append([]Step(nil), r.steps...) }

// ByID returns the step with the given ID.
func (r *Registry) ByID(id string) (Step, bool) {
	s, ok := r.byID[id]
	return s, ok
}

// ByCommand returns the step whose runs are recorded under the given
// data_migrations command.
func (r *Registry) ByCommand(command string) (Step, bool) {
	s, ok := r.byCommand[command]
	return s, ok
}

// Has reports whether the registry knows the step ID. This is the set the run
// handler accepts, and the set the frontend must offer.
func (r *Registry) Has(id string) bool {
	_, ok := r.byID[id]
	return ok
}

// CommandsInLane returns the data_migrations commands of every step in the lane.
// This is the set a step must find idle before it may start.
func (r *Registry) CommandsInLane(lane Lane) []string {
	out := make([]string, 0, len(r.steps))
	for _, s := range r.steps {
		if s.EffectiveLane() == lane {
			out = append(out, s.Command)
		}
	}
	return out
}

// OnSurface returns the steps offered on the given surface, in registry order.
// The pipeline graph renders OnSurface(SurfacePipeline); the Data tab renders
// OnSurface(SurfaceData).
func (r *Registry) OnSurface(surface Surface) []Step {
	out := make([]Step, 0, len(r.steps))
	for _, s := range r.steps {
		if s.EffectiveSurface() == surface {
			out = append(out, s)
		}
	}
	return out
}

// LaneForCommand returns the lane a data_migrations command belongs to. Commands the
// registry does not know are treated as LaneCompute — the conservative answer, since
// an unrecognised job is more likely to touch the database than not.
func (r *Registry) LaneForCommand(command string) Lane {
	if s, ok := r.byCommand[command]; ok {
		return s.EffectiveLane()
	}
	return LaneCompute
}

// LabelForCommand returns the human-readable label for a data_migrations command,
// falling back to the command itself for rows written by something outside the registry.
func (r *Registry) LabelForCommand(command string) string {
	if s, ok := r.byCommand[command]; ok {
		return s.Label
	}
	return command
}

// defaultRegistry is the pipeline as it actually runs. Adding a step here — and
// only here — makes it acceptable to the run handler, ordered by /ops/status,
// labelled in run history, and (via the generated contract) required of the UI.
var defaultRegistry = NewRegistry(
	Step{
		ID:      "import",
		Command: "cricsheet-import",
		Label:   "Import",
	},
	Step{
		ID:       "precompute",
		Command:  "precompute-features",
		Label:    "Precompute",
		Requires: []string{"import"},
	},
	Step{
		ID:       "export",
		Command:  "export-dataset",
		Label:    "Export",
		Requires: []string{"precompute"},
	},
	Step{
		ID:         "train_win",
		Command:    "train-win",
		Label:      "Train Win",
		Model:      "win",
		MLEndpoint: "win",
		Requires:   []string{"export"},
	},
	// Auto-tune requires the export and nothing after it. It consumes the same inputs
	// the train step does — the training-data API — and not one trained artifact.
	//
	// It used to require the train steps, which forced the one order the pipeline is
	// meant to avoid: train on stale params, then search for better ones and throw the
	// artifacts away. Hyperparameters are a function of the feature space, so when the
	// feature space has changed the search has to come first and the training that
	// follows it uses what it found. That is the `tune` run plan, and this edge is what
	// makes it runnable.
	Step{
		ID:         "auto_tune",
		Command:    "ml-auto-tune",
		Label:      "Auto-tune",
		MLEndpoint: "auto-tune",
		Requires:   []string{"export"},
		Optional:   true,
	},
	// Acquisition. Declared here so the lane, the label and the busy-check come from
	// the same place as every other step, but on SurfaceData: fetching a dataset is
	// what you do before the pipeline, not a stage within it, so it carries no
	// Requires and never appears on the graph.
	Step{
		ID:      "fetch",
		Command: "dataset-fetch",
		Label:   "Fetch Dataset",
		Lane:    LaneData,
		Surface: SurfaceData,
		Prerequisite: "Downloads from the Cricsheet host allowlist into the staging " +
			"directory. The server needs outbound network access.",
	},
	// Extract deliberately declares no Requires. Ordering here would say the archive
	// must have come from a fetch, and it need not have: an operator can drop one in
	// staging by hand. The handler checks the archive exists, which is the real
	// precondition, and says so in terms that name the file.
	Step{
		ID:      "extract",
		Command: "dataset-extract",
		Label:   "Extract Dataset",
		Lane:    LaneData,
		Surface: SurfaceData,
		Prerequisite: "Needs a .zip in the staging directory — fetch one first, or " +
			"place one there. Extraction replaces the dataset directory; the previous " +
			"contents are kept under staging.",
	},
)

// Steps returns the application's pipeline step registry.
func Steps() *Registry { return defaultRegistry }
