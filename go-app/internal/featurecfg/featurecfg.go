// Package featurecfg loads the centralized feature vector configuration
// (configs/feature_vectors.json) and exposes ordered feature names.
package featurecfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

// Config is the JSON schema for the feature vectors definition.
type Config struct {
	Batting []string `json:"batting"`
	Bowling []string `json:"bowling"`
}

// DefaultPath resolves the repo-root default path to configs/feature_vectors.json
// by walking up from the current directory until a repository marker is found.
// Repository markers checked (in order): go.work, .git directory.
// Returns the resolved path or an error if the current working directory cannot be determined.
func DefaultPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("featurecfg: could not determine current working directory: %w", err)
	}
	// Walk upwards to locate repository root
	d := wd
	for i := 0; i < 10; i++ { // guard to avoid infinite loops
		if _, err := os.Stat(filepath.Join(d, "go.work")); err == nil {
			return filepath.Clean(filepath.Join(d, "configs", "feature_vectors.json")), nil
		}
		if info, err := os.Stat(filepath.Join(d, ".git")); err == nil && info.IsDir() {
			return filepath.Clean(filepath.Join(d, "configs", "feature_vectors.json")), nil
		}
		parent := filepath.Dir(d)
		if parent == d { // reached filesystem root
			break
		}
		d = parent
	}
	// Fallback to starting directory under configs
	return filepath.Clean(filepath.Join(wd, "configs", "feature_vectors.json")), nil
}

// LoadFromEnv loads the config using FEATURE_CONFIG_PATH if set, otherwise DefaultPath().
func LoadFromEnv() (Config, error) {
	p := strings.TrimSpace(os.Getenv("FEATURE_CONFIG_PATH"))
	if p == "" {
		var err error
		p, err = DefaultPath()
		if err != nil {
			return Config{}, fmt.Errorf("featurecfg: unable to resolve default config path: %w", err)
		}
	}
	return Load(p)
}

// Load reads the config from path and validates names against contracts.* JSON tags.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if len(cfg.Batting) == 0 || len(cfg.Bowling) == 0 {
		return Config{}, errors.New("featurecfg: batting/bowling lists must be non-empty")
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	batTags := jsonTags(reflect.TypeOf(contracts.BattingFeatures{}))
	bowlTags := jsonTags(reflect.TypeOf(contracts.BowlingFeatures{}))
	for i, n := range c.Batting {
		if _, ok := batTags[n]; !ok {
			return fmt.Errorf("featurecfg: batting[%d]=%q not found in contracts.BattingFeatures json tags", i, n)
		}
	}
	for i, n := range c.Bowling {
		if _, ok := bowlTags[n]; !ok {
			return fmt.Errorf("featurecfg: bowling[%d]=%q not found in contracts.BowlingFeatures json tags", i, n)
		}
	}
	return nil
}

func jsonTags(t reflect.Type) map[string]struct{} {
	m := make(map[string]struct{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		// take name before comma
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}
		m[tag] = struct{}{}
	}
	return m
}
