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
// relative to the go-app directory.
func DefaultPath() string {
	// go-app/internal/featurecfg -> go-app -> repo root -> configs/feature_vectors.json
	wd, _ := os.Getwd()
	// Try to find repo root by looking for go.work near cwd; fall back to relative path
	// Keep it simple: assume running from go-app or repo root in tests/CI.
	// Prefer ../configs when current dir is go-app, otherwise ./configs at repo root.
	if strings.HasSuffix(filepath.Base(wd), "go-app") {
		return filepath.Clean(filepath.Join(wd, "..", "configs", "feature_vectors.json"))
	}
	return filepath.Clean(filepath.Join(wd, "configs", "feature_vectors.json"))
}

// LoadFromEnv loads the config using FEATURE_CONFIG_PATH if set, otherwise DefaultPath().
func LoadFromEnv() (Config, error) {
	p := strings.TrimSpace(os.Getenv("FEATURE_CONFIG_PATH"))
	if p == "" {
		p = DefaultPath()
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
