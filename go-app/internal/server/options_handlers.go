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
