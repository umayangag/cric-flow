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
type contract struct {
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
var defaultContract = contract{
	Batting: []string{
		"batting_consistency", "batting_form", "batting_form_short", "batting_form_long", "batting_momentum",
		"batting_temp", "batting_wind", "batting_rain", "batting_humidity", "batting_cloud", "batting_pressure", "batting_viscosity",
		"batting_inning", "batting_session", "toss", "venue", "opposition", "season",
		"bat_prev_sr", "bat_prev_out_rate", "bat_window_sr_12_pp", "bat_window_boundary_rate_12_pp",
		"bat_entry_sr_1_6", "bat_set_sr_13_30", "bat_react_after_dot_sr", "bat_after_k_dots_boundary_p_k2",
	},
	Bowling: []string{
		"bowling_consistency", "bowling_form", "bowling_form_short", "bowling_form_long", "bowling_momentum", "bowling_career_avg",
		"bowling_temp", "bowling_wind", "bowling_rain", "bowling_humidity", "bowling_cloud", "bowling_pressure", "bowling_viscosity",
		"batting_inning", "bowling_session", "toss", "bowling_venue", "bowling_opposition", "season",
		"bowl_prev_wkt_rate", "bowl_window_econ_24_death", "bowl_window_wkt_rate_24_death", "bowl_extras_wide_rate_pp",
		"bowl_react_after_boundary_wkt_rate_next", "bowl_spell_first_over_wkt_rate", "bowl_over_ball1_wkt_rate", "bowl_over_ball6_wkt_rate",
	},
	Fielding: []string{
		"fielding_consistency", "fielding_form", "fielding_temp", "fielding_wind", "fielding_rain", "fielding_humidity",
		"fielding_cloud", "fielding_pressure", "fielding_viscosity", "inning", "toss", "fielding_venue", "fielding_opposition", "season_id",
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
