package server

import (
	"context"
)

// buildFieldingSection reports availability and row counts for fielding data per format and overall.
// Uses a single grouped query when available to avoid N queries per format.
func buildFieldingSection(ctx context.Context, probe DBProbe) map[string]any {
	out := map[string]any{"available": false, "formats": map[string]any{}, "overall": map[string]any{"rows": int64(0)}}
	if probe == nil {
		return out
	}
	formats := getCricketFormats()
	fm := map[string]any{}
	var total int64
	counts, err := probe.CountFieldingByFormatGrouped(ctx)
	if err != nil {
		for _, f := range formats {
			fm[f] = map[string]any{"rows": int64(0)}
		}
	} else {
		for _, f := range formats {
			n := counts[f]
			fm[f] = map[string]any{"rows": n}
			total += n
		}
	}
	out["formats"] = fm
	out["overall"] = map[string]any{"rows": total}
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
