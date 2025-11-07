package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/weather"
)

// simpleUpserter is a placeholder that prints what would be upserted.
type simpleUpserter struct{}

func (simpleUpserter) UpsertForecasts(_ context.Context, f []weather.Forecast) error {
	for _, x := range f {
		log.Printf(
			"[dry-run] upsert match=%d innings=%d at=%s T=%.1fC H=%.0f%% W=%.1fkph P=%.1fmm",
			x.MatchID,
			x.Innings,
			x.Timestamp.Format("2006-01-02T15:04Z"),
			x.TemperatureC,
			x.HumidityPct,
			x.WindKph,
			x.PrecipMM,
		)
	}
	return nil
}

func main() {
	var (
		matchStr = flag.String("match", "0", "Match ID (required)")
		provider = flag.String("provider", "dummy", "Weather provider (dummy)")
		apply    = flag.Bool("apply", false, "Apply changes (no-op for now; prints only)")
	)
	flag.Parse()

	matchID, err := strconv.ParseInt(*matchStr, 10, 64)
	if err != nil || matchID <= 0 {
		fmt.Fprintln(os.Stderr, "invalid or missing -match=<id>")
		os.Exit(2)
	}

	ctx := context.Background()

	var p weather.Provider
	switch *provider {
	case "dummy":
		p = weather.DummyProvider{}
	default:
		fmt.Fprintf(os.Stderr, "unknown provider: %s\n", *provider)
		os.Exit(2)
	}

	u := simpleUpserter{}
	if err := weather.Ingest(ctx, p, u, matchID); err != nil {
		log.Fatalf("ingest failed: %v", err)
	}

	if *apply {
		log.Printf("apply requested, but this scaffold only prints for now")
	}
}
