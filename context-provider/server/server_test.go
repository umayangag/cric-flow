package server_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
	"github.com/umayangag/cric-info-scrapers/context-provider/server"
)

func TestHandleRequest(t *testing.T) {
	// Initialize server with current dir
	srv := server.NewServer(".")

	tests := []struct {
		name       string
		req        server.Request
		wantResult string // JSON substring to match
		wantError  bool
	}{
		{
			name: "initialize",
			req: server.Request{
				JSONRPC: "2.0",
				Method:  "initialize",
				ID:      jsonRawMessage("1"),
			},
			wantResult: `"serverInfo"`,
		},
		{
			name: "ping",
			req: server.Request{
				JSONRPC: "2.0",
				Method:  "ping",
				ID:      jsonRawMessage("2"),
			},
			wantResult: `{}`,
		},
		{
			name: "tools/list",
			req: server.Request{
				JSONRPC: "2.0",
				Method:  "tools/list",
				ID:      jsonRawMessage("3"),
			},
			wantResult: `"refresh_index"`,
		},
		{
			name: "resources/list",
			req: server.Request{
				JSONRPC: "2.0",
				Method:  "resources/list",
				ID:      jsonRawMessage("4"),
			},
			wantResult: `"context://summary"`,
		},
		{
			name: "unknown method",
			req: server.Request{
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
			srv.HandleRequest(&buf, &tt.req)

			respBytes := buf.Bytes()
			if len(respBytes) == 0 && tt.req.Method == "notifications/initialized" {
				return
			}

			var res server.Response
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
	srv := server.NewServer(".")
	srv.LastContext = &indexer.ProjectContext{
		Stats: indexer.Stats{Files: 42},
	}

	req := server.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "get_summary", "arguments": {}}`),
		ID:      jsonRawMessage("1"),
	}

	var buf bytes.Buffer
	srv.HandleRequest(&buf, &req)

	var res server.Response
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if res.Error != nil {
		t.Fatalf("Unexpected error: %v", res.Error)
	}

	resMap, ok := res.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result is not map[string]interface{}")
	}

	content, ok := resMap["content"].([]interface{}) // JSON unmarshal makes slices []interface{}
	if !ok || len(content) == 0 {
		t.Fatalf("Content invalid")
	}

	firstItem := content[0].(map[string]interface{})
	if !strings.Contains(firstItem["text"].(string), "42") {
		t.Errorf("Summary should contain file count 42")
	}
}

func TestHandleToolCall_RefreshIndex(t *testing.T) {
	// Setup temp dir as project root
	tempDir, err := os.MkdirTemp("", "server_test_refresh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file
	if err := os.WriteFile(filepath.Join(tempDir, "test.go"), []byte("package test"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := server.NewServer(tempDir)

	req := server.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "refresh_index", "arguments": {}}`),
		ID:      jsonRawMessage("1"),
	}

	var buf bytes.Buffer
	srv.HandleRequest(&buf, &req)

	var res server.Response
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if res.Error != nil {
		t.Fatalf("Unexpected error: %v", res.Error)
	}

	// Check if LastContext is updated
	if srv.LastContext == nil {
		t.Fatal("LastContext should be updated")
	}
	if srv.LastContext.Stats.Files != 1 {
		t.Errorf("Expected 1 file, got %d", srv.LastContext.Stats.Files)
	}

	// Verify response text
	resMap, _ := res.Result.(map[string]interface{})
	content, _ := resMap["content"].([]interface{})
	firstItem := content[0].(map[string]interface{})
	if !strings.Contains(firstItem["text"].(string), "Index refreshed") {
		t.Error("Expected success message")
	}
}

func TestHandleResourceRead(t *testing.T) {
	// Setup dummy context
	srv := server.NewServer(".")
	srv.LastContext = &indexer.ProjectContext{
		Stats: indexer.Stats{Files: 100},
	}

	// Test context://summary
	req := server.Request{
		JSONRPC: "2.0",
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri": "context://summary"}`),
		ID:      jsonRawMessage("1"),
	}

	var buf bytes.Buffer
	srv.HandleRequest(&buf, &req)

	var res server.Response
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if res.Error != nil {
		t.Fatalf("Unexpected error: %v", res.Error)
	}

	resMap, _ := res.Result.(map[string]interface{})
	contents, _ := resMap["contents"].([]interface{})
	firstContent := contents[0].(map[string]interface{})
	text := firstContent["text"].(string)

	if !strings.Contains(text, "\"files\": 100") {
		t.Errorf("Resource text missing file count, got: %s", text)
	}

	// Test invalid URI
	reqInvalid := server.Request{
		JSONRPC: "2.0",
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri": "context://invalid"}`),
		ID:      jsonRawMessage("2"),
	}
	buf.Reset()
	srv.HandleRequest(&buf, &reqInvalid)
	
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Error == nil {
		t.Error("Expected error for invalid URI")
	}
}

