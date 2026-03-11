package server

import (
	"net/http"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

type OptionsHandler struct{}

func (h *OptionsHandler) HandleGetTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teams, err := db.GetUniqueTeams(ctx)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teams)
}

func (h *OptionsHandler) HandleGetFormats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := db.GetUniqueFormats(ctx)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// HandleGetCanonicalFormats returns the canonical format codes (TEST, ODI, T20, T20I) from the formats package.
// Use this where the UI needs a stable list that matches backend semantics (e.g. ops status grids).
func (h *OptionsHandler) HandleGetCanonicalFormats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, formats.CanonicalCodes())
}

func (h *OptionsHandler) HandleGetTeamsByFormat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	format := r.URL.Query().Get("format")
	if format == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "format is required"})
		return
	}
	teams, err := db.GetTeamsByFormat(ctx, format)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teams)
}

func (h *OptionsHandler) HandleGetOpponents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	format := r.URL.Query().Get("format")
	team := r.URL.Query().Get("team")
	if format == "" || team == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "format and team are required"})
		return
	}
	opponents, err := db.GetOpponentsByFormatAndTeam(ctx, format, team)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, opponents)
}

func (h *OptionsHandler) HandleGetVenues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query().Get("q")
	venues, err := db.GetVenuesByQuery(ctx, q)
	if err != nil {
		respondErr(w, err)
		return
	}
	if venues == nil {
		venues = []string{}
	}
	writeJSON(w, http.StatusOK, venues)
}
