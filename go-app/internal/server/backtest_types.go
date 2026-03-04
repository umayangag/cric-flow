package server

import (
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
)

// Type aliases — canonical definitions live in services/backtest.
type backtestSelectResponse = backtest.SelectResponse
type backtestCandidate = backtest.Candidate
type playerPredictions = backtest.PlayerPredictions
type playerActuals = backtest.PlayerActuals
type matchAggregates = backtest.MatchAggregates
type backtestEvaluateResponse = backtest.EvaluateResponse
type accuracyTrendResponse = backtest.AccuracyTrendResponse
type contributionRow = backtest.ContributionRow
type exportContributionsRequest = backtest.ExportContributionsRequest

// BacktestPlayerResult is exported for backward compatibility.
type BacktestPlayerResult = backtest.PlayerResult

// accuracyTrendItem is used only in server for the DTO mapping.
type accuracyTrendItem = backtest.AccuracyTrendItemDTO