func TestEnsureContext_LoadFromDisk(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "ensure_ctx_load")
	defer os.RemoveAll(tempDir)

	srv := server.NewServer(tempDir)

	// Create index file
	dummyCtx := &indexer.ProjectContext{Stats: indexer.Stats{Files: 123}}
	if err := indexer.SaveContext(tempDir, dummyCtx); err != nil {
		t.Fatal(err)
	}

	// We can't call ensureContext directly as it's private.
	// But getting summary triggers it.
	req := server.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "get_summary", "arguments": {}}`),
		ID:      jsonRawMessage("1"),
	}

	var buf bytes.Buffer
	srv.HandleRequest(&buf, &req)

	if srv.LastContext == nil || srv.LastContext.Stats.Files != 123 {
		t.Error("Failed to load context from disk")
	}
}

func TestEnsureContext_Scan(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "ensure_ctx_scan")
	defer os.RemoveAll(tempDir)

	srv := server.NewServer(tempDir)

	// Create a file to scan
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Trigger scan via get_summary
	req := server.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "get_summary", "arguments": {}}`),
		ID:      jsonRawMessage("1"),
	}

	var buf bytes.Buffer
	srv.HandleRequest(&buf, &req)

	if srv.LastContext == nil || srv.LastContext.Stats.GoFiles != 1 {
		t.Error("Failed to scan project")
	}
}

func TestHandleToolCall_InvalidJSON(t *testing.T) {
	srv := server.NewServer(".")
	// We can't pass invalid JSON to HandleToolCall directly via HandleRequest because params is RawMessage.
	// But if we pass params that don't match expected structure?
	// The parsing inside handleToolCall: json.Unmarshal(params, &call)
	// If we pass `{"name": ...}` it works.
	// If we pass `[]` it might fail unmarshal to struct.
	
	req := server.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(`[]`), // Array instead of object
		ID:      jsonRawMessage("1"),
	}
	
	var buf bytes.Buffer
	srv.HandleRequest(&buf, &req)
	
	var res server.Response
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if res.Error == nil {
		t.Error("Expected error for invalid params structure")
	}
}

func TestHandleToolCall_RefreshIndex_Error(t *testing.T) {
	// Set invalid root
	srv := server.NewServer("/invalid/path/that/does/not/exist")

	req := server.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "refresh_index", "arguments": {}}`),
		ID:      jsonRawMessage("1"),
	}
	
	var buf bytes.Buffer
	srv.HandleRequest(&buf, &req)

	var res server.Response
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if res.Error == nil {
		t.Error("Expected error for refresh_index with invalid root")
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
	if err := os.WriteFile(goWorkPath, []byte("go 1.21"), 0o600); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	deepDir := filepath.Join(subdir, "deep")
	if err := os.Mkdir(deepDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Test case 1: Start from root
	if root := server.FindProjectRoot(tmpDir); root != tmpDir {
		t.Errorf("Expected root %s, got %s", tmpDir, root)
	}

	// Test case 2: Start from subdir
	if root := server.FindProjectRoot(subdir); root != tmpDir {
		t.Errorf("Expected root %s from subdir, got %s", tmpDir, root)
	}

	// Test case 3: Start from deep dir
	if root := server.FindProjectRoot(deepDir); root != tmpDir {
		t.Errorf("Expected root %s from deep dir, got %s", tmpDir, root)
	}

	// Fallback logic
	tmpDirFallback, _ := os.MkdirTemp("", "cric-info-context-fallback")
	defer os.RemoveAll(tmpDirFallback)
	subdirFallback := filepath.Join(tmpDirFallback, "subdir")
	if err := os.Mkdir(subdirFallback, 0o755); err != nil {
		t.Fatal(err)
	}

	root := server.FindProjectRoot(subdirFallback)
	// Check if it fell back to subdir (assuming no go.work up)
	if root != subdirFallback {
		if _, err := os.Stat(filepath.Join(root, "go.work")); os.IsNotExist(err) {
			t.Errorf("Expected fallback to %s, got %s which has no go.work", subdirFallback, root)
		}
	}
}
