// Command team-select runs the end-to-end team selection pipeline analogous to src/team_selection/select_pool.py
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

func main() {
	var (
		matchID   int64
		format    string
		season    string
		poolPath  string
		teamSize  int
		minBowl   int
		reqKeeper bool
		fromDB    bool
	)
	flag.Int64Var(&matchID, "match", 0, "match_id to build predictions for")
	flag.StringVar(&format, "format", "T20", "match format code (TEST, ODI, T20, T20I)")
	flag.StringVar(&season, "season", "", "season name (e.g. 2019)")
	flag.StringVar(&poolPath, "pool", "../ml-service/ml/pool.csv", "path to prepared pool CSV")
	flag.IntVar(&teamSize, "size", 11, "team size to select")
	flag.IntVar(&minBowl, "min-bowlers", 5, "minimum number of bowlers to include")
	flag.BoolVar(&reqKeeper, "require-keeper", false, "require at least one wicket-keeper")
	flag.BoolVar(&fromDB, "from-db", true, "build features from DB instead of CSV pool")
	flag.Parse()

	if matchID == 0 || season == "" {
		fmt.Fprintln(
			os.Stderr,
			"usage: team-select -match=<id> -season=<name> [-format=CODE] [-pool=path] [-size=N] [-min-bowlers=M] [--require-keeper] [--from-db=true|false]",
		)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Ensure DB connection available when using DB mode
	if fromDB {
		if _, err := db.Connect(ctx); err != nil {
			log.Fatalf("db connect failed: %v", err)
		}
	}

	opts := selection.Options{TeamSize: teamSize, MinBowlers: minBowl, RequireKeeper: reqKeeper}
	var res selection.Result
	var err error
	if fromDB {
		res, err = selection.SelectTeam(ctx, matchID, format, season, opts)
	} else {
		res, err = selection.SelectTeamFromCSV(ctx, poolPath, matchID, format, season, opts)
	}
	if err != nil {
		log.Fatalf("selection failed: %v", err)
	}

	fmt.Printf("Selected Team (size=%d) — Team Win Prob: %.4f\n", len(res.Players), res.TeamWinProbability)
	fmt.Println("-----------------------------------------------------------")
	for i, p := range res.Players {
		fmt.Printf("%2d. %-24s  win=%.4f  bat_pos=%.0f  runs=%.1f  wkts=%.1f\n",
			i+1, p.PlayerName, p.WinningProbability, p.BattingPosition, p.RunsScored, p.WicketsTaken)
	}
}
