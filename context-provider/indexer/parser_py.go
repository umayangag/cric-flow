package indexer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

//go:embed parser_script.py
var pythonParserScript string

const pythonParseTimeout = 30 * time.Second

func ParsePy(path string) ([]Symbol, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinks are not supported: %s", path)
	}

	if info.Size() > MaxParseFileSize {
		return nil, fmt.Errorf("file too large to parse: %d bytes (limit: %d)", info.Size(), MaxParseFileSize)
	}

	pythonCmd := "python3"
	if _, err := exec.LookPath(pythonCmd); err != nil {
		pythonCmd = "python"
	}
	ctx, cancel := context.WithTimeout(context.Background(), pythonParseTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonCmd, "-", path)
	cmd.Stdin = bytes.NewBufferString(pythonParserScript)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("python parse timed out after %v", pythonParseTimeout)
		}
		return nil, fmt.Errorf("python parse error: %v, stderr: %s", err, stderr.String())
	}

	var symbols []Symbol
	if err := json.Unmarshal(out.Bytes(), &symbols); err != nil {
		return nil, fmt.Errorf("failed to decode symbol json: %v", err)
	}

	return symbols, nil
}
