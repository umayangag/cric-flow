package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	clieval "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/evaluate"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/evaluate"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/evaluate"
)

// demoRepo preserves existing scaffold behavior by returning fixed arrays.
type demoRepo struct{}

func (demoRepo) LoadInputs(ctx context.Context, season, format string) (svc.Inputs, error) {
	return svc.Inputs{
		YTrue: []float64{30, 45, 10, 60},
		YPred: []float64{28, 40, 12, 55},
		YWin:  []float64{1, 0, 1, 1},
		YProb: []float64{0.7, 0.4, 0.65, 0.8},
	}, nil
}

func main() {
	// Thin delegator: parse, wire, run.
	fs := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	opts, err := clieval.ParseArgs(fs, os.Args[1:])
	if err != nil { panic(err) }

	logger.SetupFromEnv()
	_ = slog.Default() // ensure slog imported

	r := cmd.Runner{Repo: demoRepo{}, Out: os.Stdout}
	if err := r.Run(context.Background(), opts); err != nil { panic(err) }
}
