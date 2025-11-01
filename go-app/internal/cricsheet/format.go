package cricsheet

import (
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// DetectFormat returns one of standardized codes: TEST, ODI, T20, T20I
// Rules:
// - matchType "Test" -> TEST
// - matchType "ODI"  -> ODI
// - matchType "T20I" -> T20I
// - matchType "T20"  -> if both teams are international (per config) and TreatT20ISubset is true, classify as T20I; else T20
// If matchType is unrecognized, returns empty string.
func DetectFormat(matchType string, teams []string, cfg *config.Config) string {
	mt := strings.ToUpper(strings.TrimSpace(matchType))
	switch mt {
	case "TEST":
		return "TEST"
	case "ODI":
		return "ODI"
	case "T20I":
		return "T20I"
	case "T20":
		if cfg != nil && cfg.Formats.TreatT20ISubset && bothInternational(teams, cfg) {
			return "T20I"
		}
		return "T20"
	default:
		return ""
	}
}

func bothInternational(teams []string, cfg *config.Config) bool {
	if cfg == nil || len(cfg.Formats.InternationalTeams) == 0 || len(teams) < 2 {
		return false
	}
	set := map[string]struct{}{}
	for _, t := range cfg.Formats.InternationalTeams {
		set[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}
	cnt := 0
	for _, tm := range teams {
		if _, ok := set[strings.ToLower(strings.TrimSpace(tm))]; ok {
			cnt++
		}
	}
	return cnt >= 2
}
