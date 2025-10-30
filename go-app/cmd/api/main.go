package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/umayangag/cric-app/go-app/internal/db"
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}).Methods(http.MethodGet)

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	log.Printf("API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
