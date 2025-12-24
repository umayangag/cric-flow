package server

import (
	"net/http"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// seam for testing
var getNextSeasonFunc = db.GetNextSeasonAfter

type nextSeasonResponse struct {
	NextSeason *int `json:"next_season"`
}

// apiError and writeJSON moved to json.go for shared use across handlers.

// getNextSeasonHandler handles GET /seasons/next
// Query: cutoff=YYYY-MM-DD (required), format=TEST|ODI|T20I|T20 (optional)
func getNextSeasonHandler(w http.ResponseWriter, r *http.Request) {
	cutoffStr := r.URL.Query().Get("cutoff")
	if cutoffStr == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "missing cutoff date", Hint: "provide cutoff=YYYY-MM-DD"},
		)
		return
	}
	cutoff, err := time.Parse("2006-01-02", cutoffStr)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "invalid cutoff date", Hint: "expected format YYYY-MM-DD"},
		)
		return
	}
	format := r.URL.Query().Get("format")

	ns, err := getNextSeasonFunc(r.Context(), cutoff, format)
	if err != nil {
		respondErr(w, err)
		return
	}
	if ns.Valid {
		v := int(ns.Int64)
		writeJSON(w, http.StatusOK, nextSeasonResponse{NextSeason: &v})
		return
	}
	writeJSON(w, http.StatusOK, nextSeasonResponse{NextSeason: nil})
}
