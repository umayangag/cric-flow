// Package features: contract.go provides the canonical feature name lists used by export, training-data API, and prediction.
// configs/feature_vectors.json is the single source of truth; when the file is missing, built-in defaults matching that JSON are used.

package features

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// contract holds batting, bowling, and fielding feature name lists (input features only).
// Version is optional in JSON; when present it identifies the schema for compatibility checks.
type contract struct {
	Version  string   `json:"version,omitempty"`
	Batting  []string `json:"batting"`
	Bowling  []string `json:"bowling"`
	Fielding []string `json:"fielding"`
}

var (
	contractMu    sync.Mutex
	contractCache *contract
	contractPath  string
)

// defaultContract matches configs/feature_vectors.json so the app works without the file.
// Version "2" adds raw windowed stat features (batting_*_w*, bowling_*_w*, etc.) alongside formula-derived form/consistency.
var defaultContract = contract{
	Version: "2",
	Batting: []string{
		"batting_consistency", "batting_form", "batting_form_short", "batting_form_long", "batting_momentum",
		"batting_mean_w3", "batting_mean_w5", "batting_mean_w10", "batting_mean_w20",
		"batting_std_w5", "batting_std_w10", "batting_max_w10", "batting_min_w10", "batting_median_w10",
		"batting_last_1", "batting_last_2", "batting_last_3",
		"batting_career_mean", "batting_career_count", "batting_pct_zero_w10", "batting_trend_w5",
		"batting_days_since_last", "batting_innings_in_last_90d",
		"batting_temp", "batting_wind", "batting_rain", "batting_humidity", "batting_cloud", "batting_pressure", "batting_viscosity",
		"batting_inning", "batting_session", "toss", "venue", "opposition", "season", "match_date_unix",
		"bat_prev_sr", "bat_prev_out_rate", "bat_window_sr_12_pp", "bat_window_boundary_rate_12_pp",
		"bat_entry_sr_1_6", "bat_set_sr_13_30", "bat_react_after_dot_sr", "bat_after_k_dots_boundary_p_k2",
	},
	Bowling: []string{
		"bowling_consistency", "bowling_form", "bowling_momentum", "bowling_career_avg",
		"bowling_mean_w3", "bowling_mean_w5", "bowling_mean_w10", "bowling_mean_w20",
		"bowling_std_w5", "bowling_std_w10", "bowling_max_w10", "bowling_min_w10", "bowling_median_w10",
		"bowling_last_1", "bowling_last_2", "bowling_last_3",
		"bowling_career_mean", "bowling_career_count", "bowling_pct_zero_w10", "bowling_trend_w5",
		"bowling_days_since_last", "bowling_innings_in_last_90d",
		"bowling_temp", "bowling_wind", "bowling_rain", "bowling_humidity", "bowling_cloud", "bowling_pressure", "bowling_viscosity",
		"batting_inning", "bowling_session", "toss", "bowling_venue", "bowling_opposition", "season", "match_date_unix",
		"bowl_prev_wkt_rate", "bowl_window_econ_24_death", "bowl_window_wkt_rate_24_death", "bowl_extras_wide_rate_pp",
		"bowl_react_after_boundary_wkt_rate_next", "bowl_spell_first_over_wkt_rate", "bowl_over_ball1_wkt_rate", "bowl_over_ball6_wkt_rate",
	},
	Fielding: []string{
		"fielding_consistency", "fielding_form", "fielding_temp", "fielding_wind", "fielding_rain", "fielding_humidity",
		"fielding_cloud", "fielding_pressure", "fielding_viscosity", "inning", "toss", "fielding_venue", "fielding_opposition", "season_id", "match_date_unix",
	},
}

func loadContract(path string) (*contract, error) {
	if path == "" {
		return &defaultContract, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("features.contract: config not found, using defaults", "path", path)
			return &defaultContract, nil
		}
		return nil, err
	}
	var c contract
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if len(c.Batting) == 0 {
		c.Batting = defaultContract.Batting
	}
	if len(c.Bowling) == 0 {
		c.Bowling = defaultContract.Bowling
	}
	if len(c.Fielding) == 0 {
		c.Fielding = defaultContract.Fielding
	}
	return &c, nil
}

