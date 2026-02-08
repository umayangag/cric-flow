package indexer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

//go:embed parser_script.py
var pythonParserScript string

func ParsePy(path string) ([]Symbol, error) {
	info, err := os.Lstat(path)
    if err != nil {
        return nil, fmt.Errorf("failed to stat file: %w", err)
    }
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinks are not supported: %s", path)
	}

	cmd := exec.Command("python3", "-", path)
	cmd.Stdin = bytes.NewBufferString(pythonParserScript)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("python parse error: %v, stderr: %s", err, stderr.String())
	}

	var symbols []Symbol
	if err := json.Unmarshal(out.Bytes(), &symbols); err != nil {
		return nil, fmt.Errorf("failed to decode symbol json: %v", err)
	}

	return symbols, nil
}
