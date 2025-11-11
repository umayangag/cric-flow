package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/precompute"
)

// healthHandler responds with liveness OK.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readinessHandler pings the DB to verify readiness.
func readinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := db.Pool.Ping(ctx); err != nil {
		respondJSON(
			w,
			http.StatusServiceUnavailable,
			map[string]string{"status": "db_unavailable", "error": err.Error()},
		)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// precomputeHandler triggers precompute with optional filters.
// Optional JSON body: {"season":"2019", "formats":["ODI","T20I"]}
func precomputeHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Season  string   `json:"season"`
		Formats []string `json:"formats"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Error("error decoding the response", slog.Any("err", err))
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "error decoding the response"})
	}
	go func(season string, formats []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := precompute.Run(ctx, season, formats); err != nil {
			slog.Error(
				"precompute failed",
				slog.Any("err", err),
				slog.String("season", season),
				slog.Any("formats", formats),
			)
		} else {
			slog.Info("precompute completed", slog.String("season", season), slog.Any("formats", formats))
		}
	}(body.Season, body.Formats)
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// precomputeStatusHandler returns in-memory status of the last run.
func precomputeStatusHandler(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, precompute.GetStatus())
}

// importCricsheetHandler runs import of cricsheet data directory.
// Request body: {"dir":"../data", "placeholders_weather":true, "placeholders_fielding":true}
func importCricsheetHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir                  string `json:"dir"`
		PlaceholdersWeather  bool   `json:"placeholders_weather"`
		PlaceholdersFielding bool   `json:"placeholders_fielding"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {

	}
	if strings.TrimSpace(body.Dir) == "" {
		body.Dir = "../data"
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		opts := &cricsheet.Options{
			PlaceholdersWeather:  body.PlaceholdersWeather,
			PlaceholdersFielding: body.PlaceholdersFielding,
		}
		if n, err := cricsheet.ImportDir(ctx, body.Dir, opts); err != nil {
			slog.Error("cricsheet import failed", slog.Any("err", err))
		} else {
			slog.Info("cricsheet import completed", slog.Int("files", n))
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// getPlayerHandler returns player info with optional consistency stats.
func getPlayerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondBadRequest(w, err)
		return
	}

	season := r.URL.Query().Get("season")
	format := r.URL.Query().Get("format")

	// Get base player data
	player, err := db.GetPlayerByID(r.Context(), id)
	if err != nil {
		respondErr(w, err)
		return
	}

	// Get consistency data
	consistency, err := db.GetPlayerConsistency(r.Context(), id, season, format)
	if err != nil {
		// It's okay for consistency data to be missing, so just log the error
		slog.Warn(
			"could not get player consistency",
			slog.Any("err", err),
			slog.Int64("player_id", id),
			slog.String("season", season),
			slog.String("format", format),
		)
	}

	type respStruct struct {
		ID                 int64    `json:"id"`
		Name               string   `json:"player_name"`
		IsWicketKeeper     int16    `json:"is_wicket_keeper"`
		IsRetired          int16    `json:"is_retired"`
		BattingConsistency *float32 `json:"batting_consistency,omitempty"`
		BowlingConsistency *float32 `json:"bowling_consistency,omitempty"`
	}

	resp := respStruct{
		ID:             player.ID,
		Name:           player.Name,
		IsWicketKeeper: player.IsWicketKeeper,
		IsRetired:      player.IsRetired,
	}

	if consistency != nil {
		resp.BattingConsistency = &consistency.BattingConsistency
		resp.BowlingConsistency = &consistency.BowlingConsistency
	}

	respondJSON(w, http.StatusOK, resp)
}

// getMatchHandler returns match details for a given match id.
func getMatchHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	mid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondBadRequest(w, err)
		return
	}
	row := db.Pool.QueryRow(
		r.Context(),
		`SELECT id, match_id, venue_id, opposition_id, season_id, toss, batting_session, bowling_session FROM match_details WHERE match_id = $1`,
		mid,
	)
	var resp struct {
		ID             int64   `json:"id"`
		MatchID        int64   `json:"match_id"`
		VenueID        *int64  `json:"venue_id"`
		OppositionID   *int64  `json:"opposition_id"`
		SeasonID       *int64  `json:"season_id"`
		Toss           *string `json:"toss"`
		BattingSession *string `json:"batting_session"`
		BowlingSession *string `json:"bowling_session"`
	}
	if err := row.Scan(&resp.ID, &resp.MatchID, &resp.VenueID, &resp.OppositionID, &resp.SeasonID, &resp.Toss, &resp.BattingSession, &resp.BowlingSession); err != nil {
		respondErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// predictBattingHandler sends features to ML service for batting predictions.
func (a *app) predictBattingHandler(w http.ResponseWriter, r *http.Request) {
	var feats []contracts.BattingFeatures
	if err := json.NewDecoder(r.Body).Decode(&feats); err != nil {
		respondBadRequest(w, err)
		return
	}
	preds, err := a.mlClient.PredictBatting(r.Context(), feats)
	if err != nil {
		respondErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, preds)
}

// predictBowlingHandler sends features to ML service for bowling predictions.
func (a *app) predictBowlingHandler(w http.ResponseWriter, r *http.Request) {
	var feats []contracts.BowlingFeatures
	if err := json.NewDecoder(r.Body).Decode(&feats); err != nil {
		respondBadRequest(w, err)
		return
	}
	preds, err := a.mlClient.PredictBowling(r.Context(), feats)
	if err != nil {
		respondErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, preds)
}
