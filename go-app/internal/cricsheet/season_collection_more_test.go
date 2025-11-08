package cricsheet_test

import (
	"strings"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

func TestSeason_UnmarshalJSON_InvalidTypesFallback(t *testing.T) {
	var s Season
	// boolean should not match string/int/float -> fallback to empty
	if err := s.UnmarshalJSON([]byte("true")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "" {
		t.Fatalf("expected empty season on invalid type, got %q", string(s))
	}
}

func TestCollection_UnmarshalJSON_InvalidNumberFallback(t *testing.T) {
	var c Collection
	if err := c.UnmarshalJSON([]byte("12345")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil || len(c) != 0 {
		t.Fatalf("expected empty collection, got %#v", []string(c))
	}
}

func TestBothInternational_CaseAndWhitespace(t *testing.T) {
	cfg := &config.Config{}
	cfg.Formats.InternationalTeams = []string{" india ", "australia"}
	teams := []string{" India", "AUSTRALIA "}
	if !bothInternational(teams, cfg) {
		t.Fatalf("expected bothInternational to be true for case/whitespace variants")
	}
	// not enough teams
	if bothInternational([]string{"India"}, cfg) {
		t.Fatalf("expected false when less than two teams provided")
	}
	// nil cfg
	if bothInternational([]string{"India", "Australia"}, nil) {
		t.Fatalf("expected false when cfg is nil")
	}
}

func TestParse_ErrorOnBadJSON(t *testing.T) {
	r := strings.NewReader("not-json")
	_, err := Parse(r)
	if err == nil {
		t.Fatalf("expected error for bad json")
	}
}
