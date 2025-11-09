package osfs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
)

// OSFS is a concrete implementation of fsx.FS backed by the standard library.
// It intentionally ignores context for filesystem operations because the
// underlying os package does not support cancellation.
type OSFS struct{}

// New returns a new OSFS instance.
func New() *OSFS { return &OSFS{} }

// ReadFile reads the entire file content at path.
func (OSFS) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes data to the file at path with the given permissions.
func (OSFS) WriteFile(_ context.Context, path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// MkdirAll creates a directory and all parents with the given permissions.
func (OSFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }

// Glob returns the names of all files matching pattern.
func (OSFS) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }
