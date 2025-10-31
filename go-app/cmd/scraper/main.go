//go:build legacy_cricinfo

package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/umayangag/cric-app/go-app/internal/db"
	"github.com/umayangag/cric-app/go-app/internal/scrape"
)

var (
	team   = flag.String("team", "Sri Lanka", "Team name (as shown on Cricinfo)")
	from   = flag.String("from", "2010-01-01", "From date (YYYY-MM-DD)")
	toDate = flag.String("to", "2020-01-01", "To date (YYYY-MM-DD)")
)

func main() {
	flag.Parse()
	log.Printf("scraper starting team=%s from=%s to=%s", *team, *from, *toDate)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	// Reuse internal scraper with a small default limit of 5
	if err := scrape.Run(ctx, *team, *from, *toDate, 5); err != nil {
		log.Fatalf("scrape run failed: %v", err)
	}
	log.Println("scraper finished")
}
