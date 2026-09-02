package pipeline

import (
	"regexp"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/teams"
)

// This file declares every literal go-app puts on the wire to ml-service: the paths it
// posts to, the query parameters it names, and the format of the values it sends.
//
// It exists because of D-9 (H-24). The training cutoff was formatted here as RFC3339
// and parsed there as a date, each side tested against its own assumption, and the
// console's Retrain button failed on its first line of work. A literal that crosses a
// service boundary is declared once, generated into
// contracts/ops-console.contract.json, and asserted by tests on *both* sides — never
// re-typed on the far side of the boundary and hoped about.

// The cutoff's wire format. A retrain cutoff is a date: rows before it train, rows at
// or after it are the holdout, and the time of day names no different set of rows.
const (
	// CutoffPattern is the regular expression a cutoff value must match. Both
	// go-app's default and the console's date field are checked against it.
	CutoffPattern = `^\d{4}-\d{2}-\d{2}$`

	// CutoffHint is how the format is spelled to an operator; the console's cutoff
	// field carries it as its label.
	CutoffHint = "YYYY-MM-DD"

	// CutoffExample is a value of the format, for the far side's tests to parse and
	// for error messages to quote.
	CutoffExample = "2025-09-01"
)

// cutoffFormat is the compiled CutoffPattern, so the tests on this side check the same
// expression the contract publishes rather than a second copy of it.
var cutoffFormat = regexp.MustCompile(CutoffPattern)

// IsValidCutoff reports whether a cutoff value is in the wire format ml-service is
// promised. It is the same check the contract states.
func IsValidCutoff(value string) bool { return cutoffFormat.MatchString(value) }

// DefaultCutoff returns today's UTC date, used when a training step is started without
// one — which is what the ops console sends when its cutoff box is left empty.
func DefaultCutoff() string {
	return time.Now().UTC().Format(time.DateOnly)
}

// The ml-service admin surface go-app calls. Every path and query parameter name here
// is matched by a route on the other side, and the contract test asserts exactly that.
const (
	// MLTrainPathPrefix is joined with a step's MLEndpoint to address a training step.
	MLTrainPathPrefix = "/admin/train/"

	// MLProgressPath reports what a running step has published so far.
	MLProgressPath = MLTrainPathPrefix + "progress"

	// MLStopPath stops the training subprocess ml-service is running.
	//
	// Stopping is a request, not an inference. Cancelling our own outgoing HTTP call
	// closes a socket; it does not reach the process on the other side, which is how a
	// Stop could report success while `ml.xi.retrain` kept running (D-10).
	MLStopPath = MLTrainPathPrefix + "stop"

	// MLReloadPath points `current` at a run and loads it.
	MLReloadPath = "/admin/reload"
)

// Query parameter names on the same boundary.
const (
	// MLQueryCutoff carries the training cutoff to retrain and evaluate.
	MLQueryCutoff = "cutoff"

	// MLQueryStep names the pipeline step whose progress is being read.
	MLQueryStep = "step"

	// MLQueryRun names the run a reload should load.
	MLQueryRun = "run"
)

// MLCall is one endpoint on the go-app -> ml-service boundary: what go-app sends and
// what ml-service must therefore accept.
type MLCall struct {
	Method string
	Path   string
	Query  []string
}

// TeamGenders is the gender half of a team's identity, as both services spell it.
//
// It is here for D-11's reason, which is D-9's reason one table along: go-app writes these
// values into `opposition.gender` and `match.gender` from Cricsheet's `info.gender`, the
// frontend now names a side to go-app with one of them, and ml-service *matches on the
// literal* -- `RatingState._ctx_group` reads `gender == "female"` to pick E7's context
// baseline group. Three components, one vocabulary, and until now three private copies of
// it. Declared once here, generated into the contract, asserted from all three sides.
func TeamGenders() []string { return teams.Genders() }

// MLCalls returns the ml-service admin endpoints go-app calls, derived from the step
// registry so a training step added there is a call ml-service is tested for.
func MLCalls() []MLCall {
	calls := make([]MLCall, 0, len(Steps().All())+2)
	for _, step := range Steps().All() {
		if !step.RunsOnMLService() {
			continue
		}
		calls = append(calls, MLCall{
			Method: "POST",
			Path:   MLTrainPathPrefix + step.MLEndpoint,
			Query:  []string{MLQueryCutoff},
		})
	}
	return append(calls,
		MLCall{Method: "GET", Path: MLProgressPath, Query: []string{MLQueryStep}},
		MLCall{Method: "POST", Path: MLStopPath, Query: []string{MLQueryStep}},
		MLCall{Method: "POST", Path: MLReloadPath, Query: []string{MLQueryRun}},
	)
}

// StopResponseField is the field ml-service's stop answers with: the steps whose process
// it watched exit.
//
// It is declared here, and asserted from both sides, because go-app now *matches on it*.
// H-24's own note said go-app parsed nothing out of ml-service's bodies and so had no
// literal that could drift — that stopped being true the moment a Stop depended on
// reading this one (D-10).
const StopResponseField = "stopped"
