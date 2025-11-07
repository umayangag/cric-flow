package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/eval"
)

func main() {
	// Scaffold: evaluate tiny fixed arrays just to prove the wiring.
	format := flag.String("format", "T20", "Match format (e.g., T20, ODI)")
	season := flag.String("season", "demo", "Season identifier (used for reporting only in scaffold)")
	flag.Parse()

	yTrue := []float64{30, 45, 10, 60}
	yPred := []float64{28, 40, 12, 55}

	mae := eval.MAE(yTrue, yPred)
	rmse := eval.RMSE(yTrue, yPred)

	// Team-level example for Brier score (probability of win)
	yWin := []float64{1, 0, 1, 1}
	yProb := []float64{0.7, 0.4, 0.65, 0.8}
	brier := eval.BrierScore(yWin, yProb)

	log.Printf("Evaluation (scaffold) — season=%s format=%s", *season, *format)
	fmt.Printf("MAE=%.4f RMSE=%.4f Brier=%.4f\n", mae, rmse, brier)
}
