package server

import (
	"net/http"

	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
)

// OpsStatusResponse is kept as a type alias for backward compatibility.
type OpsStatusResponse = opsstatus.Response

// opsStatusHandler assembles and returns the ops status payload.
func (a *App) opsStatusHandler(w http.ResponseWriter, r *http.Request) {
	resp := opsstatus.AssembleResponse(r.Context(), a.dbProbe)
	respondJSON(w, http.StatusOK, resp)
}
