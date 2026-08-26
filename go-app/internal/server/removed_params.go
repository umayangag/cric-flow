package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// removedParam describes a request parameter the API used to honour and now refuses.
//
// The alternative — accepting it and quietly ignoring it — is the failure mode this
// codebase has already been bitten by: a caller asks for one thing, gets another, and
// nothing says so. A removed parameter that changed which model answered the request
// is exactly that case, so it is refused with an explanation instead.
type removedParam struct {
	// Name is the query parameter.
	Name string
	// Value, when non-empty, narrows the rejection to that one value. Empty means
	// the parameter is refused whatever it is set to.
	Value string
	// Code is the machine-readable reason, for clients that map codes to remedies.
	Code string
	// Message says what was removed.
	Message string
	// Hint says what to do instead.
	Hint string
}

// matches reports whether the request carries this removed parameter.
func (p removedParam) matches(q url.Values) bool {
	if !q.Has(p.Name) {
		return false
	}
	if p.Value == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(q.Get(p.Name)), p.Value)
}

// removedQueryParams is the whole list. Adding an entry here retires a parameter
// across every route in one edit.
var removedQueryParams = []removedParam{
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
}

// rejectRemovedParams refuses requests carrying a retired parameter with 400 and a
// hint, rather than serving a result the caller did not ask for.
//
// Note this deliberately does not touch auto_tune's own `unified` flag: that is a
// training mode (train one model across formats), not a serving tier, and it is a
// different parameter with a different name. Two same-named concepts caught the last
// refactor out; keeping the rule keyed on the exact parameter keeps them apart.
func rejectRemovedParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, p := range removedQueryParams {
			if !p.matches(q) {
				continue
			}
			slog.Info("rejected removed request parameter",
				slog.String("param", p.Name), slog.String("path", r.URL.Path))
			writeJSON(w, http.StatusBadRequest, apiError{Code: p.Code, Message: p.Message, Hint: p.Hint})
			return
		}
		next.ServeHTTP(w, r)
	})
}
