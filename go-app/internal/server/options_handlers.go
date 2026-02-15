package server

import (
	"net/http"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
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
	formats, err := db.GetUniqueFormats(ctx)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, formats)
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
