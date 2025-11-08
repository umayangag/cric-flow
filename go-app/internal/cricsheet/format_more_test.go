package cricsheet

import (
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

func TestDetectFormat_EdgeCases(t *testing.T) {
	cfg := &config.Config{}
	cfg.Formats.TreatT20ISubset = false
	cfg.Formats.InternationalTeams = []string{"India", "Australia"}

	tests := []struct {
		name      string
		matchType string
		teams     []string
		cfg       *config.Config
		expect    string
	}{
		{"lowercase test", "test", []string{"India", "Australia"}, cfg, "TEST"},
		{"whitespace odi", "  odi \n", []string{"India", "Australia"}, cfg, "ODI"},
		{"t20 subset disabled", "t20", []string{"India", "Australia"}, cfg, "T20"},
		{"t20 nil cfg", "T20", []string{"India", "Australia"}, nil, "T20"},
		{"t20 less than 2 teams", "T20", []string{"India"}, cfg, "T20"},
		{"unknown empty", "Friendly", []string{"A", "B"}, cfg, ""},
	}
	for _, tc := range tests {
		got := detectFormat(tc.matchType, tc.teams, tc.cfg)
		if got != tc.expect {
			t.Fatalf("%s: expected %q got %q", tc.name, tc.expect, got)
		}
	}
}
