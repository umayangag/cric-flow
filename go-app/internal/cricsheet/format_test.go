package cricsheet_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

// NOTE: This file follows the gold-standard for tests: external package,
// table-driven subtests, Arrange → Act → Assert, and fail-fast require assertions.

// SPDX-License-Identifier: MIT
// Package-level tests consolidated: prefer table-driven, black-box tests.

func testConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Formats.TreatT20ISubset = true
	cfg.Formats.InternationalTeams = []string{
		"Afghanistan", "Australia", "Bangladesh", "England", "India", "Ireland",
		"New Zealand", "Pakistan", "South Africa", "Sri Lanka", "West Indies", "Zimbabwe",
	}
	return cfg
}

func TestDetectFormat_Table(t *testing.T) {
	t.Parallel()

	base := testConfig()
	disabled := &config.Config{}
	disabled.Formats.TreatT20ISubset = false
	disabled.Formats.InternationalTeams = []string{"India", "Australia"}

	enabledCase := &config.Config{}
	enabledCase.Formats.TreatT20ISubset = true
	enabledCase.Formats.InternationalTeams = []string{" india ", "australia"}

	type testCase struct {
		name      string
		matchType string
		teams     []string
		cfg       *config.Config
		want      string
	}

	cases := []testCase{
		// Basic mappings
		{name: "TEST basic", matchType: "Test", teams: []string{"India", "Australia"}, cfg: base, want: "TEST"},
		{name: "ODI basic", matchType: "ODI", teams: []string{"India", "Australia"}, cfg: base, want: "ODI"},
		{name: "T20I basic", matchType: "T20I", teams: []string{"India", "Australia"}, cfg: base, want: "T20I"},
		// Aliases
		{name: "IT20 alias", matchType: "IT20", teams: []string{"India", "Australia"}, cfg: base, want: "T20I"},
		{name: "ODM alias", matchType: "ODM", teams: []string{"India", "Australia"}, cfg: base, want: "ODI"},
		{name: "MDM alias", matchType: "MDM", teams: []string{"India", "Australia"}, cfg: base, want: "TEST"},
		// T20 subset rule
		{
			name:      "T20 subset -> T20I (intl vs intl)",
			matchType: "T20",
			teams:     []string{"India", "Australia"},
			cfg:       base,
			want:      "T20I",
		},
		{
			name:      "T20 domestic -> T20",
			matchType: "T20",
			teams:     []string{"Mumbai Indians", "Chennai Super Kings"},
			cfg:       base,
			want:      "T20",
		},
		{
			name:      "T20 mixed intl+domestic -> T20",
			matchType: "T20",
			teams:     []string{"India", "Mumbai Indians"},
			cfg:       base,
			want:      "T20",
		},
		// Unknown
		{name: "Unknown -> empty", matchType: "Friendly", teams: []string{"Team A", "Team B"}, cfg: base, want: ""},
		// Edge cases
		{name: "lowercase test", matchType: "test", teams: []string{"India", "Australia"}, cfg: disabled, want: "TEST"},
		{
			name:      "whitespace odi",
			matchType: "  odi \n",
			teams:     []string{"India", "Australia"},
			cfg:       disabled,
			want:      "ODI",
		},
		{
			name:      "t20 subset disabled",
			matchType: "t20",
			teams:     []string{"India", "Australia"},
			cfg:       disabled,
			want:      "T20",
		},
		{name: "t20 nil cfg", matchType: "T20", teams: []string{"India", "Australia"}, cfg: nil, want: "T20"},
		{name: "t20 less than 2 teams", matchType: "T20", teams: []string{"India"}, cfg: disabled, want: "T20"},
		{
			name:      "t20 subset enabled with case/whitespace",
			matchType: "T20",
			teams:     []string{" India", "AUSTRALIA "},
			cfg:       enabledCase,
			want:      "T20I",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange -> Act
			got := cricsheet.DetectFormat(tc.matchType, tc.teams, tc.cfg)
			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}
