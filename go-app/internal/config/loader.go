package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	cached     *Config
	loadedFrom string // path of config file loaded; empty if none found
	loadedRaw  []byte // the bytes it was decoded from, so retired keys can be spotted
	loadErr    error  // set when a found config file's JSON failed to decode
)

// Load reads config.json from the current working directory if present.
// It is safe to call multiple times; the result is cached for the process lifetime.
//
// A file that is found but does not parse as JSON still returns a *Config -- the zero
// value Load started building before the decode failed -- because every caller of Load
// besides ValidateForServer reads it as "the config, or sensible zero values" and none of
// them checks an error return. LoadError is where that decode failure actually surfaces
// (GO-13): ValidateForServer is the one caller whose job is to refuse a broken config
// rather than run on zero values it never chose.
func Load() *Config {
	if cached != nil {
		return cached
	}
	cfg := &Config{}
	loadedFrom = ""
	loadErr = nil
	if p := os.Getenv("GO_APP_CONFIG"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			loadErr = json.Unmarshal(b, cfg)
			cached, loadedFrom, loadedRaw = cfg, p, b
			return cfg
		}
	}
	candidates := []string{
		"config.json",
		filepath.Join("..", "config.json"),
		filepath.Join("..", "..", "config.json"),
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			loadErr = json.Unmarshal(b, cfg)
			cached, loadedFrom, loadedRaw = cfg, p, b
			return cfg
		}
	}
	cached = cfg
	return cfg
}

// LoadError reports the error from decoding the config file Load found, if its JSON was
// invalid. Nil whenever no config file was found at all (ValidateForServer's loadedFrom
// check covers that case) or the file it found decoded cleanly.
func LoadError() error {
	Load()
	return loadErr
}

// ValidateForServer returns an error if config is missing or invalid for the API server.
// Call at server startup; exit on error rather than using fallback defaults.
func ValidateForServer() error {
	cfg := Load()
	if loadedFrom == "" {
		err := fmt.Errorf("config file not found: set GO_APP_CONFIG or ensure config.json exists (CWD, .., or ../..)")
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	if decodeErr := LoadError(); decodeErr != nil {
		err := fmt.Errorf("config file %s is not valid JSON: %w", loadedFrom, decodeErr)
		slog.Error("config.ValidateForServer failed", slog.String("path", loadedFrom), slog.Any("err", err))
		return err
	}
	if err := RetiredKeys(loadedRaw); err != nil {
		slog.Error("config.ValidateForServer failed", slog.String("path", loadedFrom), slog.Any("err", err))
		return err
	}
	if cfg.Pipeline.ImportTimeoutMs < 0 {
		err := fmt.Errorf(
			"pipeline.import_timeout_ms must be >= 0 (0 = no timeout); got %d",
			cfg.Pipeline.ImportTimeoutMs,
		)
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	return nil
}

// PipelineTimeout returns the timeout for an import run.
// Uses pipeline.import_timeout_ms. 0 = no timeout.
func PipelineTimeout() time.Duration {
	cfg := Load()
	if cfg == nil || cfg.Pipeline.ImportTimeoutMs <= 0 {
		return 0
	}
	return time.Duration(cfg.Pipeline.ImportTimeoutMs) * time.Millisecond
}

// DefaultCricsheetDir returns the configured cricsheet input dir or a sensible built-in default.
func DefaultCricsheetDir() string {
	cfg := Load()
	if cfg != nil && cfg.Inputs.CricsheetDir != "" {
		return cfg.Inputs.CricsheetDir
	}
	return filepath.Join("..", "data", "go-app", "cricsheet")
}

// DefaultCricsheetSourceURL is the archive an unconfigured box acquires.
//
// A default rather than a required setting: the whole point of consumer plan W6 is
// that Import works without being told where to get data, and every deployment of this
// project so far wants the same archive.
const DefaultCricsheetSourceURL = "https://cricsheet.org/downloads/all_json.zip"

// CricsheetSourceURL returns the configured archive URL, or the built-in default.
//
// The value is still checked against the download allowlist before anything is
// fetched: being ours is not an exemption, the same rule the named feeds follow.
func CricsheetSourceURL() string {
	cfg := Load()
	if cfg != nil && strings.TrimSpace(cfg.Inputs.CricsheetSourceURL) != "" {
		return strings.TrimSpace(cfg.Inputs.CricsheetSourceURL)
	}
	return DefaultCricsheetSourceURL
}
