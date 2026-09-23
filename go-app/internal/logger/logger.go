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
//
// ops is every WithAttrs/WithGroup call this handler's chain has seen, in the order they
// were made (GO-15). Handle rebuilds the inner handler fresh on every call (it writes into
// a per-call buffer, so there is no long-lived inner handler to keep) and replays ops onto
// it: slog.Handler's own contract is a call chain, where each WithAttrs/WithGroup returns a
// handler carrying everything before it plus the new operation, and a WithGroup followed by
// a WithAttrs scopes those attrs inside the group -- which only replaying in the same order
// preserves. Before this, WithAttrs and WithGroup both returned the receiver unchanged, so
// every slog.With(...) or WithGroup(...) a caller made vanished the moment LOG_COLOR=1 set
// this handler as the default.
type colorHandler struct {
	level  *slog.LevelVar
	format string // "text" or "json"
	ops    []func(slog.Handler) slog.Handler
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
	for _, op := range h.ops {
		inner = op(inner)
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

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.withOp(func(inner slog.Handler) slog.Handler {
		return inner.WithAttrs(attrs)
	})
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	return h.withOp(func(inner slog.Handler) slog.Handler {
		return inner.WithGroup(name)
	})
}

// withOp returns a new colorHandler carrying every operation this one had, plus op.
// slog.Handler's contract requires WithAttrs/WithGroup to leave the receiver usable
// afterwards -- a caller may still hold and use the handler it called this on -- so this
// copies rather than appending onto h.ops in place.
func (h *colorHandler) withOp(op func(slog.Handler) slog.Handler) *colorHandler {
	ops := make([]func(slog.Handler) slog.Handler, len(h.ops), len(h.ops)+1)
	copy(ops, h.ops)
	ops = append(ops, op)
	return &colorHandler{level: h.level, format: h.format, ops: ops}
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
