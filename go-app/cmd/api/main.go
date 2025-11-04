// Command api starts the HTTP API server for the cricket data service.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/precompute"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	// Run migrations on startup (idempotent)
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "/migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	r := mux.NewRouter()
	// Liveness
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}).Methods(http.MethodGet)
	// Readiness (checks DB connectivity)
	r.HandleFunc("/readiness", func(w http.ResponseWriter, r *http.Request) {
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
	}).Methods(http.MethodGet)

	// POST /precompute
	// Optional JSON body: {"season":"2019", "formats":["ODI","T20I"]}
	r.HandleFunc("/precompute", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Season  string   `json:"season"`
			Formats []string `json:"formats"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		go func(season string, formats []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := precompute.Run(ctx, season, formats); err != nil {
				log.Printf("precompute failed: %v", err)
			} else {
				log.Printf("precompute completed: season=%s formats=%v", season, formats)
			}
		}(body.Season, body.Formats)
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
	}).Methods(http.MethodPost)

	// Cricinfo scraping has been removed.

	// POST /import/cricsheet {"dir":"../data", "placeholders_weather":true, "placeholders_fielding":true}
	r.HandleFunc("/import/cricsheet", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Dir                  string `json:"dir"`
			PlaceholdersWeather  bool   `json:"placeholders_weather"`
			PlaceholdersFielding bool   `json:"placeholders_fielding"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
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
				log.Printf("cricsheet import failed: %v", err)
			} else {
				log.Printf("cricsheet import completed: %d files", n)
			}
		}()
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
	}).Methods(http.MethodPost)

	// GET /players/{id}?season=2019&format=T20
	r.HandleFunc("/players/{id}", func(w http.ResponseWriter, r *http.Request) {
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
			log.Printf("could not get player consistency: %v", err)
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
	}).Methods(http.MethodGet)

	// GET /matches/{id}
	r.HandleFunc("/matches/{id}", func(w http.ResponseWriter, r *http.Request) {
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
	}).Methods(http.MethodGet)

	// POST /predict/batting
	r.HandleFunc("/predict/batting", func(w http.ResponseWriter, r *http.Request) {
		var feats []contracts.BattingFeatures
		if err := json.NewDecoder(r.Body).Decode(&feats); err != nil {
			respondBadRequest(w, err)
			return
		}
		cli := mlclient.New()
		preds, err := cli.PredictBatting(r.Context(), feats)
		if err != nil {
			respondErr(w, err)
			return
		}
		respondJSON(w, http.StatusOK, preds)
	}).Methods(http.MethodPost)

	// POST /predict/bowling
	r.HandleFunc("/predict/bowling", func(w http.ResponseWriter, r *http.Request) {
		var feats []contracts.BowlingFeatures
		if err := json.NewDecoder(r.Body).Decode(&feats); err != nil {
			respondBadRequest(w, err)
			return
		}
		cli := mlclient.New()
		preds, err := cli.PredictBowling(r.Context(), feats)
		if err != nil {
			respondErr(w, err)
			return
		}
		respondJSON(w, http.StatusOK, preds)
	}).Methods(http.MethodPost)

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	log.Printf("API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func respondJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func respondErr(w http.ResponseWriter, err error) {
	respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func respondBadRequest(w http.ResponseWriter, err error) {
	respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
