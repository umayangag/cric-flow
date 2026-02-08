package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

func TestHandleRequest(t *testing.T) {
	// Initialize global vars
	projectRoot = "."

	tests := []struct {
		name       string
		req        Request
		wantResult string // JSON substring to match
		wantError  bool
	}{
		{
			name: "initialize",
			req: Request{
				JSONRPC: "2.0",
				Method:  "initialize",
				ID:      jsonRawMessage("1"),
			},
			wantResult: `"serverInfo"`,
		},
		{
			name: "ping",
			req: Request{
				JSONRPC: "2.0",
				Method:  "ping",
				ID:      jsonRawMessage("2"),
			},
			wantResult: `{}`,
		},
		{
			name: "tools/list",
			req: Request{
				JSONRPC: "2.0",
				Method:  "tools/list",
				ID:      jsonRawMessage("3"),
			},
			wantResult: `"refresh_index"`,
		},
		{
			name: "resources/list",
			req: Request{
				JSONRPC: "2.0",
				Method:  "resources/list",
				ID:      jsonRawMessage("4"),
			},
			wantResult: `"context://summary"`,
		},
		{
			name: "unknown method",
			req: Request{
				JSONRPC: "2.0",
				Method:  "unknown/method",
				ID:      jsonRawMessage("5"),
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handleRequest(&buf, &tt.req)

			respBytes := buf.Bytes()
			if len(respBytes) == 0 && tt.req.Method == "notifications/initialized" {
				return
			}

			var res Response
			if err := json.Unmarshal(respBytes, &res); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if tt.wantError {
				if res.Error == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if res.Error != nil {
					t.Errorf("Unexpected error: %v", res.Error)
				}
				resultJSON, _ := json.Marshal(res.Result)
				if !strings.Contains(string(resultJSON), tt.wantResult) {
					t.Errorf("Expected result to contain %s, got %s", tt.wantResult, string(resultJSON))
				}
			}
		})
	}
}

func jsonRawMessage(s string) *json.RawMessage {
	msg := json.RawMessage(s)
	return &msg
}

func TestHandleToolCall_GetSummary(t *testing.T) {
	// Setup dummy context
	lastContext = &indexer.ProjectContext{
		Stats: indexer.Stats{Files: 42},
	}

	params := json.RawMessage(`{"name": "get_summary", "arguments": {}}`)
	res, rpcErr := handleToolCall(params)
	if rpcErr != nil {
		t.Fatalf("Unexpected error: %v", rpcErr)
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("Result is not map[string]interface{}")
	}

	content, ok := resMap["content"].([]map[string]string)
	if !ok || len(content) == 0 {
		t.Fatalf("Content invalid")
	}

	if !strings.Contains(content[0]["text"], "42") {
		t.Errorf("Summary should contain file count 42")
	}
}

func TestHandleToolCall_RefreshIndex(t *testing.T) {
	// Setup temp dir as project root
	tempDir, err := os.MkdirTemp("", "main_test_refresh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file
	if err := os.WriteFile(filepath.Join(tempDir, "test.go"), []byte("package test"), 0600); err != nil {
		t.Fatal(err)
	}

	projectRoot = tempDir

	params := json.RawMessage(`{"name": "refresh_index", "arguments": {}}`)
	res, rpcErr := handleToolCall(params)
	if rpcErr != nil {
		t.Fatalf("Unexpected error: %v", rpcErr)
	}

	// Check if lastContext is updated
	if lastContext == nil {
		t.Fatal("lastContext should be updated")
	}
	if lastContext.Stats.Files != 1 {
		t.Errorf("Expected 1 file, got %d", lastContext.Stats.Files)
	}

	// Verify response text
	resMap, _ := res.(map[string]interface{})
	content, _ := resMap["content"].([]map[string]string)
	if !strings.Contains(content[0]["text"], "Index refreshed") {
		t.Error("Expected success message")
	}
}

func TestHandleResourceRead(t *testing.T) {
	// Setup dummy context
	lastContext = &indexer.ProjectContext{
		Stats: indexer.Stats{Files: 100},
	}

	// Test context://summary
	params := json.RawMessage(`{"uri": "context://summary"}`)
	res, rpcErr := handleResourceRead(params)
	if rpcErr != nil {
		t.Fatalf("Unexpected error: %v", rpcErr)
	}

	resMap, _ := res.(map[string]interface{})
	contents, _ := resMap["contents"].([]map[string]interface{})
	text := contents[0]["text"].(string)

	if !strings.Contains(text, "\"files\": 100") {
		t.Errorf("Resource text missing file count, got: %s", text)
	}

	// Test invalid URI
	paramsInvalid := json.RawMessage(`{"uri": "context://invalid"}`)
	_, rpcErr = handleResourceRead(paramsInvalid)
	if rpcErr == nil {
		t.Error("Expected error for invalid URI")
	}
}

func TestEnsureContext_LoadFromDisk(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "ensure_ctx_load")
	defer os.RemoveAll(tempDir)

	projectRoot = tempDir
	lastContext = nil

	// Create index file
	dummyCtx := &indexer.ProjectContext{Stats: indexer.Stats{Files: 123}}
	if err := indexer.SaveContext(tempDir, dummyCtx); err != nil {
		t.Fatal(err)
	}

	err := ensureContext()
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	if lastContext == nil || lastContext.Stats.Files != 123 {
		t.Error("Failed to load context from disk")
	}
}

