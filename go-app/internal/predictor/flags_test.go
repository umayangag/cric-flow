package predictor

import (
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

func TestParseFlags_BlankFormatOrSeasonErrors(t *testing.T) {
	cfg := &config.Config{}
	bad := [][]string{
		{"-match=10", "-format= ", "-season=2025"},
		{"-match=10", "-format=T20", "-season=   "},
	}
	for _, args := range bad {
		if _, err := parseFlags(args, cfg); err == nil {
			t.Fatalf("expected error for args: %v", args)
		}
	}
}
