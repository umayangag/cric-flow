package server

import (
	"log/slog"
	"net/http"

	"github.com/umayangag/cric-flow/go-app/internal/services/apiparams"
)

// rejectRemovedParams refuses requests carrying a retired query parameter with 400 and
// a hint, rather than serving a result the caller did not ask for.
//
// The list itself lives in apiparams, which the generated frontend contract is built
// from too — so a parameter cannot be retired on the server while the UI goes on
// sending it. Body-only parameters are not visible here; the handler that decodes the
// body refuses those (see rejectRetiredBodyField).
//
// The rule matches the exact parameter name and value, never a substring: "unified"
// named both a serving tier and, separately, an auto-tune training mode, and a rule
// that matched the word would have refused requests that had nothing to do with the
// retired parameter.
func rejectRemovedParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		get := func(name string) (string, bool) {
			if !q.Has(name) {
				return "", false
			}
			return q.Get(name), true
		}
		for _, p := range apiparams.Query() {
			if !p.Matches(get) {
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

// retiredBodyFieldError is returned when a decoded request body carries a retired
// field. encoding/json ignores unknown fields, so a caller still sending one would
// otherwise get a prediction computed without it and no indication why — the
// silent-success failure the retired list exists to prevent.
type retiredBodyFieldError struct{ param apiparams.Retired }

func (e retiredBodyFieldError) Error() string { return e.param.Message }

// rejectRetiredBodyField reports an error when the raw JSON body carries the named
// retired field, so the handler can refuse with the code and hint the list carries.
func rejectRetiredBodyField(present bool, name string) error {
	if !present {
		return nil
	}
	p, ok := apiparams.ByName(name)
	if !ok {
		return nil
	}
	return retiredBodyFieldError{param: p}
}