func TestEnsureContext_Scan(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "ensure_ctx_scan")
	defer os.RemoveAll(tempDir)

	projectRoot = tempDir
	lastContext = nil

	// Create a file to scan
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	err := ensureContext()
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	if lastContext == nil || lastContext.Stats.GoFiles != 1 {
		t.Error("Failed to scan project")
	}
}

func TestSendResponse_MarshalError(t *testing.T) {
	var buf bytes.Buffer
	id := jsonRawMessage("1")
	// pass a channel which fails json marshal
	sendResponse(&buf, id, make(chan int), nil)

	output := buf.String()
	if !strings.Contains(output, "-32603") {
		t.Errorf("Expected internal error code -32603, got %s", output)
	}
}

func TestHandleToolCall_InvalidJSON(t *testing.T) {
	params := json.RawMessage(`{invalid_json`)
	_, err := handleToolCall(params)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestHandleToolCall_RefreshIndex_Error(t *testing.T) {
	// Set invalid root
	projectRoot = "/invalid/path/that/does/not/exist"

	params := json.RawMessage(`{"name": "refresh_index", "arguments": {}}`)
	_, err := handleToolCall(params)
	if err == nil {
		t.Error("Expected error for refresh_index with invalid root")
	}
}

func TestHandleResourceRead_InvalidJSON(t *testing.T) {
	params := json.RawMessage(`{invalid_json`)
	_, err := handleResourceRead(params)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestHandleRequest_Notification(t *testing.T) {
	var buf bytes.Buffer
	req := Request{
		JSONRPC: "2.0",
		Method:  "unknown/notification",
		ID:      nil,
	}

	handleRequest(&buf, &req)

	if buf.Len() > 0 {
		t.Errorf("Expected no response for unknown notification, got %s", buf.String())
	}
}

func TestHandleToolCall_GetSummary_Error(t *testing.T) {
	projectRoot = "/invalid/root"
	lastContext = nil // Force ensureContext to run

	params := json.RawMessage(`{"name": "get_summary", "arguments": {}}`)
	_, err := handleToolCall(params)
	if err == nil {
		t.Error("Expected error for get_summary with invalid root")
	}
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	params := json.RawMessage(`{"name": "unknown_tool", "arguments": {}}`)
	_, err := handleToolCall(params)
	if err == nil {
		t.Error("Expected error for unknown tool")
		return
	}
	if err.Code != -32601 {
		t.Errorf("Expected code -32601, got %d", err.Code)
	}
}

func TestFindProjectRoot(t *testing.T) {
	// Create a temporary directory structure
	// /tmp/test-root/go.work
	// /tmp/test-root/subdir/
	// /tmp/test-root/subdir/deep/

	tmpDir, err := os.MkdirTemp("", "cric-info-context-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create go.work in root
	goWorkPath := filepath.Join(tmpDir, "go.work")
	if err := os.WriteFile(goWorkPath, []byte("go 1.21"), 0600); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	deepDir := filepath.Join(subdir, "deep")
	if err := os.Mkdir(deepDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test case 1: Start from root
	if root := findProjectRoot(tmpDir); root != tmpDir {
		t.Errorf("Expected root %s, got %s", tmpDir, root)
	}

	// Test case 2: Start from subdir
	if root := findProjectRoot(subdir); root != tmpDir {
		t.Errorf("Expected root %s from subdir, got %s", tmpDir, root)
	}

	// Test case 3: Start from deep dir
	if root := findProjectRoot(deepDir); root != tmpDir {
		t.Errorf("Expected root %s from deep dir, got %s", tmpDir, root)
	}

	// Fallback logic
	tmpDirFallback, _ := os.MkdirTemp("", "cric-info-context-fallback")
	defer os.RemoveAll(tmpDirFallback)
	subdirFallback := filepath.Join(tmpDirFallback, "subdir")
	if err := os.Mkdir(subdirFallback, 0755); err != nil {
		t.Fatal(err)
	}

	root := findProjectRoot(subdirFallback)
	// Check if it fell back to subdir (assuming no go.work up)
	if root != subdirFallback {
		if _, err := os.Stat(filepath.Join(root, "go.work")); os.IsNotExist(err) {
			t.Errorf("Expected fallback to %s, got %s which has no go.work", subdirFallback, root)
		}
	}
}
