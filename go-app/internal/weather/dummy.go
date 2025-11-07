package weather

import (
	"context"
	"time"
)

// DummyProvider returns a tiny deterministic forecast suitable for tests/dev.
type DummyProvider struct{}

func (d DummyProvider) Name() string { return "dummy" }

func (d DummyProvider) GetForecast(ctx context.Context, matchID int64) ([]Forecast, error) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	return []Forecast{
		{MatchID: matchID, Innings: 1, Timestamp: base, TemperatureC: 28, HumidityPct: 60, WindKph: 12, PrecipMM: 0.2},
		{MatchID: matchID, Innings: 2, Timestamp: base.Add(3 * time.Hour), TemperatureC: 26, HumidityPct: 65, WindKph: 10, PrecipMM: 0.1},
	}, nil
}
