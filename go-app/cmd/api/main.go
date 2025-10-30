package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-app/go-app/internal/contracts"
	"github.com/umayangag/cric-app/go-app/internal/db"
	"github.com/umayangag/cric-app/go-app/internal/features"
	"github.com/umayangag/cric-app/go-app/internal/mlclient"
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
