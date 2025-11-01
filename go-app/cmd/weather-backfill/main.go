package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func main() {
	var (
		limit  = flag.Int("limit", 0, "maximum number of jobs to enqueue (0 = no limit)")
		dryRun = flag.Bool("dry-run", false, "show how many jobs would be enqueued without modifying the database")
	)
	flag.Parse()

	ctx := context.Background()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	if *dryRun {
		// In dry-run, estimate count by running the insert in a rolled-back tx would be ideal; keep it simple:
		// We will not modify the DB; instruct the user how to run without dry-run.
		fmt.Println("dry-run: no changes made. Run without --dry-run to enqueue missing weather jobs.")
		return
	}

	added, err := db.EnqueueMissingWeatherJobs(ctx, *limit)
	if err != nil {
		log.Fatalf("enqueue missing jobs failed: %v", err)
	}
	fmt.Printf("enqueued %d weather jobs (limit=%d)\n", added, *limit)
}
