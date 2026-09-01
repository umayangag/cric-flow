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
)

// Load reads config.json from the current working directory if present.
// It is safe to call multiple times; the result is cached for the process lifetime.
func Load() *Config {
	if cached != nil {
		return cached
	}
	cfg := &Config{}
	loadedFrom = ""
	if p := os.Getenv("GO_APP_CONFIG"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, cfg)
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
			_ = json.Unmarshal(b, cfg)
			cached, loadedFrom, loadedRaw = cfg, p, b
			return cfg
		}
	}
	cached = cfg
	return cfg
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
	if err := RetiredKeys(loadedRaw); err != nil {
		slog.Error("config.ValidateForServer failed", slog.String("path", loadedFrom), slog.Any("err", err))
		return err
	}
	if cfg.Features.PrecomputeTimeoutMs < 0 {
		err := fmt.Errorf(
			"features.precompute_timeout_ms must be >= 0 (0 = no timeout); got %d",
			cfg.Features.PrecomputeTimeoutMs,
		)
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	if cfg.Features.ExportTimeoutMs < 0 {
		err := fmt.Errorf(
			"features.export_timeout_ms must be >= 0 (0 = use pipeline timeout); got %d",
			cfg.Features.ExportTimeoutMs,
		)
		slog.Error("config.ValidateForServer failed", slog.Any("err", err))
		return err
	}
	return nil
}

// PipelineTimeout returns the timeout for long-running pipeline jobs (import, precompute).
// Uses features.precompute_timeout_ms. 0 = no timeout.
func PipelineTimeout() time.Duration {
	cfg := Load()
	if cfg == nil || cfg.Features.PrecomputeTimeoutMs <= 0 {
		return 0
	}
	return time.Duration(cfg.Features.PrecomputeTimeoutMs) * time.Millisecond
}

// ExportTimeout returns the timeout for the export-dataset pipeline step.
// Uses features.export_timeout_ms when set (must be > 0); otherwise falls back to PipelineTimeout().
// A resulting duration of 0 means no timeout (only shutdown cancels).
func ExportTimeout() time.Duration {
	cfg := Load()
	if cfg != nil && cfg.Features.ExportTimeoutMs > 0 {
		return time.Duration(cfg.Features.ExportTimeoutMs) * time.Millisecond
	}
	return PipelineTimeout()
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
