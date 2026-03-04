package server

import (
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
)

// Type aliases — canonical definitions live in services/backtest.
type (
	backtestSelectResponse     = backtest.SelectResponse
	backtestCandidate          = backtest.Candidate
	playerPredictions          = backtest.PlayerPredictions
	playerActuals              = backtest.PlayerActuals
	matchAggregates            = backtest.MatchAggregates
	backtestEvaluateResponse   = backtest.EvaluateResponse
	accuracyTrendResponse      = backtest.AccuracyTrendResponse
	contributionRow            = backtest.ContributionRow
	exportContributionsRequest = backtest.ExportContributionsRequest
)

// BacktestPlayerResult is exported for backward compatibility.
type BacktestPlayerResult = backtest.PlayerResult

// accuracyTrendItem is used only in server for the DTO mapping.
type accuracyTrendItem = backtest.AccuracyTrendItemDTO
