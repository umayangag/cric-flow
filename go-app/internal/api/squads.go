package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Data model returned by DAO layer
type playerPredictionDTO struct {
	PlayerName      string  `json:"player_name"`
	RunsScored      float64 `json:"runs_scored"`
	BallsFaced      float64 `json:"balls_faced"`
	FoursScored     float64 `json:"fours_scored"`
	SixesScored     float64 `json:"sixes_scored"`
	BattingPosition float64 `json:"batting_position"`
	StrikeRate      float64 `json:"strike_rate"`
	RunsConceded    float64 `json:"runs_conceded"`
	Deliveries      float64 `json:"deliveries"`
	WicketsTaken    float64 `json:"wickets_taken"`
	Econ            float64 `json:"econ"`
}

type squadDTO struct {
	TeamName  string                `json:"team_name"`
	ActualWin int                   `json:"actual_win"`
	Players   []playerPredictionDTO `json:"players"`
}

type matchSquadsData struct {
	MatchID int64       `json:"match_id"`
	Date    time.Time   `json:"date"`
	Teams   [2]string   `json:"teams"`
	Squads  [2]squadDTO `json:"squads"`
}

// Response shape
type matchSquadsResponse struct {
	MatchID int64      `json:"match_id"`
	Date    string     `json:"date"`
	Teams   [2]string  `json:"teams"`
	Squads  []squadDTO `json:"squads"`
}

// Seam for DAO integration
type getMatchSquadsDAOFunc func(matchID int64, asof time.Time, format string) (matchSquadsData, error)

var getMatchSquadsFunc getMatchSquadsDAOFunc

// Sentinel errors used by DAO to signal typed failures
var (
	errMatchNotFound   = errors.New("match_not_found")
	errIncompleteSquad = errors.New("incomplete_squads")
)

// getMatchSquadsHandler handles GET /match/{id}/squads
// Query: asof=YYYY-MM-DD (required), format=TEST|ODI|T20I|T20 (optional)
func getMatchSquadsHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    idStr := vars["id"]
    if idStr == "" {
        writeJSON(
            w,
            http.StatusBadRequest,
            apiError{
                Code:    "INVALID_PARAM",
                Message: "missing id",
                Hint:    "provide path /match/{id}/squads with numeric id",
            },
        )
        return
    }
	matchID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || matchID <= 0 {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "invalid id", Hint: "id must be a positive integer"},
		)
		return
	}

	q := r.URL.Query()
	asofStr := q.Get("asof")
	if asofStr == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "missing asof date", Hint: "provide asof=YYYY-MM-DD"},
		)
		return
	}
	asof, err := time.Parse("2006-01-02", asofStr)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			apiError{Code: "INVALID_PARAM", Message: "invalid asof date", Hint: "expected format YYYY-MM-DD"},
		)
		return
	}
	format := q.Get("format")

	if getMatchSquadsFunc == nil {
		// until wired to DAO, return not implemented
		writeJSON(
			w,
			http.StatusNotImplemented,
			apiError{Code: "NOT_IMPLEMENTED", Message: "squads retrieval not implemented yet"},
		)
		return
	}

	data, err := getMatchSquadsFunc(matchID, asof, format)
	if err != nil {
		switch {
		case errors.Is(err, errMatchNotFound) || errors.Is(err, db.ErrMatchNotFound):
			writeJSON(w, http.StatusNotFound, apiError{Code: "NOT_FOUND", Message: "match not found"})
			return
		case errors.Is(err, errIncompleteSquad) || errors.Is(err, db.ErrIncompleteSquads):
			writeJSON(
				w,
				http.StatusUnprocessableEntity,
				apiError{
					Code:    "INCOMPLETE_SQUADS",
					Message: "one or both squads are incomplete",
					Hint:    "ensure both teams and players exist for the match",
				},
			)
			return
		default:
			respondErr(w, err)
			return
		}
	}

	resp := matchSquadsResponse{
		MatchID: data.MatchID,
		Date:    data.Date.Format("2006-01-02"),
		Teams:   data.Teams,
		Squads:  []squadDTO{data.Squads[0], data.Squads[1]},
	}
	writeJSON(w, http.StatusOK, resp)
}

// Wire the DAO seam to the real DB implementation and map structs.
func init() {
	getMatchSquadsFunc = func(matchID int64, asof time.Time, format string) (matchSquadsData, error) {
		ms, err := db.GetMatchSquads(context.Background(), matchID, asof, format)
		if err != nil {
			return matchSquadsData{}, err
		}
		// Map DB structs to handler DTOs
		toPlayer := func(p db.PlayerPredictionRow) playerPredictionDTO {
			return playerPredictionDTO{
				PlayerName:      p.PlayerName,
				RunsScored:      p.RunsScored,
				BallsFaced:      p.BallsFaced,
				FoursScored:     p.FoursScored,
				SixesScored:     p.SixesScored,
				BattingPosition: p.BattingPosition,
				StrikeRate:      p.StrikeRate,
				RunsConceded:    p.RunsConceded,
				Deliveries:      p.Deliveries,
				WicketsTaken:    p.WicketsTaken,
				Econ:            p.Econ,
			}
		}
		toSquad := func(s db.SquadRow) squadDTO {
			players := make([]playerPredictionDTO, 0, len(s.Players))
			for _, pr := range s.Players {
				players = append(players, toPlayer(pr))
			}
			return squadDTO{TeamName: s.TeamName, ActualWin: s.ActualWin, Players: players}
		}
		return matchSquadsData{
			MatchID: ms.MatchID,
			Date:    ms.Date,
			Teams:   ms.Teams,
			Squads:  [2]squadDTO{toSquad(ms.Squads[0]), toSquad(ms.Squads[1])},
		}, nil
	}
}
