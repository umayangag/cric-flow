package indexer

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

//go:embed parser_script.py
var pythonParserScript string

type PythonBatchParser struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Scanner
	mu        sync.Mutex
	scriptTmp string
	pythonCmd string
}

func NewPythonBatchParser() (*PythonBatchParser, error) {
	// 1. Write script to temp file
	tmpFile, err := os.CreateTemp("", "parser_script_*.py")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := tmpFile.WriteString(pythonParserScript); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to write script: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// 2. Determine python command
	pythonCmd := "python3"
	if _, err := exec.LookPath(pythonCmd); err != nil {
		pythonCmd = "python"
		if _, err := exec.LookPath(pythonCmd); err != nil {
			os.Remove(tmpPath)
			return nil, fmt.Errorf("neither 'python3' nor 'python' were found in PATH")
		}
	}

	parser := &PythonBatchParser{
		scriptTmp: tmpPath,
		pythonCmd: pythonCmd,
	}

	// 3. Start process
	if err := parser.startProcess(); err != nil {
		os.Remove(tmpPath)
		return nil, err
	}

	return parser, nil
}

func (p *PythonBatchParser) startProcess() error {
	// No arguments -> loop mode
	//nolint:gosec // p.pythonCmd is resolved via LookPath and p.scriptTmp is a created temp file
	cmd := exec.Command(p.pythonCmd, p.scriptTmp)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Inherit stderr for logging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to start python process: %w", err)
	}

	p.cmd = cmd
	p.stdin = stdin
	p.stdout = bufio.NewScanner(stdoutPipe)
	// Increase buffer size to handle large JSON outputs
	buf := make([]byte, 1024*1024)
	p.stdout.Buffer(buf, 10*1024*1024)
	return nil
}

func (p *PythonBatchParser) restartProcess() error {
	// Cleanup old process
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}

	return p.startProcess()
}

func (p *PythonBatchParser) Parse(path string) ([]Symbol, error) {
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

	p.mu.Lock()
	defer p.mu.Unlock()

	// Helper to perform parsing
	doParse := func() ([]Symbol, error) {
		if p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() {
			return nil, fmt.Errorf("python process exited unexpectedly")
		}

		// Write path
		if _, err := fmt.Fprintln(p.stdin, path); err != nil {
			return nil, fmt.Errorf("failed to write to python process: %w", err)
		}

		// Read response
		if !p.stdout.Scan() {
			if err := p.stdout.Err(); err != nil {
				return nil, fmt.Errorf("error reading from python: %w", err)
			}
			return nil, fmt.Errorf("python process closed stdout")
		}

		line := p.stdout.Bytes()
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			var errObj struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(trimmed, &errObj); err == nil && errObj.Error != "" {
				return nil, fmt.Errorf("%s", errObj.Error)
			}
		}

		var symbols []Symbol
		if err := json.Unmarshal(line, &symbols); err != nil {
			return nil, fmt.Errorf("failed to decode symbol json: %v", err)
		}

		return symbols, nil
	}

	// Attempt 1
	symbols, err := doParse()
	if err == nil {
		return symbols, nil
	}

	// Retry logic
	isTransportError := false
	errMsg := err.Error()
	if (p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited()) ||
		errMsg == "python process exited unexpectedly" ||
		errMsg == "python process closed stdout" ||
		strings.Contains(errMsg, "failed to write") ||
		strings.Contains(errMsg, "error reading") {
		isTransportError = true
	}

	if isTransportError {
		// Attempt restart
		if restartErr := p.restartProcess(); restartErr != nil {
			return nil, fmt.Errorf("failed to restart python process: %v (original error: %v)", restartErr, err)
		}
		// Attempt 2
		return doParse()
	}

	return nil, err
}

func (p *PythonBatchParser) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
	if p.scriptTmp != "" {
		_ = os.Remove(p.scriptTmp)
	}
}
