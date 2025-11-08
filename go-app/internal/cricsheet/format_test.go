package cricsheet_test

import (
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
)

// NOTE: This file was consolidated to follow table-driven, black-box tests.
// All DetectFormat cases (including edge cases previously in format_more_test.go)
// are merged into a single table-driven test below.

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

func TestDetectFormat_BasicMappings(t *testing.T) {
	cfg := testConfig()
	if got := cricsheet.DetectFormat("Test", []string{"India", "Australia"}, cfg); got != "TEST" {
		t.Fatalf("expected TEST, got %s", got)
	}
	if got := cricsheet.DetectFormat("ODI", []string{"India", "Australia"}, cfg); got != "ODI" {
		t.Fatalf("expected ODI, got %s", got)
	}
	if got := cricsheet.DetectFormat("T20I", []string{"India", "Australia"}, cfg); got != "T20I" {
		t.Fatalf("expected T20I, got %s", got)
	}
}

func TestDetectFormat_T20SubsetRule(t *testing.T) {
	cfg := testConfig()
	// International vs International in T20 -> T20I
	if got := cricsheet.DetectFormat("T20", []string{"India", "Australia"}, cfg); got != "T20I" {
		t.Fatalf("expected T20I (subset rule), got %s", got)
	}
	// Domestic/Franchise teams in T20 -> T20
	if got := cricsheet.DetectFormat("T20", []string{"Mumbai Indians", "Chennai Super Kings"}, cfg); got != "T20" {
		t.Fatalf("expected T20, got %s", got)
	}
	// Mixed international + domestic -> T20
	if got := cricsheet.DetectFormat("T20", []string{"India", "Mumbai Indians"}, cfg); got != "T20" {
		t.Fatalf("expected T20 when mixed, got %s", got)
	}
}

func TestDetectFormat_Unknown(t *testing.T) {
	cfg := testConfig()
	if got := cricsheet.DetectFormat("Friendly", []string{"Team A", "Team B"}, cfg); got != "" {
		t.Fatalf("expected empty for unknown type, got %s", got)
	}
}

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
		// subset enabled + case/whitespace + international teams -> T20I
	}
	// add a case with subset enabled and case/whitespace varieties
	cfg2 := &config.Config{}
	cfg2.Formats.TreatT20ISubset = true
	cfg2.Formats.InternationalTeams = []string{" india ", "australia"}
	tests = append(tests, struct {
		name      string
		matchType string
		teams     []string
		cfg       *config.Config
		expect    string
	}{
		name:      "t20 subset enabled with case/whitespace",
		matchType: "T20",
		teams:     []string{" India", "AUSTRALIA "},
		cfg:       cfg2,
		expect:    "T20I",
	})

	for _, tc := range tests {
		got := cricsheet.DetectFormat(tc.matchType, tc.teams, tc.cfg)
		if got != tc.expect {
			t.Fatalf("%s: expected %q got %q", tc.name, tc.expect, got)
		}
	}
}
