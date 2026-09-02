package server

import (
	"net/http"
	"strconv"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

type OptionsHandler struct{}

// teamSideResponse is one side as a picker offers it.
//
// It carries the club id because that is what a prediction request must send: a name alone
// names two teams for 130 of the 394 names in the dataset, and a picker that offers the name
// once cannot say which of them the user chose (D-10). `display_name` comes from the backend
// so the picker, the prediction's echo and the ambiguity error all spell a side the same way.
type teamSideResponse struct {
	ClubID      int64  `json:"club_id"`
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	DisplayName string `json:"display_name"`
}

func newTeamSideResponses(sides []db.TeamSide) []teamSideResponse {
	out := make([]teamSideResponse, 0, len(sides))
	for _, side := range sides {
		out = append(out, teamSideResponse{
			ClubID:      side.ClubID,
			Name:        side.Name,
			Gender:      side.Gender,
			DisplayName: side.Label(),
		})
	}
	return out
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
func (h *OptionsHandler) HandleGetCanonicalFormats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, formats.CanonicalCodes())
}

// HandleGetTeamSidesByFormat returns the sides that have played the format.
func (h *OptionsHandler) HandleGetTeamSidesByFormat(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Code: "INVALID_PARAM", Message: "format is required"})
		return
	}
	sides, err := db.ListTeamSidesForFormat(r.Context(), format)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newTeamSideResponses(sides))
}

// HandleGetOpponentSides returns the sides this club has played in the format.
//
// It takes the club id the teams list returned, not a name: asking for "the opponents of
// India" is the same unanswerable question one level along.
func (h *OptionsHandler) HandleGetOpponentSides(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	format := query.Get("format")
	clubID, err := strconv.ParseInt(query.Get("team_id"), 10, 64)
	if format == "" || err != nil || clubID <= 0 {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "format and team_id are required",
			Hint:    "team_id is the club_id from /api/options/teams-by-format",
		})
		return
	}
	sides, err := db.ListOpponentSidesForFormat(r.Context(), format, clubID)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newTeamSideResponses(sides))
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
