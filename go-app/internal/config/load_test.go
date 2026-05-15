package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to write a small config.json file
func writeConfigFile(t *testing.T, dir string, content string) string {
	t.Helper()
	p := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestLoad_EnvPathPrecedence(t *testing.T) {
	cached = nil

	tmp := t.TempDir()
	cfgJSON := `{"inputs":{"cricsheet_dir":"/env/cricsheet"},"outputs":{"export_dir":"/env/export"}}`
	p := filepath.Join(tmp, "custom.json")
	require.NoError(t, os.WriteFile(p, []byte(cfgJSON), 0o600))
	t.Setenv("GO_APP_CONFIG", p)

	c := Load()
	assert.Equal(t, "/env/cricsheet", c.Inputs.CricsheetDir)
	assert.Equal(t, "/env/export", c.Outputs.ExportDir)
}

func TestLoad_ConfigJsonFromCWD(t *testing.T) {
	cached = nil
	oldWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	cfgJSON := `{"inputs":{"cricsheet_dir":"/cwd/cricsheet"},"outputs":{"export_dir":"/cwd/export"}}`
	_ = writeConfigFile(t, tmp, cfgJSON)
	t.Setenv("GO_APP_CONFIG", "")

	c := Load()
	assert.Equal(t, "/cwd/cricsheet", c.Inputs.CricsheetDir)
	assert.Equal(t, "/cwd/export", c.Outputs.ExportDir)
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

	Load()
	err := ValidateForServer()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")
}

func TestValidateForServer_FailsWhenPrecomputeTimeoutNegative(t *testing.T) {
	cached = nil
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	cfgJSON := `{"features":{"precompute_timeout_ms":-1}}`
	require.NoError(t, os.WriteFile(p, []byte(cfgJSON), 0o600))
	t.Setenv("GO_APP_CONFIG", p)

	Load()
	err := ValidateForServer()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "precompute_timeout_ms")
}

func TestValidateForServer_FailsWhenExportTimeoutNegative(t *testing.T) {
	cached = nil
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	cfgJSON := `{"features":{"export_timeout_ms":-1}}`
	require.NoError(t, os.WriteFile(p, []byte(cfgJSON), 0o600))
	t.Setenv("GO_APP_CONFIG", p)

	Load()
	err := ValidateForServer()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "export_timeout_ms")
}

func TestLoad_CachePersistsUntilReset(t *testing.T) {
	cached = nil
	tmp := t.TempDir()
	p := filepath.Join(tmp, "a.json")
	first := `{"inputs":{"cricsheet_dir":"/first"}}`
	second := `{"inputs":{"cricsheet_dir":"/second"}}`
	require.NoError(t, os.WriteFile(p, []byte(first), 0o600))
	t.Setenv("GO_APP_CONFIG", p)

	c1 := Load()
	assert.Equal(t, "/first", c1.Inputs.CricsheetDir)

	// mutate file
	require.NoError(t, os.WriteFile(p, []byte(second), 0o600))
	c2 := Load()
	assert.Equal(t, "/first", c2.Inputs.CricsheetDir, "cache should still return first value")

	// reset cache and verify change is picked up
	cached = nil
	c3 := Load()
	assert.Equal(t, "/second", c3.Inputs.CricsheetDir)
}

func TestLoadMetaModel_NilConfigReturnsNil(t *testing.T) {
	got := loadMetaModel(nil)
	assert.Nil(t, got)
}

func TestLoadMetaModel_EmptyPathReturnsNil(t *testing.T) {
	cfg := &Config{}
	got := loadMetaModel(cfg)
	assert.Nil(t, got)
}

func TestLoadMetaModel_FileNotFoundReturnsNil(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	cfg := &Config{}
	cfg.Selection.MetaModelPath = "/nonexistent/path/meta.json"
	got := loadMetaModel(cfg)
	assert.Nil(t, got)
}

func TestLoadMetaModel_InvalidJSONReturnsNil(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{not json}`), 0o600))
	cfg := &Config{}
	cfg.Selection.MetaModelPath = bad
	got := loadMetaModel(cfg)
	assert.Nil(t, got)
}

func TestLoadMetaModel_RelativePathResolvedAgainstConfigDir(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	cached = nil

	tmp := t.TempDir()
	metaContent := `{"bat":0.4,"bowl":0.3,"field":0.2,"keeper_bonus":0.1}`
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "meta.json"), []byte(metaContent), 0o600))
	cfgPath := filepath.Join(tmp, "config.json")
	cfgJSON := `{"selection":{"meta_model_path":"meta.json"}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgJSON), 0o600))
	t.Setenv("GO_APP_CONFIG", cfgPath)
	cfg := Load()

	got := loadMetaModel(cfg)
	require.NotNil(t, got)
	assert.Equal(t, 0.4, got.Bat)
}

func TestLoadMetaModel_CacheHitReturnsSamePointer(t *testing.T) {
	metaModelCache = nil
	metaModelPath = ""
	cached = nil

	tmp := t.TempDir()
	metaContent := `{"bat":0.5,"bowl":0.3,"field":0.15,"keeper_bonus":0.05}`
	metaFile := filepath.Join(tmp, "meta_cache.json")
	require.NoError(t, os.WriteFile(metaFile, []byte(metaContent), 0o600))
	cfg := &Config{}
	cfg.Selection.MetaModelPath = metaFile

	firstResult := loadMetaModel(cfg)
	require.NotNil(t, firstResult)
	secondResult := loadMetaModel(cfg)
	assert.Same(t, firstResult, secondResult, "cache hit should return same pointer")
}
