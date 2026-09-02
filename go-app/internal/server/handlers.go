package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// healthHandler responds with liveness OK.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// mlHealthClient is created per-request with config timeout to allow config reload; use getMLHealthClient().
func getMLHealthClient() *http.Client {
	cfg := config.Load()
	sec := config.ServerMLHealthTimeoutSec(cfg)
	return &http.Client{Timeout: time.Duration(sec) * time.Second}
}

// mlServiceProxy returns an http.HandlerFunc that proxies GET to the ML service at the given endpoint.
// label is used in log messages (e.g., "ml health proxy").
// enrich, if non-nil, is called with the decoded payload and request after successful decode; it may modify payload in place.
func (a *App) mlServiceProxy(
	endpoint string,
	label string,
	enrich func(map[string]any, *http.Request),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := strings.TrimSpace(os.Getenv("ML_SERVICE_URL"))
		if base == "" {
			base = config.ServerMLBaseURLFallback(config.Load())
		}
		base = strings.TrimSuffix(base, "/")
		cfg := config.Load()
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.ServerMLHealthTimeoutSec(cfg))*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+endpoint, nil)
		if err != nil {
			slog.Warn(label+": new request failed", slog.Any("err", err))
			respondJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": err.Error()})
			return
		}
		client := getMLHealthClient()
		resp, err := client.Do(req)
		if err != nil {
			slog.Warn(label+": request failed", slog.Any("err", err))
			respondJSON(w, http.StatusBadGateway, map[string]string{"status": "error", "error": err.Error()})
			return
		}
		defer resp.Body.Close()
		maxBody := config.ServerMLHealthBodyLimitBytes(config.Load())
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxBody)))
			slog.Warn(
				label+": upstream non-2xx",
				slog.Int("status", resp.StatusCode),
				slog.String("body", string(body)),
			)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(io.LimitReader(resp.Body, int64(maxBody))).Decode(&payload); err != nil {
			slog.Warn(label+": decode failed", slog.Any("err", err))
			respondJSON(
				w,
				http.StatusInternalServerError,
				map[string]string{"status": "error", "error": "invalid " + label + " response"},
			)
			return
		}
		if enrich != nil {
			enrich(payload, r)
		}
		respondJSON(w, http.StatusOK, payload)
	}
}

// readinessHandler pings the DB to verify readiness.
func readinessHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.ServerReadinessTimeoutSec(cfg))*time.Second)
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

// importCricSheetHandler runs import of the cricsheet data directory.
//
// Request body: {"dir":"<optional override>", "placeholders_fielding":true, "refresh":true}
//
// With no dir it *acquires and then imports* the configured archive (consumer plan
// W6-2): fetch, extract, import, as one run plan. Getting a dataset onto the box used
// to be three operator actions across two tabs, with the Data tab's own caption
// admitting it — "Once extracted, run Import from Ops Status". The archive URL is
// configuration, because a deployment pulls the same one every time.
//
// Fetch and extract are skipped, visibly and with a reason, when the directory already
// holds data from that source; `refresh` overrides that. An explicit `dir` means the
// caller is naming data they already have, so nothing is acquired.
func (a *App) importCricSheetHandler(w http.ResponseWriter, r *http.Request) {
	var body cricSheetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("import: decode request body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	// The console triggers steps with query parameters, the CLI and tests with a body.
	// Reading both is one line; making the operator's Re-download button work only from
	// one of them is a bug report.
	if v := strings.TrimSpace(r.URL.Query().Get("refresh")); v == "1" || strings.EqualFold(v, "true") {
		body.Refresh = true
	}
	dir := strings.TrimSpace(body.Dir)
	if dir == "" {
		if a.startImportPlan(w, r, body) {
			return
		}
		dir = dataset.Dir()
	}
	opts := &cricsheet.Options{
		PlaceholdersFielding: body.PlaceholdersFielding,
	}
	if busy, _ := pipeline.LaneBusy(r.Context(), "cricsheet-import"); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": pipeline.ErrPipelineBusy.Error()})
		return
	}
	slog.Info("import: request accepted, starting background job", slog.String("dir", dir))
	importLane := pipelinesvc.Steps().LaneForCommand("cricsheet-import")
	jobCtx, cancel := context.WithCancel(a.JobContext())
	a.SetJobCancel(importLane, cancel)
	go func() {
		defer a.ClearJobCancel(importLane)
		slog.Info("cricsheet import job started", slog.String("dir", dir))
		runErr := pipeline.RunJob(
			jobCtx,
			"cricsheet-import",
			map[string]any{"dir": dir},
			config.PipelineTimeout(),
			func(ctx context.Context) (any, error) {
				n, err := cricsheet.ImportDir(ctx, dir, opts, 0)
				return map[string]any{"files": n, "dir": dir}, err
			},
		)
		if runErr != nil {
			slog.Error("cricsheet import job failed", slog.String("dir", dir), slog.Any("err", runErr))
		} else {
			slog.Info("cricsheet import job completed successfully", slog.String("dir", dir))
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
		slog.Info("getPlayer: invalid player id", slog.String("id", idStr), slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}

	season := r.URL.Query().Get("season")
	format := r.URL.Query().Get("format")

	// Get base player data
	player, err := db.GetPlayerByID(r.Context(), id)
	if err != nil {
		slog.Error("getPlayer: GetPlayerByID failed", slog.Int64("player_id", id), slog.Any("err", err))
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
		slog.Info("getMatch: invalid match id", slog.String("id", idStr), slog.Any("err", err))
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
		slog.Error("getMatch: query/scan failed", slog.Int64("match_id", mid), slog.Any("err", err))
		respondErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}
