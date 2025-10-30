package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-app/go-app/internal/contracts"
	"github.com/umayangag/cric-app/go-app/internal/db"
	"github.com/umayangag/cric-app/go-app/internal/features"
	"github.com/umayangag/cric-app/go-app/internal/mlclient"
	"github.com/umayangag/cric-app/go-app/internal/scrape"
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
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}).Methods(http.MethodGet)

	// POST /precompute {"season":"2019"}
	r.HandleFunc("/precompute", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Season string `json:"season"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		if err := features.ComputeSeasonalForm(ctx, body.Season); err != nil { respondErr(w, err); return }
		if err := features.ComputeVenueEffects(ctx); err != nil { respondErr(w, err); return }
		if err := features.ComputeOppositionEffects(ctx); err != nil { respondErr(w, err); return }
		if err := features.UpdatePlayerConsistency(ctx); err != nil { respondErr(w, err); return }
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}).Methods(http.MethodPost)

	// POST /scrape {"team":"Sri Lanka","from":"2010-01-01","to":"2010-02-01","limit":5}
	r.HandleFunc("/scrape", func(w http.ResponseWriter, r *http.Request) {
		var req struct{
			Team  string `json:"team"`
			From  string `json:"from"`
			To    string `json:"to"`
			Limit int    `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil { respondBadRequest(w, err); return }
		if req.Team == "" { req.Team = "Sri Lanka" }
		if req.From == "" { req.From = "2010-01-01" }
		if req.To == "" { req.To = "2010-02-01" }
		if req.Limit <= 0 { req.Limit = 5 }
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			_ = scrape.Run(ctx, req.Team, req.From, req.To, req.Limit)
		}()
		respondJSON(w, http.StatusAccepted, map[string]string{"status":"started"})
	}).Methods(http.MethodPost)

	// GET /players/{id}
	r.HandleFunc("/players/{id}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		idStr := vars["id"]
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil { respondBadRequest(w, err); return }
		row := db.Pool.QueryRow(r.Context(), `SELECT id, player_name, is_wicket_keeper, is_retired, batting_consistency, bowling_consistency FROM player WHERE id = $1`, id)
		var resp struct{
			ID int64 `json:"id"`
			Name string `json:"player_name"`
			IsWicketKeeper int16 `json:"is_wicket_keeper"`
			IsRetired int16 `json:"is_retired"`
			BattingConsistency *float32 `json:"batting_consistency"`
			BowlingConsistency *float32 `json:"bowling_consistency"`
		}
		if err := row.Scan(&resp.ID, &resp.Name, &resp.IsWicketKeeper, &resp.IsRetired, &resp.BattingConsistency, &resp.BowlingConsistency); err != nil { respondErr(w, err); return }
		respondJSON(w, http.StatusOK, resp)
	}).Methods(http.MethodGet)

	// GET /matches/{id}
	r.HandleFunc("/matches/{id}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		idStr := vars["id"]
		mid, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil { respondBadRequest(w, err); return }
		row := db.Pool.QueryRow(r.Context(), `SELECT id, match_id, venue_id, opposition_id, season_id, toss, batting_session, bowling_session FROM match_details WHERE match_id = $1`, mid)
		var resp struct{
			ID int64 `json:"id"`
			MatchID int64 `json:"match_id"`
			VenueID *int64 `json:"venue_id"`
			OppositionID *int64 `json:"opposition_id"`
			SeasonID *int64 `json:"season_id"`
			Toss *string `json:"toss"`
			BattingSession *string `json:"batting_session"`
			BowlingSession *string `json:"bowling_session"`
		}
		if err := row.Scan(&resp.ID, &resp.MatchID, &resp.VenueID, &resp.OppositionID, &resp.SeasonID, &resp.Toss, &resp.BattingSession, &resp.BowlingSession); err != nil { respondErr(w, err); return }
		respondJSON(w, http.StatusOK, resp)
	}).Methods(http.MethodGet)

	// POST /predict/batting
	r.HandleFunc("/predict/batting", func(w http.ResponseWriter, r *http.Request) {
		var feats []contracts.BattingFeatures
		if err := json.NewDecoder(r.Body).Decode(&feats); err != nil { respondBadRequest(w, err); return }
		cli := mlclient.New()
		preds, err := cli.PredictBatting(r.Context(), feats)
		if err != nil { respondErr(w, err); return }
		respondJSON(w, http.StatusOK, preds)
	}).Methods(http.MethodPost)

	// POST /predict/bowling
	r.HandleFunc("/predict/bowling", func(w http.ResponseWriter, r *http.Request) {
		var feats []contracts.BowlingFeatures
		if err := json.NewDecoder(r.Body).Decode(&feats); err != nil { respondBadRequest(w, err); return }
		cli := mlclient.New()
		preds, err := cli.PredictBowling(r.Context(), feats)
		if err != nil { respondErr(w, err); return }
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
