package logging

import (
	"context"
)

// Logger is a small, test-friendly logging interface.
//go:generate mockery --name Logger --output internal/mocks --case underscore
// NOTE: This interface is separate from the slog-backed helpers in this package
// to allow commands/services to depend on an interface and use mocks in tests.
type Logger interface {
	Debugf(ctx context.Context, format string, args ...any)
	Infof(ctx context.Context, format string, args ...any)
	Warnf(ctx context.Context, format string, args ...any)
	Errorf(ctx context.Context, format string, args ...any)
	With(key string, value any) Logger
}
