package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// DTO for response items
type matchItem struct {
	MatchID int64     `json:"match_id"`
	Date    string    `json:"date"` // YYYY-MM-DD
	Format  *string   `json:"format,omitempty"`
	Teams   [2]string `json:"teams"`
}

// testing seam for DAO
var listMatchesFunc = db.ListMatches

// listMatchesHandler handles GET /matches
// Query: season=YYYY (required), after=YYYY-MM-DD (required), format=TEST|ODI|T20I|T20 (optional)
func listMatchesHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	seasonStr := q.Get("season")
	afterStr := q.Get("after")
	format := q.Get("format")

	if seasonStr == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "missing season", Hint: "provide season=YYYY"},
		)
		return
	}
	season, err := strconv.Atoi(seasonStr)
	if err != nil || season < 1000 || season > 9999 {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "invalid season", Hint: "expected 4-digit year"},
		)
		return
	}

	if afterStr == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "missing after date", Hint: "provide after=YYYY-MM-DD"},
		)
		return
	}
	after, err := time.Parse("2006-01-02", afterStr)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "invalid after date", Hint: "expected format YYYY-MM-DD"},
		)
		return
	}

	rows, err := listMatchesFunc(r.Context(), season, after, format)
	if err != nil {
		respondErr(w, err)
		return
	}
	items := make([]matchItem, 0, len(rows))
	for _, row := range rows {
		var fmtPtr *string
		if row.FormatCode.Valid && row.FormatCode.String != "" {
			s := row.FormatCode.String
			fmtPtr = &s
		}
		items = append(items, matchItem{
			MatchID: row.MatchID,
			Date:    row.Date.Format("2006-01-02"),
			Format:  fmtPtr,
			Teams:   row.Teams,
		})
	}
	writeJSON(w, http.StatusOK, items)
}
