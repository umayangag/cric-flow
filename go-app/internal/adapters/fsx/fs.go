package fsx

import (
	"context"
	"io/fs"
)

// FS abstracts basic filesystem operations for testability.
//go:generate mockery --name FS --output internal/mocks --case underscore
// NOTE: mocks generated to go-app/internal/mocks (run from module root)
type FS interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	Glob(pattern string) ([]string, error)
}
