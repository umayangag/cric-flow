package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	formatsPkg "github.com/umayangag/cric-flow/go-app/internal/formats"
	"github.com/umayangag/cric-flow/go-app/internal/models"
	"github.com/umayangag/cric-flow/go-app/internal/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/precompute"
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

// enrichModelStatsPayload adds migration info (trained_at, duration) and params/metrics from ml_tuned_params
// to each model when available. DB data takes precedence over disk-based model-stats so the latest auto-tune
// results are shown even when tuning_report_*.json on disk is stale.
// It also attaches the static match-type hierarchy so the ML Model Stats tab can render it
// without a separate API call.
func enrichModelStatsPayload(payload map[string]any, r *http.Request) {
	payload["hierarchy"] = formatsPkg.GetHierarchy()

	modelsVal, ok := payload["models"]
	if !ok {
		return
	}
	modelsList, ok := modelsVal.([]any)
	if !ok || !db.Available() {
		return
	}
	ctx := r.Context()

	// Migration info (trained_at, duration)
	migrationInfo, err := db.GetMigrationInfoForTunedParams(ctx)
	if err != nil {
		slog.Warn("ml model-stats proxy: migration info fetch failed", slog.Any("err", err))
	}
	// Params and metrics from ml_tuned_params (source of truth for latest auto-tune)
	paramsMetrics, err := db.ListLatestParamsMetricsForModelStats(ctx)
	if err != nil {
		slog.Warn("ml model-stats proxy: params/metrics fetch failed", slog.Any("err", err))
	}

	for _, m := range modelsList {
		modelMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		modelName, ok := modelMap["model_name"].(string)
		if !ok {
			continue
		}
		matchFormat, _ := modelMap["match_format"].(string)
		formatKey := matchFormat
		if matchFormat == "Unified" || matchFormat == "" {
			formatKey = ""
		}
		// model_kind is the machine-readable kind (e.g. "batting_share");
		// fall back to lowercased model_name for backward compatibility.
		kindStr, _ := modelMap["model_kind"].(string)
		if kindStr == "" {
			kindStr = strings.ToLower(modelName)
		}
		key := kindStr + "|" + formatKey

		if info, has := migrationInfo[key]; has {
			modelMap["trained_at"] = info.TrainedAt
			if info.CompletedAt != "" {
				modelMap["completed_at"] = info.CompletedAt
			}
			if info.DurationSecs > 0 {
				modelMap["duration_seconds"] = info.DurationSecs
			}
		}

		// Override with latest params/metrics from ml_tuned_params (DB is source of truth after auto-tune)
		if paramsMetrics != nil {
			if pm, has := paramsMetrics[key]; has {
				enrichWithDBParams(modelMap, pm.Params)
				enrichWithDBMetrics(modelMap, pm.Metrics)
			}
		}
	}
}

// enrichWithDBParams enriches modelMap with tuned params from DB (tuned_parameters, algorithm).
func enrichWithDBParams(modelMap map[string]any, params json.RawMessage) {
	if len(params) == 0 {
		return
	}
	var p map[string]any
	if err := json.Unmarshal(params, &p); err != nil || len(p) == 0 {
		return
	}
	modelMap["tuned_parameters"] = p
	modelMap["tuned"] = true
	if algo := p["algorithm"]; algo != nil {
		modelMap["algorithm"] = algorithmDisplayName(fmt.Sprintf("%v", algo))
	} else if algos, ok := p["algorithms"].([]any); ok && len(algos) > 0 {
		modelMap["algorithm"] = algorithmDisplayName(fmt.Sprintf("%v", algos[0]))
	}
}

// enrichWithDBMetrics enriches modelMap with metrics from DB (metrics, mlqa_audit, accuracy_display).
func enrichWithDBMetrics(modelMap map[string]any, metrics json.RawMessage) {
	if len(metrics) == 0 {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(metrics, &m); err != nil || len(m) == 0 {
		return
	}
	modelMap["metrics"] = m
	modelMap["tuned"] = true
	if mlqa, ok := m["mlqa_audit"].(map[string]any); ok && len(mlqa) > 0 {
		modelMap["mlqa_audit"] = mlqa
	}
	if disp := formatAccuracyDisplayFromMetrics(m); disp != "" {
		modelMap["accuracy_display"] = disp
	}
}

// formatAccuracyDisplayFromMetrics builds the accuracy_display string from metrics.
func formatAccuracyDisplayFromMetrics(metrics map[string]any) string {
	if acc := metrics["accuracy_pct"]; acc != nil {
		return formatAccuracyPct(acc)
	}
	if maeVal, ok := metrics["mae"]; ok {
		if mae, ok := toFloat64(maeVal); ok {
			parts := []string{fmt.Sprintf("MAE=%.2f", mae)}
			if rmseVal, ok := metrics["rmse"]; ok {
				if rmse, ok := toFloat64(rmseVal); ok {
					parts = append(parts, fmt.Sprintf("RMSE=%.2f", rmse))
				}
			}
			if r2Val, ok := metrics["r2_pct"]; ok {
				if r2, ok := toFloat64(r2Val); ok {
					parts = append(parts, fmt.Sprintf("R²=%.1f%%", r2))
				}
			}
			return strings.Join(parts, ", ")
		}
	}
	if r2Val, ok := metrics["r2_pct"]; ok {
		if r2, ok := toFloat64(r2Val); ok {
			return fmt.Sprintf("R²=%.1f%%", r2)
		}
	}
	return ""
}

// toFloat64 safely converts an any value to float64 if it's a known numeric type.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func formatAccuracyPct(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%.1f%%", x)
	case int:
		return fmt.Sprintf("%d%%", x)
	case string:
		return x + "%"
	default:
		return fmt.Sprintf("%v%%", v)
	}
}

