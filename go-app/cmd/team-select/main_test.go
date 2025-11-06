package main

import (
	"reflect"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

func TestParseFlags_SuccessAndDefaults(t *testing.T) {
	cfg := &config.Config{}

	tests := []struct {
		name string
		args []string
		exp  options
	}{
		{
			name: "defaults applied",
			args: []string{"-match=262039498036", "-season=2025"},
			exp: options{
				matchID:      262039498036,
				formatCode:   "T20",
				seasonName:   "2025",
				poolPath:     "../ml-service/ml/pool.csv",
				teamSize:     11,
				minBowlers:   5,
				requireKeeper: false,
				fromDB:       true,
			},
		},
		{
			name: "overrides respected",
			args: []string{
				"-match=1", "-season=2019", "-format=ODI", "-pool=/tmp/pool.csv",
				"-size=9", "-min-bowlers=4", "-require-keeper", "-from-db=false",
			},
			exp: options{
				matchID:      1,
				formatCode:   "ODI",
				seasonName:   "2019",
				poolPath:     "/tmp/pool.csv",
				teamSize:     9,
				minBowlers:   4,
				requireKeeper: true,
				fromDB:       false,
			},
		},
	}

	for _, tc := range tests {
		c := tc
		t.Run(c.name, func(t *testing.T) {
			got, err := parseFlags(c.args, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.exp) {
				t.Fatalf("options mismatch\n got: %#v\nwant: %#v", got, c.exp)
			}
		})
	}
}

func TestParseFlags_Errors(t *testing.T) {
	cfg := &config.Config{}
	bad := [][]string{
		{},
		{"-season=2025"},
		{"-match=1"},
	}
	for _, args := range bad {
		if _, err := parseFlags(args, cfg); err == nil {
			t.Fatalf("expected error for args: %v", args)
		}
	}
}
