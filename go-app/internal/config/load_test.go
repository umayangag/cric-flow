package config

import (
	"os"
	"path/filepath"
	"testing"
)

// helper to write a small config.json file
func writeConfigFile(t *testing.T, dir string, content string) string {
	t.Helper()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestLoad_EnvPathPrecedence(t *testing.T) {
	// ensure cache clean
	cached = nil

	tmp := t.TempDir()
	cfgJSON := `{"inputs":{"cricsheet_dir":"/env/cricsheet"},"outputs":{"export_dir":"/env/export"}}`
	p := filepath.Join(tmp, "custom.json")
	if err := os.WriteFile(p, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("write env cfg: %v", err)
	}
	t.Setenv("GO_APP_CONFIG", p)

	c := Load()
	if c.Inputs.CricsheetDir != "/env/cricsheet" {
		t.Fatalf("env precedence failed: got %q", c.Inputs.CricsheetDir)
	}
	if c.Outputs.ExportDir != "/env/export" {
		t.Fatalf("env precedence failed: got %q", c.Outputs.ExportDir)
	}
}

func TestLoad_ConfigJsonFromCWD(t *testing.T) {
	// Reset cache and change working dir to a temp dir containing config.json.
	cached = nil
	oldWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	cfgJSON := `{"inputs":{"cricsheet_dir":"/cwd/cricsheet"},"outputs":{"export_dir":"/cwd/export"}}`
	_ = writeConfigFile(t, tmp, cfgJSON)
	// Ensure env var not set so file discovery is used
	t.Setenv("GO_APP_CONFIG", "")

	c := Load()
	if c.Inputs.CricsheetDir != "/cwd/cricsheet" {
		t.Fatalf("cwd config not applied: %q", c.Inputs.CricsheetDir)
	}
	if c.Outputs.ExportDir != "/cwd/export" {
		t.Fatalf("cwd config not applied: %q", c.Outputs.ExportDir)
	}
}

func TestLoad_CachePersistsUntilReset(t *testing.T) {
	cached = nil
	// Use env file to set one value, then mutate the file and ensure Load() keeps cached result until we reset.
	tmp := t.TempDir()
	p := filepath.Join(tmp, "a.json")
	first := `{"inputs":{"cricsheet_dir":"/first"}}`
	second := `{"inputs":{"cricsheet_dir":"/second"}}`
	if err := os.WriteFile(p, []byte(first), 0o600); err != nil {
		t.Fatalf("write first: %v", err)
	}
	t.Setenv("GO_APP_CONFIG", p)
	c1 := Load()
	if c1.Inputs.CricsheetDir != "/first" {
		t.Fatalf("expected /first, got %q", c1.Inputs.CricsheetDir)
	}
	// mutate file
	if err := os.WriteFile(p, []byte(second), 0o600); err != nil {
		t.Fatalf("write second: %v", err)
	}
	c2 := Load()
	if c2.Inputs.CricsheetDir != "/first" {
		t.Fatalf("cache not used: got %q", c2.Inputs.CricsheetDir)
	}
	// reset cache and verify change is picked up
	cached = nil
	c3 := Load()
	if c3.Inputs.CricsheetDir != "/second" {
		t.Fatalf("expected /second after reset, got %q", c3.Inputs.CricsheetDir)
	}
}
