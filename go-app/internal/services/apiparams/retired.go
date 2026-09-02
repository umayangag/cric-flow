// Package apiparams owns the list of request parameters go-app used to honour and now
// refuses.
//
// It is a leaf package with no dependencies for one reason: three places need this
// list and two of them cannot import the third. internal/server enforces it in
// middleware; the pipeline package's contract test writes it into
// contracts/ops-console.contract.json, which the frontend then asserts it never
// constructs. Before this package the contract test carried a hand-typed copy with a
// comment explaining why the copy was acceptable — and nothing checked that the copy
// matched. A list about drift that could itself drift is the wrong shape.
package apiparams

import "strings"

// Retired describes a request parameter that is refused rather than ignored.
//
// Refusing is the decision, made once in consumer plan W0-1 and applied since:
// accepting a parameter and quietly ignoring it answers a question the caller did not
// ask and says nothing about it. Every entry here was, at some point, a parameter that
// changed which model answered a request.
type Retired struct {
	// Name is the parameter, as a query key or a JSON body field.
	Name string
	// Value, when non-empty, narrows the rejection to that one value. Empty means the
	// parameter is refused whatever it is set to.
	Value string
	// Code is the machine-readable reason, for clients that map codes to remedies.
	Code string
	// Message says what was removed.
	Message string
	// Hint says what to do instead.
	Hint string
	// Body is true for a parameter that only ever arrived in a JSON request body.
	// Those cannot be caught by query-string middleware, so the handler that decodes
	// the body refuses them; the flag records which mechanism is responsible.
	Body bool
}

// Matches reports whether the given query value set carries this parameter.
func (p Retired) Matches(get func(string) (string, bool)) bool {
	raw, present := get(p.Name)
	if !present {
		return false
	}
	if p.Value == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(raw), p.Value)
}

// retired is the whole list. Adding an entry here retires a parameter across every
// route, and puts it in the generated contract in the same commit.
var retired = []Retired{
	{
		Name:    "use_unified_model",
		Code:    "UNIFIED_MODEL_REMOVED",
		Message: "use_unified_model has been removed; models are per-format",
		Hint:    "Drop the parameter and pass the format you want. There is no cross-format serving model.",
	},
	{
		Name:    "model",
		Value:   "unified",
		Code:    "UNIFIED_MODEL_REMOVED",
		Message: "model=unified has been removed; models are per-format",
		Hint:    "Drop the parameter and pass the format you want. There is no cross-format serving model.",
	},
	{
		Name:    "use_latest_model",
		Code:    "LATEST_MODEL_REMOVED",
		Message: "use_latest_model has been removed; prediction always uses the loaded artifacts",
		Hint: "Drop the parameter. To evaluate against a different model, retrain and reload it; " +
			"the Workbench tab reports which dataset each loaded artifact was trained on.",
	},
	{
		Name:    "weather",
		Code:    "WEATHER_NOT_IMPLEMENTED",
		Message: "weather has been removed from prediction; no weather feature reaches a model",
		Hint: "Drop the field. Weather is wanted but not implemented: no venue has coordinates " +
			"and nothing fetches observations, so no weather feature reaches a model.",
		Body: true,
	},
	{
		Name:    "simulate",
		Code:    "MONTE_CARLO_REMOVED",
		Message: "simulate has been removed; the match simulator runs on every limited-overs prediction",
		Hint: "Drop the field. Totals, per-player ranges and the median-band scorecard come from " +
			"ml-service /simulate and are in the response's scorecard block whenever the format has " +
			"an innings length.",
		Body: true,
	},
	{
		Name:    "use_reconciled_scorecard",
		Code:    "RECONCILIATION_REMOVED",
		Message: "use_reconciled_scorecard has been removed; there is one scorecard and nothing is rescaled",
		Hint: "Drop the field. The scorecard and the win probability come from one simulator, so there " +
			"is no second estimate to reconcile toward.",
		Body: true,
	},
	{
		Name:    "include_both_scorecards",
		Code:    "RECONCILIATION_REMOVED",
		Message: "include_both_scorecards has been removed; there is only one scorecard",
		Hint:    "Drop the field. See use_reconciled_scorecard.",
		Body:    true,
	},
}

// Query returns the parameters refused by query-string middleware.
func Query() []Retired {
	out := make([]Retired, 0, len(retired))
	for _, p := range retired {
		if !p.Body {
			out = append(out, p)
		}
	}
	return out
}

// ByName returns the retired parameter with the given name.
func ByName(name string) (Retired, bool) {
	for _, p := range retired {
		if p.Name == name {
			return p, true
		}
	}
	return Retired{}, false
}

// QueryNames returns the retired query parameters as the frontend contract spells
// them: the bare name, or "name=value" for an entry that only refuses one value.
//
// The frontend test greps every production source for these strings, so the spelling
// has to be the one a caller would actually write. That breadth is safe here because a
// query parameter's name only ever appears where a request is being built.
func QueryNames() []string { return names(Query()) }

// BodyNames returns the retired JSON body fields.
//
// These are checked only against the frontend's one API module, not every source: a
// body field's name is an ordinary word — "weather" is also a section of /ops/status —
// and a repo-wide grep would refuse the UI for rendering a response. Narrowing to the
// module that builds requests is enough, because the request types there are closed
// object literals: TypeScript refuses an extra key at every call site.
func BodyNames() []string {
	out := make([]string, 0, len(retired))
	for _, p := range retired {
		if p.Body {
			out = append(out, p.Name)
		}
	}
	return out
}

func names(params []Retired) []string {
	out := make([]string, 0, len(params))
	for _, p := range params {
		if p.Value == "" {
			out = append(out, p.Name)
			continue
		}
		out = append(out, p.Name+"="+p.Value)
	}
	return out
}
