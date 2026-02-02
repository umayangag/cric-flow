package server

import (
	"context"
)

// buildFieldingSection reports simple availability stats for fielding data.
// It uses DBProbe to remain testable and decoupled from concrete DB layer.
func buildFieldingSection(ctx context.Context, probe DBProbe) map[string]any {
	out := map[string]any{"available": false}
	if probe == nil {
		return out
	}
	// Counts across entire table; can be extended later per-format/season
	if n, err := probe.Count(ctx, "fielding_data"); err == nil {
		out["rows"] = n
		out["available"] = n > 0
	}
	return out
}

// buildWeatherSection reports availability stats for weather data using DBProbe.
func buildWeatherSection(ctx context.Context, probe DBProbe) map[string]any {
	out := map[string]any{"available": false}
	if probe == nil {
		return out
	}
	if n, err := probe.Count(ctx, "weather_data"); err == nil {
		out["rows"] = n
		out["available"] = n > 0
	}
	return out
}
