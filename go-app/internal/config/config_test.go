package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTeamSettings(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		err  string
	}{
		{
			name: "valid settings",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Team.DefaultBatters = 6
				c.Team.DefaultBowlers = 5
				c.Predictor.TeamSize = 11
				c.Predictor.DefaultExtras = 4
				return c
			}(),
			err: "",
		},
		{
			name: "min bowlers < 1",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 0
				c.Predictor.TeamSize = 11
				return c
			}(),
			err: "min bowlers",
		},
		{
			name: "team size < min bowlers",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 6
				c.Predictor.TeamSize = 5
				return c
			}(),
			err: "team size",
		},
		{
			name: "negative extras",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Predictor.TeamSize = 11
				c.Predictor.DefaultExtras = -1
				return c
			}(),
			err: "extras",
		},
		{
			name: "default bowlers < min bowlers",
			cfg: func() *Config {
				c := &Config{}
				c.Team.MinBowlers = 5
				c.Team.DefaultBowlers = 3
				c.Predictor.TeamSize = 11
				return c
			}(),
			err: "default bowlers",
		},
	}

	for _, tc := range tests {
		// capture
		c := tc
		t.Run(c.name, func(t *testing.T) {
			err := ValidateTeamSettings(c.cfg)
			if c.err == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.err)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(c.err)) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.err)
			}
		})
	}
}

func TestDefaultDirs_UseConfigValues(t *testing.T) {
	// ensure cached does not leak across tests
	cached = &Config{}
	cached.Inputs.CricsheetDir = "/tmp/cricsheet"
	cached.Inputs.EtlDir = "/tmp/etl"
	cached.Outputs.ExportDir = "/tmp/export"

	if got := DefaultCricsheetDir(); got != "/tmp/cricsheet" {
		t.Fatalf("DefaultCricsheetDir got %q want %q", got, "/tmp/cricsheet")
	}
	if got := DefaultEtlDir(); got != "/tmp/etl" {
		t.Fatalf("DefaultEtlDir got %q want %q", got, "/tmp/etl")
	}
	if got := DefaultExportDir(); got != "/tmp/export" {
		t.Fatalf("DefaultExportDir got %q want %q", got, "/tmp/export")
	}
}

func TestDefaultDirs_FallbacksWhenUnset(t *testing.T) {
	cached = &Config{} // simulate empty config loaded
	wantCricsheet := filepath.Join("..", "data", "go-app", "cricsheet")
	wantEtl := filepath.Join("..", "data", "go-app", "createdb")
	wantExport := filepath.Join("..", "output", "go-app")

	if got := DefaultCricsheetDir(); got != wantCricsheet {
		t.Fatalf("DefaultCricsheetDir fallback got %q want %q", got, wantCricsheet)
	}
	if got := DefaultEtlDir(); got != wantEtl {
		t.Fatalf("DefaultEtlDir fallback got %q want %q", got, wantEtl)
	}
	if got := DefaultExportDir(); got != wantExport {
		t.Fatalf("DefaultExportDir fallback got %q want %q", got, wantExport)
	}
}
