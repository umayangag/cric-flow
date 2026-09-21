package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// actorHeader names the user whose retirement ledger a request reads and writes.
//
// A deployment carries one API key today, so the header is almost always absent and the
// ledger is the default user's. It is read rather than assumed because the difference
// between "this user thinks he has retired" and "he has retired" is the distinction the
// ledger is built on, and that distinction needs a user to be about.
const actorHeader = "X-User-Id"

// actorFrom returns whose ledger this request is about.
func actorFrom(r *http.Request) string {
	if actor := strings.TrimSpace(r.Header.Get(actorHeader)); actor != "" {
		return actor
	}
	return availability.DefaultActor
}

// candidatesHandler answers GET /api/options/candidates: the list a user picks a pool
// out of, with each player's last-played date and any ledger exclusion marked (D-12).
func (a *App) candidatesHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	format := strings.TrimSpace(query.Get("format"))
	clubID := parseClubID(query.Get("club_id"))
	if format == "" || clubID == 0 {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "format and club_id are required",
			Hint:    "club_id is the id from /api/options/teams-by-format",
		})
		return
	}
	matchDate, apiErr := parseCandidatesDate(query.Get("match_date"))
	if apiErr != nil {
		writeJSON(w, http.StatusBadRequest, *apiErr)
		return
	}
	request, apiErr := parsePoolRequest(query.Get("window_months"), query.Get("all_time"), nil)
	if apiErr != nil {
		writeJSON(w, http.StatusBadRequest, *apiErr)
		return
	}

	result, err := predictteam.Candidates(r.Context(), predictteam.CandidatesInput{
		Format:    format,
		Team:      db.TeamRef{ClubID: clubID},
		MatchDate: matchDate,
		Request:   request,
		Actor:     actorFrom(r),
	})
	if err != nil {
		respondPredictErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// parseCandidatesDate reads the cutoff the window ends at. An absent value is today:
// the candidate list is opened while planning an upcoming match, and today's window is
// the one that will be used unless a date says otherwise.
func parseCandidatesDate(raw string) (time.Time, *apiError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return availability.CalendarDay(time.Now().UTC()), nil
	}
	parsed, err := parseMatchDate(raw)
	if err != nil {
		return time.Time{}, &apiError{
			Code:    "INVALID_PARAM",
			Message: "match_date must be RFC3339 or YYYY-MM-DD: " + err.Error(),
		}
	}
	return parsed, nil
}

// parsePoolRequest reads the pool scope a caller asked for: the window, whether to widen
// it to all-time, and a manual pick.
//
// A window of zero is not "no window" — it is "the per-format default". There is no way
// to ask for an unbounded pool by leaving something out; all-time is a value a caller
// spells, because the unbounded pool is the defect D-12 fixes.
func parsePoolRequest(windowMonths, allTime string, manual []int64) (predictteam.PoolRequest, *apiError) {
	request := predictteam.PoolRequest{Manual: manual}
	if raw := strings.TrimSpace(allTime); raw != "" {
		request.AllTime = strings.EqualFold(raw, "true") || raw == "1"
	}
	raw := strings.TrimSpace(windowMonths)
	if raw == "" {
		return request, nil
	}
	months, err := strconv.Atoi(raw)
	if err != nil || months <= 0 {
		return predictteam.PoolRequest{}, &apiError{
			Code:    "INVALID_PARAM",
			Message: "window_months must be a positive whole number of months",
			Hint:    "leave it out for the per-format default, or send all_time=true to widen the pool",
		}
	}
	request.WindowMonths = months
	return request, nil
}

// retirementFlagHandler answers POST and DELETE on
// /api/players/{id}/retirement: a user's claim that a player has retired, and its
// withdrawal.
//
// POST records the claim and promotes it to the stored `is_retired` fact only where a
// criterion corroborates it; the response says which criterion did, or which checks could
// not be made, so a user is never left to guess why an exclusion is theirs alone.
func (a *App) retirementFlagHandler(w http.ResponseWriter, r *http.Request) {
	playerID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || playerID <= 0 {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "player id must be a positive whole number",
		})
		return
	}
	ledger := availability.NewLedger(db.NewPlayerStatusStore(), availability.Criteria(config.Load()))
	actor := actorFrom(r)

	if r.Method == http.MethodDelete {
		a.respondUnflag(w, r, ledger, actor, playerID)
		return
	}
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	result, err := ledger.Flag(r.Context(), actor, playerID, format, time.Now().UTC())
	if err != nil {
		slog.Error("retirementFlag: flag failed", slog.Int64("player_id", playerID), slog.Any("err", err))
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newRetirementStatus(result))
}

// respondUnflag withdraws a claim and reports whether one was there to withdraw.
func (a *App) respondUnflag(
	w http.ResponseWriter,
	r *http.Request,
	ledger *availability.Ledger,
	actor string,
	playerID int64,
) {
	removed, existed, err := ledger.Unflag(r.Context(), actor, playerID)
	if err != nil {
		slog.Error("retirementFlag: unflag failed", slog.Int64("player_id", playerID), slog.Any("err", err))
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, retirementStatus{
		PlayerID: playerID,
		Flagged:  false,
		Existed:  existed,
		Demoted:  existed && removed.Promoted(),
	})
}

// retirementStatus is what a flag or an un-flag did, on the wire.
//
// Promoted and Criterion are the honest part: a claim that corroborated nothing hides the
// player from this user's pools and from nobody else's, and the response says so rather
// than letting a user believe he has changed a fact about the player.
type retirementStatus struct {
	PlayerID int64 `json:"player_id"`
	// Flagged is whether a claim now stands for this user.
	Flagged bool `json:"flagged"`
	// Promoted is whether the claim was corroborated into the stored `is_retired` fact.
	Promoted bool `json:"promoted"`
	// Criterion names what corroborated it, and Detail is the evidence it read.
	Criterion string `json:"criterion,omitempty"`
	Detail    string `json:"detail,omitempty"`
	// Unchecked names the criteria whose evidence does not exist yet (X-1a).
	Unchecked []string `json:"unchecked,omitempty"`
	// Notes is what each criterion that answered said, in the order they were tried.
	Notes []string `json:"notes,omitempty"`
	// Existed and Demoted describe an un-flag: whether there was a claim, and whether
	// withdrawing it lowered the stored fact.
	Existed bool `json:"existed,omitempty"`
	Demoted bool `json:"demoted,omitempty"`
}

// newRetirementStatus renders a flag's outcome.
func newRetirementStatus(result availability.FlagResult) retirementStatus {
	return retirementStatus{
		PlayerID:  result.Flag.PlayerID,
		Flagged:   true,
		Promoted:  result.Flag.Promoted(),
		Criterion: result.Flag.Criterion,
		Detail:    result.Flag.Detail,
		Unchecked: result.Unchecked,
		Notes:     result.Notes,
	}
}
