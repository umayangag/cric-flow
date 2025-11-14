package featurecfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func findRepoRoot(start string) string {
	d := start
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(d, "go.work")); err == nil {
			return d
		}
		d = filepath.Dir(d)
	}
	return start
}

func TestLoadFromEnv_ValidConfig(t *testing.T) {
	wd, _ := os.Getwd()
	root := findRepoRoot(wd)
	cfgPath := filepath.Join(root, "configs", "feature_vectors.json")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config not found at %s: %v", cfgPath, err)
	}
	os.Setenv("FEATURE_CONFIG_PATH", cfgPath)
	t.Cleanup(func() { os.Unsetenv("FEATURE_CONFIG_PATH") })

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv error: %v", err)
	}
	if len(cfg.Batting) != 15 || len(cfg.Bowling) != 15 {
		t.Fatalf("unexpected lengths: batting=%d bowling=%d", len(cfg.Batting), len(cfg.Bowling))
	}
	if cfg.Batting[0] != "batting_consistency" {
		t.Fatalf("unexpected first batting feature: %s", cfg.Batting[0])
	}
}

func TestLoad_InvalidName(t *testing.T) {
	wd, _ := os.Getwd()
	root := findRepoRoot(wd)
	path := filepath.Join(root, "output", "test_feature_vectors_invalid.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	bad := map[string][]string{
		"batting": {"batting_consistency", "bad_field"},
		"bowling": {"bowling_consistency"},
	}
	b, _ := json.Marshal(bad)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected error for invalid name, got nil")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(os.TempDir(), "no_such_file_12345.json"))
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}
