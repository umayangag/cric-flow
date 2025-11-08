package cricsheet_test

import (
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

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
	if got := DetectFormat("Test", []string{"India", "Australia"}, cfg); got != "TEST" {
		t.Fatalf("expected TEST, got %s", got)
	}
	if got := DetectFormat("ODI", []string{"India", "Australia"}, cfg); got != "ODI" {
		t.Fatalf("expected ODI, got %s", got)
	}
	if got := DetectFormat("T20I", []string{"India", "Australia"}, cfg); got != "T20I" {
		t.Fatalf("expected T20I, got %s", got)
	}
}

func TestDetectFormat_T20SubsetRule(t *testing.T) {
	cfg := testConfig()
	// International vs International in T20 -> T20I
	if got := DetectFormat("T20", []string{"India", "Australia"}, cfg); got != "T20I" {
		t.Fatalf("expected T20I (subset rule), got %s", got)
	}
	// Domestic/Franchise teams in T20 -> T20
	if got := DetectFormat("T20", []string{"Mumbai Indians", "Chennai Super Kings"}, cfg); got != "T20" {
		t.Fatalf("expected T20, got %s", got)
	}
	// Mixed international + domestic -> T20
	if got := DetectFormat("T20", []string{"India", "Mumbai Indians"}, cfg); got != "T20" {
		t.Fatalf("expected T20 when mixed, got %s", got)
	}
}

func TestDetectFormat_Unknown(t *testing.T) {
	cfg := testConfig()
	if got := DetectFormat("Friendly", []string{"Team A", "Team B"}, cfg); got != "" {
		t.Fatalf("expected empty for unknown type, got %s", got)
	}
}