var algorithmDisplayNames = map[string]string{
	"rf": "Random Forest", "gb": "Gradient Boosting", "quantile": "Quantile Regressor",
	"stacked": "Stacking Regressor", "ridge": "Ridge", "mlp": "MLP Regressor",
	"et": "Extra Trees", "hgb": "Hist Gradient Boosting",
}

func algorithmDisplayName(key string) string {
	trimmedKey := strings.TrimSpace(key)
	k := strings.ToLower(trimmedKey)
	if name, ok := algorithmDisplayNames[k]; ok {
		return name
	}
	return trimmedKey
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

// precomputeHandler triggers precompute with optional filters.
// Optional JSON body: {"season":"2019", "formats":["ODI","T20I"]}. Empty body is allowed (defaults to all seasons/formats).
func (a *App) precomputeHandler(w http.ResponseWriter, r *http.Request) {
	var body precomputeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("precompute: decode request body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	season := body.Season
	formats := body.Formats
	// Align with export: when no formats specified and config uses split-by-format, use the same canonical list.
	if len(formats) == 0 {
		if cfg := config.Load(); cfg != nil && cfg.Export.SplitByFormat {
			formats = formatsPkg.CanonicalCodes()
		}
	}
	if busy, _ := pipeline.HasPipelineBusy(r.Context()); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "another pipeline step is already running"})
		return
	}
	slog.Info(
		"precompute: request accepted, starting background job",
		slog.String("season", season),
		slog.Any("formats", formats),
	)
	jobCtx, cancel := context.WithCancel(a.JobContext())
	a.SetCurrentJobCancel(cancel)
	go func() {
		defer a.ClearCurrentJobCancel()
		timeout := config.PipelineTimeout()
		slog.Info(
			"precompute job started",
			slog.Duration("timeout", timeout),
			slog.String("season", season),
			slog.Any("formats", formats),
		)
		runErr := pipeline.RunJob(
			jobCtx,
			"precompute-features",
			map[string]any{"season": season, "formats": formats},
			timeout,
			func(ctx context.Context) (any, error) {
				err := precompute.Run(ctx, season, formats, nil)
				return map[string]any{"season": season, "formats": formats}, err
			},
		)
		if runErr != nil {
			slog.Error(
				"precompute job failed (DB connections may show 'connection to client lost' if cancelled or crashed)",
				slog.Any("err", runErr),
				slog.String("season", season),
				slog.Any("formats", formats),
			)
		} else {
			slog.Info("precompute job completed successfully", slog.String("season", season), slog.Any("formats", formats))
		}
	}()
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// precomputeStatusHandler returns in-memory status of the last run.
func precomputeStatusHandler(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, precompute.GetStatus())
}

// importCricSheetHandler runs import of cricsheet data directory.
// Request body: {"dir":"../data", "placeholders_weather":true, "placeholders_fielding":true}
func (a *App) importCricSheetHandler(w http.ResponseWriter, r *http.Request) {
	var body cricSheetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("import: decode request body failed", slog.Any("err", err))
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
	if busy, _ := pipeline.HasPipelineBusy(r.Context()); busy {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "another pipeline step is already running"})
		return
	}
	slog.Info("import: request accepted, starting background job", slog.String("dir", dir))
	jobCtx, cancel := context.WithCancel(a.JobContext())
	a.SetCurrentJobCancel(cancel)
	go func() {
		defer a.ClearCurrentJobCancel()
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

// predictBattingHandler sends features to mlCleint service for batting predictions.
func (a *App) predictBattingHandler(w http.ResponseWriter, r *http.Request) {
	var feats []models.BattingFeatures
	if err := json.NewDecoder(r.Body).Decode(&feats); err != nil {
		slog.Info("predictBatting: decode body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	preds, err := a.mlClient.PredictBatting(r.Context(), feats)
	if err != nil {
		slog.Error(
			"predictBatting: ML client PredictBatting failed",
			slog.Int("features_count", len(feats)),
			slog.Any("err", err),
		)
		respondErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, preds)
}

// predictBowlingHandler sends features to mlCleint service for bowling predictions.
func (a *App) predictBowlingHandler(w http.ResponseWriter, r *http.Request) {
	var feats []models.BowlingFeatures
	if err := json.NewDecoder(r.Body).Decode(&feats); err != nil {
		slog.Info("predictBowling: decode body failed", slog.Any("err", err))
		respondBadRequest(w, err)
		return
	}
	preds, err := a.mlClient.PredictBowling(r.Context(), feats)
	if err != nil {
		slog.Error(
			"predictBowling: ML client PredictBowling failed",
			slog.Int("features_count", len(feats)),
			slog.Any("err", err),
		)
		respondErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, preds)
}
