package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	// Resolve default input directory from env or config fallback
	defDataDir := os.Getenv("GO_APP_INPUT_DIR")
	if defDataDir == "" {
		defDataDir = config.DefaultCricsheetDir()
	}

	var (
		dataDir = flag.String(
			"dir",
			defDataDir,
			"Directory containing Cricsheet .json files (default from GO_APP_INPUT_DIR or ../data/go-app)",
		)
		phWeather = flag.Bool("placeholders-weather", false, "Insert placeholder weather rows per match")
		phField   = flag.Bool("placeholders-fielding", false, "Insert zeroed fielding rows for all players seen")
		wEnqueue  = flag.Bool("weather-enqueue", true, "Enqueue async weather jobs per match (non-blocking)")
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

	opts := &cricsheet.Options{
		PlaceholdersWeather:  *phWeather,
		PlaceholdersFielding: *phField,
		WeatherEnqueue:       *wEnqueue,
	}
	n, err := cricsheet.ImportDir(ctx, *dataDir, opts)
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}
	log.Printf("cricsheet-importer finished: %d files imported", n)
}
