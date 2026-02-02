package server

// Suggestion computation for /ops/status.
// The functions operate on the already-built sections (maps) to avoid tight
// coupling with concrete types and to keep testing simple.

import (
	"fmt"
	"sort"
	"strings"
)

// computeSuggestions inspects the snapshot sections and returns ordered
// suggestions with reasons and commands for the operator to run.
func computeSuggestions(
	db map[string]any,
	precompute map[string]any,
	exports map[string]any,
	artifacts map[string]any,
	services map[string]bool,
	fielding map[string]any,
	weather map[string]any,
) []map[string]any {
	var out []map[string]any

	// 1) DB not migrated or empty
	needsDB := false
	if db != nil {
		connected, _ := db["connected"].(bool)
		if !connected {
			needsDB = true
		} else {
			// any of the counts is zero -> treat as needing import
			if countsAny, ok := db["counts"].(map[string]any); ok {
				zero := func(k string) bool {
					if v, ok := countsAny[k]; ok {
						switch nv := v.(type) {
						case float64:
							return int64(nv) == 0
						case int64:
							return nv == 0
						case int:
							return nv == 0
						}
					}
					return true // absent -> treat as zero
				}
				if zero("players") || zero("matches") {
					needsDB = true
				}
			} else {
				// no counts -> probably needs import
				needsDB = true
			}
		}
	} else {
		needsDB = true
	}
	if needsDB {
		out = append(out, map[string]any{
			"reason":   "Database not ready (disconnected or empty)",
			"commands": []string{"make migrate", "make cricsheet-import"},
		})
		// According to priority rules, if DB is not ready, other steps will be blocked,
		// but we continue to surface additional information for operator context.
	}

	// Helper: list of formats from sections we standardize on
	formats := []string{"TEST", "ODI", "T20I", "T20"}

	// 2) Precompute missing or stale
	var precBad []string
	if precompute != nil {
		if fmAny, ok := precompute["formats"].(map[string]any); ok {
			for _, f := range formats {
				if v, ok := fmAny[f].(map[string]any); ok {
					st, _ := v["status"].(string)
					if st == "missing" || st == "stale" {
						precBad = append(precBad, f)
					}
				}
			}
		}
	}
	if len(precBad) > 0 {
		sort.Strings(precBad)
		out = append(out, map[string]any{
			"reason":   fmt.Sprintf("Precompute missing/stale for %s", strings.Join(precBad, ",")),
			"commands": []string{"make precompute", "# or", "make precompute-asof ASOF=YYYY-MM-DD"},
		})
	}

	// 3) Exports missing for any format
	var missingExports []string
	if exports != nil {
		if fmAny, ok := exports["formats"].(map[string]any); ok {
			for _, f := range formats {
				if v, ok := fmAny[f].(map[string]any); ok {
					// Expect files: []map[string]any where some may have exists=true
					has := false
					if arr, ok := v["files"].([]map[string]any); ok {
						for _, e := range arr {
							if ex, _ := e["exists"].(bool); ex {
								has = true
								break
							}
						}
					} else if arrAny, ok := v["files"].([]any); ok {
						for _, it := range arrAny {
							if m, ok := it.(map[string]any); ok {
								if ex, _ := m["exists"].(bool); ex {
									has = true
									break
								}
							}
						}
					}
					if !has {
						missingExports = append(missingExports, f)
					}
				}
			}
		}
	}
	if len(missingExports) > 0 {
		sort.Strings(missingExports)
		if len(missingExports) == 1 {
			f := missingExports[0]
			out = append(out, map[string]any{
				"reason": fmt.Sprintf("Exports missing for %s", f),
				"commands": []string{
					fmt.Sprintf(
						"cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -format=%s",
						f,
					),
				},
			})
		} else {
			out = append(out, map[string]any{
				"reason":   fmt.Sprintf("Exports missing for %s", strings.Join(missingExports, ",")),
				"commands": []string{fmt.Sprintf("cd go-app && GO_APP_OUTPUT_DIR=../output/go-app go run ./cmd/export-dataset -formats=%s", strings.Join(missingExports, ","))},
			})
		}
	}

	// 4) ML artifacts missing
	var missingArtifacts []string
	if artifacts != nil {
		if fmAny, ok := artifacts["formats"].(map[string]any); ok {
			for _, f := range formats {
				if v, ok := fmAny[f].(map[string]any); ok {
					hasBat, hasBowl := false, false
					if b, ok := v["batting"].(map[string]any); ok {
						hasBat, _ = b["exists"].(bool)
					}
					if b, ok := v["bowling"].(map[string]any); ok {
						hasBowl, _ = b["exists"].(bool)
					}
					if !hasBat || !hasBowl {
						missingArtifacts = append(missingArtifacts, f)
					}
				}
			}
		}
	}
	if len(missingArtifacts) > 0 {
		sort.Strings(missingArtifacts)
		out = append(out, map[string]any{
			"reason":   fmt.Sprintf("ML artifacts missing for %s", strings.Join(missingArtifacts, ",")),
			"commands": []string{"make ml-install", "make train-all"},
		})
	}

	// 5) Services down (currently only ml_health considered here)
	if services != nil {
		if mlOK, ok := services["ml_health"]; ok && !mlOK {
			out = append(out, map[string]any{
				"reason":   "ML service not healthy",
				"commands": []string{"make dev-up", "# if needed", "make dev-rebuild"},
			})
		}
	}

	// 6) Fielding data missing — only suggest if DB looks ready (avoid noise)
	dbReady := false
	if db != nil {
		if connected, _ := db["connected"].(bool); connected {
			// consider ready if counts present and players/matches > 0 (or counts absent -> consider ready to still surface)
			dbReady = true
			if countsAny, ok := db["counts"].(map[string]any); ok {
				gtZero := func(k string) bool {
					if v, ok := countsAny[k]; ok {
						switch nv := v.(type) {
						case float64:
							return int64(nv) > 0
						case int64:
							return nv > 0
						case int:
							return nv > 0
						}
					}
					return false
				}
				dbReady = gtZero("players") && gtZero("matches")
			}
		}
	}

	if dbReady && fielding != nil {
		avail, _ := fielding["available"].(bool)
		rows := int64(0)
		if v, ok := fielding["rows"]; ok {
			switch nv := v.(type) {
			case float64:
				rows = int64(nv)
			case int64:
				rows = nv
			case int:
				rows = int64(nv)
			}
		}
		if !avail || rows == 0 {
			out = append(out, map[string]any{
				"reason": "Fielding data missing",
				"commands": []string{
					"cd go-app && make backfill-fielding",
					"# or",
					"cd go-app && go run ./cmd/backfill-fielding --all",
				},
			})
		}
	}

	// 7) Weather data missing — only suggest if DB looks ready
	if dbReady && weather != nil {
		avail, _ := weather["available"].(bool)
		rows := int64(0)
		if v, ok := weather["rows"]; ok {
			switch nv := v.(type) {
			case float64:
				rows = int64(nv)
			case int64:
				rows = nv
			case int:
				rows = int64(nv)
			}
		}
		if !avail || rows == 0 {
			out = append(out, map[string]any{
				"reason": "Weather data missing",
				"commands": []string{
					"cd go-app && go run ./cmd/weather-import",
					"cd go-app && go run ./cmd/weather-worker",
					"# optional",
					"cd go-app && go run ./cmd/weather-backfill --all",
				},
			})
		}
	}

	return out
}
