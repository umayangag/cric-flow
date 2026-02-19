package server

import (
	"context"
)

// buildFieldingSection reports availability and row counts for fielding data per format and overall.
func buildFieldingSection(ctx context.Context, probe DBProbe) map[string]any {
	out := map[string]any{"available": false, "formats": map[string]any{}, "overall": map[string]any{"rows": int64(0)}}
	if probe == nil {
		return out
	}
	formats := getCricketFormats()
	fm := map[string]any{}
	var total int64
	for _, f := range formats {
		n, err := probe.CountFieldingByFormat(ctx, f)
		if err != nil {
			fm[f] = map[string]any{"rows": int64(0)}
			continue
		}
		fm[f] = map[string]any{"rows": n}
		total += n
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
