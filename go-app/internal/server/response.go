// revive:disable:var-naming
package server

import (
	"net/http"
)

// respondJSON is kept for backward compatibility. Prefer using writeJSON directly.
func respondJSON(w http.ResponseWriter, code int, v any) {
	writeJSON(w, code, v)
}
