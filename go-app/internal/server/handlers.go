package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/pipeline"
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
// Optional JSON body: {"season":"2019", "formats":["ODI","T20I"]}. Empty body is allowed (defaults to all seasons/formats).
func precomputeHandler(w http.ResponseWriter, r *http.Request) {
	var body precomputeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("error decoding precompute request body", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	season := body.Season
	formats := body.Formats
	go func() {
		timeout := precomputeHandlerTimeout()
		slog.Info("precompute started", slog.Duration("timeout", timeout), slog.String("season", season), slog.Any("formats", formats))
		runErr := pipeline.RunJob(context.Background(), "precompute-features", map[string]any{"season": season, "formats": formats}, timeout, func(ctx context.Context) (any, error) {
			err := precompute.Run(ctx, season, formats, nil)
			return map[string]any{"season": season, "formats": formats}, err
		})
		if runErr != nil {
			slog.Error("precompute failed (DB connections may show 'connection to client lost' if cancelled or crashed)",
				slog.Any("err", runErr), slog.String("season", season), slog.Any("formats", formats))
		} else {
			slog.Info("precompute completed", slog.String("season", season), slog.Any("formats", formats))
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// precomputeHandlerTimeout returns the timeout for the precompute handler goroutine.
// Uses features.precompute_timeout_ms from config (validated at startup). 0 = no timeout.
func precomputeHandlerTimeout() time.Duration {
	cfg := config.Load()
	ms := cfg.Features.PrecomputeTimeoutMs
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// precomputeStatusHandler returns in-memory status of the last run.
func precomputeStatusHandler(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, precompute.GetStatus())
}

// importCricSheetHandler runs import of cricsheet data directory.
// Request body: {"dir":"../data", "placeholders_weather":true, "placeholders_fielding":true}
func importCricSheetHandler(w http.ResponseWriter, r *http.Request) {
	var body cricSheetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("error decoding import request body", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	if strings.TrimSpace(body.Dir) == "" {
		body.Dir = "../data"
	}
	dir := body.Dir
	opts := &cricsheet.Options{
		PlaceholdersWeather:  body.PlaceholdersWeather,
		PlaceholdersFielding: body.PlaceholdersFielding,
	}
	go func() {
		slog.Info("cricsheet import started", slog.String("dir", dir))
		runErr := pipeline.RunJob(context.Background(), "cricsheet-import", map[string]any{"dir": dir}, 10*time.Minute, func(ctx context.Context) (any, error) {
			n, err := cricsheet.ImportDir(ctx, dir, opts)
			return map[string]any{"files": n, "dir": dir}, err
		})
		if runErr != nil {
			slog.Error("cricsheet import failed", slog.Any("err", runErr))
		} else {
			slog.Info("cricsheet import completed", slog.String("dir", dir))
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

	resp := playerResponse{
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
		`SELECT m.match_id, m.match_id, m.venue_id, mi.batting_team_opposition_id, m.season_id, m.toss_decision
		FROM match m
		LEFT JOIN LATERAL (SELECT batting_team_opposition_id FROM match_inning WHERE match_id = m.match_id ORDER BY inning_number LIMIT 1) mi ON true
		WHERE m.match_id = $1`,
		mid,
	)

	var resp matchDetailsResponse
	if err := row.Scan(&resp.ID, &resp.MatchID, &resp.VenueID, &resp.OppositionID, &resp.SeasonID, &resp.Toss); err != nil {
		respondErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// predictBattingHandler sends features to mlCleint service for batting predictions.
func (a *App) predictBattingHandler(w http.ResponseWriter, r *http.Request) {
	var feats []models.BattingFeatures
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

// predictBowlingHandler sends features to mlCleint service for bowling predictions.
func (a *App) predictBowlingHandler(w http.ResponseWriter, r *http.Request) {
	var feats []models.BowlingFeatures
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
