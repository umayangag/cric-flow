package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	var (
		dataDir   = flag.String("dir", "../data", "Directory containing Cricsheet .json files")
		phWeather = flag.Bool("placeholders-weather", false, "Insert placeholder weather rows per match")
		phField   = flag.Bool("placeholders-fielding", false, "Insert zeroed fielding rows for all players seen")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	// Apply migrations to ensure schema is ready
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	opts := &cricsheet.Options{PlaceholdersWeather: *phWeather, PlaceholdersFielding: *phField}
	n, err := cricsheet.ImportDir(ctx, *dataDir, opts)
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}
	log.Printf("cricsheet-importer finished: %d files imported", n)
}
