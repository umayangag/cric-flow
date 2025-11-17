package predictor

import (
	"reflect"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

func TestParseFlags_SuccessAndDefaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.Team.DefaultBatters = 6
	cfg.Team.DefaultBowlers = 5
	cfg.Team.MinBowlers = 5

	tests := []struct {
		name string
		args []string
		exp  options
	}{
		{
			name: "explicit bat/bowl kept",
			args: []string{"-match=100", "-format=T20", "-season=2025", "-bat=7", "-bowl=6"},
			exp:  options{matchID: 100, formatCode: "T20", seasonName: "2025", batters: 7, bowlers: 6},
		},
		{
			name: "defaults applied when zero",
			args: []string{"-match=1", "-format=ODI", "-season=2019"},
			exp:  options{matchID: 1, formatCode: "ODI", seasonName: "2019", batters: 6, bowlers: 5},
		},
		{
			name: "min bowlers enforced",
			args: []string{"-match=2", "-format=T20", "-season=2024", "-bowl=3"},
			exp:  options{matchID: 2, formatCode: "T20", seasonName: "2024", batters: 6, bowlers: 5},
		},
	}

	for _, tc := range tests {
		// capture range var
		c := tc
		t.Run(c.name, func(t *testing.T) {
			got, err := parseFlags(c.args, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.exp) {
				t.Fatalf("options mismatch:\n got: %#v\nwant: %#v", got, c.exp)
			}
		})
	}
}

func TestParseFlags_Errors(t *testing.T) {
	cfg := &config.Config{}
	badCases := [][]string{
		{},
		{"-match=0", "-format=T20", "-season=2025"},
		{"-match=10", "-format=", "-season=2025"},
		{"-match=10", "-format=T20", "-season="},
	}
	for _, args := range badCases {
		if _, err := parseFlags(args, cfg); err == nil {
			t.Fatalf("expected error for args: %v", args)
		}
	}
}