func getContract() *contract {
	path := os.Getenv("FEATURE_VECTORS_PATH")
	if path == "" {
		for _, rel := range []string{"configs/feature_vectors.json", "../configs/feature_vectors.json", "../../configs/feature_vectors.json"} {
			if abs, err := filepath.Abs(rel); err == nil {
				if _, err := os.Stat(abs); err == nil {
					path = abs
					break
				}
			}
		}
	}
	contractMu.Lock()
	defer contractMu.Unlock()
	if contractCache != nil && contractPath == path {
		return contractCache
	}
	c, err := loadContract(path)
	if err != nil {
		slog.Warn("features.contract: load failed, using defaults", "path", path, "err", err)
		c = &defaultContract
	}
	contractCache = c
	contractPath = path
	return c
}

// BattingFeatureNames returns the canonical ordered list of batting input feature names (configs/feature_vectors.json).
func BattingFeatureNames() []string {
	c := getContract()
	out := make([]string, len(c.Batting))
	copy(out, c.Batting)
	return out
}

// BowlingFeatureNames returns the canonical ordered list of bowling input feature names (configs/feature_vectors.json).
func BowlingFeatureNames() []string {
	c := getContract()
	out := make([]string, len(c.Bowling))
	copy(out, c.Bowling)
	return out
}

// FieldingFeatureNames returns the canonical ordered list of fielding input feature names (configs/feature_vectors.json).
func FieldingFeatureNames() []string {
	c := getContract()
	out := make([]string, len(c.Fielding))
	copy(out, c.Fielding)
	return out
}

// ContractVersion returns the version string from the loaded contract, or "" if unversioned.
// Used for compatibility checks and observability.
func ContractVersion() string {
	return getContract().Version
}

// numRawStatsPerCategory is the number of raw windowed stat features per discipline (batting, bowling).
const numRawStatsPerCategory = 18

// battingRawStatOffset is the index in contract.Batting where raw stats start (after formula features).
const battingRawStatOffset = 5

// bowlingRawStatOffset is the index in contract.Bowling where raw stats start (after formula features).
const bowlingRawStatOffset = 4

// RawStatsFeatureNames returns the canonical list of raw windowed stat feature names (v2 contract).
// Derived from the loaded contract (defaultContract or configs/feature_vectors.json) so export,
// training, and prediction stay in sync without duplicating the list.
// Order: batting (18) then bowling (18).
func RawStatsFeatureNames() []string {
	c := getContract()
	batting := c.Batting
	bowling := c.Bowling
	if len(batting) < battingRawStatOffset+numRawStatsPerCategory || len(bowling) < bowlingRawStatOffset+numRawStatsPerCategory {
		// Fallback if contract is truncated; should not happen with defaultContract.
		return rawStatsFeatureNamesFallback()
	}
	out := make([]string, 0, numRawStatsPerCategory*2)
	out = append(out, batting[battingRawStatOffset:battingRawStatOffset+numRawStatsPerCategory]...)
	out = append(out, bowling[bowlingRawStatOffset:bowlingRawStatOffset+numRawStatsPerCategory]...)
	return out
}

// rawStatsFeatureNamesFallback returns the default v2 raw stat names when contract slices are too short.
func rawStatsFeatureNamesFallback() []string {
	return []string{
		"batting_mean_w3", "batting_mean_w5", "batting_mean_w10", "batting_mean_w20",
		"batting_std_w5", "batting_std_w10", "batting_max_w10", "batting_min_w10", "batting_median_w10",
		"batting_last_1", "batting_last_2", "batting_last_3",
		"batting_career_mean", "batting_career_count", "batting_pct_zero_w10", "batting_trend_w5",
		"batting_days_since_last", "batting_innings_in_last_90d",
		"bowling_mean_w3", "bowling_mean_w5", "bowling_mean_w10", "bowling_mean_w20",
		"bowling_std_w5", "bowling_std_w10", "bowling_max_w10", "bowling_min_w10", "bowling_median_w10",
		"bowling_last_1", "bowling_last_2", "bowling_last_3",
		"bowling_career_mean", "bowling_career_count", "bowling_pct_zero_w10", "bowling_trend_w5",
		"bowling_days_since_last", "bowling_innings_in_last_90d",
	}
}

// RawStatsFeatureNamesBatting returns the batting raw stat feature names (first 18 of RawStatsFeatureNames).
// Used by batting export to build CSV headers dynamically so they stay in sync with the contract.
func RawStatsFeatureNamesBatting() []string {
	all := RawStatsFeatureNames()
	return all[:numRawStatsPerCategory]
}

// RawStatsFeatureNamesBowling returns the bowling raw stat feature names (last 18 of RawStatsFeatureNames).
// Used by bowling export to build CSV headers dynamically so they stay in sync with the contract.
func RawStatsFeatureNamesBowling() []string {
	all := RawStatsFeatureNames()
	return all[numRawStatsPerCategory:]
}
