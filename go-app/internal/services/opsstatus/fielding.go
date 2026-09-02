package opsstatus

import (
	"context"
)

// BuildFieldingSection reports availability and row counts for fielding data per format and overall.
func BuildFieldingSection(ctx context.Context, probe DBProbe) map[string]any {
	out := map[string]any{"available": false, "formats": map[string]any{}, "overall": map[string]any{"rows": int64(0)}}
	if probe == nil {
		return out
	}
	formats := CricketFormatCodes
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
