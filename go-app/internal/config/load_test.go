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
	cfgJSON := `{"inputs":{"cricsheet_dir":"/env/cricsheet"},"outputs":{"dir":"/env/export"}}`
	p := filepath.Join(tmp, "custom.json")
	require.NoError(t, os.WriteFile(p, []byte(cfgJSON), 0o600))
	t.Setenv("GO_APP_CONFIG", p)

	c := Load()
	assert.Equal(t, "/env/cricsheet", c.Inputs.CricsheetDir)
	assert.Equal(t, "/env/export", c.Outputs.Dir)
}

func TestLoad_ConfigJsonFromCWD(t *testing.T) {
	cached = nil
	oldWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	cfgJSON := `{"inputs":{"cricsheet_dir":"/cwd/cricsheet"},"outputs":{"dir":"/cwd/export"}}`
	_ = writeConfigFile(t, tmp, cfgJSON)
	t.Setenv("GO_APP_CONFIG", "")

	c := Load()
	assert.Equal(t, "/cwd/cricsheet", c.Inputs.CricsheetDir)
	assert.Equal(t, "/cwd/export", c.Outputs.Dir)
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

func TestValidateForServer_FailsWhenImportTimeoutNegative(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"pipeline":{"import_timeout_ms":-1}}`), 0o600))
	t.Setenv("GO_APP_CONFIG", path)
	cached = nil

	err := ValidateForServer()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "import_timeout_ms")
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
