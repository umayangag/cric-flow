package logger

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
)

// ANSI escape codes for coloring log lines in terminal/docker logs.
const (
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiReset  = "\033[0m"
)

// colorHandler wraps a slog handler and prefixes error/warn lines with ANSI color
// so they stand out in docker logs and terminals. Enabled when LOG_COLOR=1.
type colorHandler struct {
	level  *slog.LevelVar
	format string // "text" or "json"
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *colorHandler) Handle(ctx context.Context, r slog.Record) error {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: h.level}
	var inner slog.Handler
	if h.format == "text" {
		inner = slog.NewTextHandler(&buf, opts)
	} else {
		inner = slog.NewJSONHandler(&buf, opts)
	}
	if err := inner.Handle(ctx, r); err != nil {
		return err
	}
	b := buf.Bytes()
	out := os.Stdout
	switch {
	case r.Level >= slog.LevelError:
		_, _ = out.Write([]byte(ansiRed))
		_, _ = out.Write(b)
		_, _ = out.Write([]byte(ansiReset))
	case r.Level >= slog.LevelWarn:
		_, _ = out.Write([]byte(ansiYellow))
		_, _ = out.Write(b)
		_, _ = out.Write([]byte(ansiReset))
	default:
		_, _ = out.Write(b)
	}
	return nil
}

func (h *colorHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *colorHandler) WithGroup(_ string) slog.Handler {
	return h
}

// SetupFromEnv configures the global slog default logger.
//
// Environment variables:
//
//	LOG_FORMAT = json|text (default: json)
//	LOG_LEVEL  = debug|info|warn|error (default: info)
//	LOG_COLOR  = 1 to enable ANSI color for error (red) and warn (yellow) in docker/terminal logs
func SetupFromEnv() *slog.Logger {
	level := new(slog.LevelVar)
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn", "warning":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}

	format := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if format != "text" {
		format = "json"
	}
	useColor := strings.TrimSpace(os.Getenv("LOG_COLOR")) == "1"

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if useColor {
		handler = &colorHandler{level: level, format: format}
	} else {
		switch format {
		case "text":
			handler = slog.NewTextHandler(os.Stdout, opts)
		default:
			handler = slog.NewJSONHandler(os.Stdout, opts)
		}
	}

	lg := slog.New(handler)
	slog.SetDefault(lg)
	return lg
}

// L returns the global default slog logger (after SetupFromEnv).
func L() *slog.Logger { return slog.Default() }
