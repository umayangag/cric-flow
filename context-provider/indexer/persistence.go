package indexer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func GetIndexPath(root string) string {
	return filepath.Join(root, ".junie", "context_index.json")
}

func SaveContext(root string, ctx *ProjectContext) error {
	path := GetIndexPath(root)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create index file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(ctx); err != nil {
		return fmt.Errorf("failed to encode context: %w", err)
	}
	return nil
}

func LoadContext(root string) (*ProjectContext, error) {
	path := GetIndexPath(root)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ctx ProjectContext
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&ctx); err != nil {
		return nil, fmt.Errorf("failed to decode context: %w", err)
	}
	// Ensure root matches current if needed, or just trust the file content
	// Updating root in memory might be safer if the project moved
	ctx.Root = root
	return &ctx, nil
}
