package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	var dir string
	flag.StringVar(&dir, "dir", "migrations", "directory with .sql migration files")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	if err := db.RunMigrations(ctx, dir); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	log.Printf("migrations applied successfully from %s", dir)
}
