package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestValidateForServer_FailsWhenNoConfig(t *testing.T) {
	cached = nil
	oldWD, _ := os.Getwd()
	oldEnv := os.Getenv("GO_APP_CONFIG")
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
		_ = os.Setenv("GO_APP_CONFIG", oldEnv)
	})

	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	_ = os.Setenv("GO_APP_CONFIG", "")

	Load() // no config file in tmp
	err := ValidateForServer()
	if err == nil {
		t.Fatal("ValidateForServer expected to fail when no config file")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("expected 'config file not found', got: %v", err)
	}
}

func TestValidateForServer_FailsWhenPrecomputeTimeoutNegative(t *testing.T) {
	cached = nil
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	cfgJSON := `{"features":{"precompute_timeout_ms":-1}}`
	if err := os.WriteFile(p, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("GO_APP_CONFIG", p)

	Load()
	err := ValidateForServer()
	if err == nil {
		t.Fatal("ValidateForServer expected to fail when precompute_timeout_ms < 0")
	}
	if !strings.Contains(err.Error(), "precompute_timeout_ms") {
		t.Errorf("expected 'precompute_timeout_ms' in error, got: %v", err)
	}
}

func TestValidateForServer_FailsWhenExportTimeoutNegative(t *testing.T) {
	cached = nil
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	cfgJSON := `{"features":{"export_timeout_ms":-1}}`
	if err := os.WriteFile(p, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("GO_APP_CONFIG", p)

	Load()
	err := ValidateForServer()
	if err == nil {
		t.Fatal("ValidateForServer expected to fail when export_timeout_ms < 0")
	}
	if !strings.Contains(err.Error(), "export_timeout_ms") {
		t.Errorf("expected 'export_timeout_ms' in error, got: %v", err)
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

func TestLoadMetaModel_NilConfigReturnsNil(t *testing.T) {
	got := loadMetaModel(nil)
	if got != nil {
		t.Fatalf("expected nil for nil config, got %+v", got)
	}
}

func TestLoadMetaModel_EmptyPathReturnsNil(t *testing.T) {
	cfg := &Config{}
	got := loadMetaModel(cfg)
	if got != nil {
		t.Fatalf("expected nil for empty MetaModelPath, got %+v", got)
	}
}

func TestLoadMetaModel_FileNotFoundReturnsNil(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	cfg := &Config{}
	cfg.Selection.MetaModelPath = "/nonexistent/path/meta.json"
	got := loadMetaModel(cfg)
	if got != nil {
		t.Fatalf("expected nil for missing file, got %+v", got)
	}
}

func TestLoadMetaModel_InvalidJSONReturnsNil(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "bad.json")
	if err := os.WriteFile(bad, []byte(`{not json}`), 0o600); err != nil {
		t.Fatalf("write bad json: %v", err)
	}
	cfg := &Config{}
	cfg.Selection.MetaModelPath = bad
	got := loadMetaModel(cfg)
	if got != nil {
		t.Fatalf("expected nil for invalid JSON, got %+v", got)
	}
}

func TestLoadMetaModel_RelativePathResolvedAgainstConfigDir(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	cached = nil

	tmp := t.TempDir()
	metaContent := `{"bat":0.4,"bowl":0.3,"field":0.2,"keeper_bonus":0.1}`
	if err := os.WriteFile(filepath.Join(tmp, "meta.json"), []byte(metaContent), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	cfgPath := filepath.Join(tmp, "config.json")
	cfgJSON := `{"selection":{"meta_model_path":"meta.json"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("GO_APP_CONFIG", cfgPath)
	cfg := Load()

	got := loadMetaModel(cfg)
	if got == nil {
		t.Fatal("expected non-nil meta model")
	}
	if got.Bat != 0.4 {
		t.Fatalf("expected bat=0.4, got %f", got.Bat)
	}
}

func TestLoadMetaModel_CacheHitReturnsSamePointer(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	cached = nil

	tmp := t.TempDir()
	metaContent := `{"bat":0.5,"bowl":0.3,"field":0.15,"keeper_bonus":0.05}`
	metaFile := filepath.Join(tmp, "meta_cache.json")
	if err := os.WriteFile(metaFile, []byte(metaContent), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	cfg := &Config{}
	cfg.Selection.MetaModelPath = metaFile

	first := loadMetaModel(cfg)
	if first == nil {
		t.Fatal("expected non-nil on first load")
	}
	second := loadMetaModel(cfg)
	if first != second {
		t.Fatal("expected cache hit to return same pointer")
	}
}
