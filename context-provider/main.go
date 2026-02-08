package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/umayangag/cric-info-scrapers/context-provider/indexer"
)

// MCP Protocol Types
type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
	ID      *json.RawMessage `json:"id,omitempty"`
}

type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
	ID      *json.RawMessage `json:"id,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Global state
var (
	projectRoot string
	lastContext *indexer.ProjectContext
)

func main() {
	rootFlag := flag.String("root", "", "Path to project root")
	flag.Parse()

	// Logging to stderr so it doesn't interfere with stdout JSON-RPC
	log.SetOutput(os.Stderr)
	log.Println("Starting Context MCP Server...")

	if *rootFlag != "" {
		projectRoot = *rootFlag
	} else {
		// Determine root
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Failed to get current working directory: %v", err)
		}
		// Assume we run from root, or parent is root if running inside module
		// Logic: If we see go.work in current dir, it's root.
		// If we see go.mod and parent has go.work, parent is root.
		if _, err := os.Stat(filepath.Join(wd, "go.work")); err == nil {
			projectRoot = wd
		} else if filepath.Base(wd) == "context-provider" {
			projectRoot = filepath.Dir(wd)
		} else {
			projectRoot = wd // Fallback
		}
	}
	log.Printf("Project Root: %s", projectRoot)

	// Initialize context (Load from disk or Scan)
	if ctx, err := indexer.LoadContext(projectRoot); err == nil {
		lastContext = ctx
		log.Printf("Context loaded from disk (%d files)", ctx.Stats.Files)
	} else {
		log.Printf("No existing context found or load failed: %v. Scanning now...", err)
		// Perform initial scan
		ctx, err := indexer.ScanProject(projectRoot)
		if err != nil {
			log.Printf("Initial scan failed: %v", err)
		} else {
			lastContext = ctx
			if err := indexer.SaveContext(projectRoot, ctx); err != nil {
				log.Printf("Failed to save context to disk: %v", err)
			} else {
				log.Printf("Context scanned and saved to disk.")
			}
		}
	}

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size just in case
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("Error unmarshaling: %v", err)
			continue
		}
		handleRequest(&req)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("Scanner error: %v", err)
	}
}

func handleRequest(req *Request) {
	var res interface{}
	var err *RPCError

	log.Printf("Received request: %s", req.Method)

	switch req.Method {
	case "initialize":
		res = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    "context-provider",
				"version": "0.1.0",
			},
		}
	case "notifications/initialized":
		// No response needed for notifications
		return
	case "tools/list":
		res = map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "refresh_index",
					"description": "Scans the project and rebuilds the context index.",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
				{
					"name":        "get_summary",
					"description": "Returns a high-level summary of the project structure and statistics.",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
			},
		}
	case "tools/call":
		res, err = handleToolCall(req.Params)
	case "resources/list":
		res = map[string]interface{}{
			"resources": []map[string]interface{}{
				{
					"uri":      "context://summary",
					"name":     "Project Context Summary",
					"mimeType": "application/json",
				},
			},
		}
	case "resources/read":
		res, err = handleResourceRead(req.Params)
	case "ping":
		res = map[string]interface{}{}
	default:
		// Ignore unknown notifications
		if req.ID == nil {
			return
		}
		err = &RPCError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	if req.ID != nil {
		response := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  res,
			Error:   err,
		}
		bytes, err := json.Marshal(response)
		if err != nil {
			log.Printf("Error: failed to marshal response for request %v: %v", req.ID, err)
			return
		}
		fmt.Printf("%s\n", bytes)
	}
}

func handleToolCall(params json.RawMessage) (interface{}, *RPCError) {
	var call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &RPCError{Code: -32700, Message: "Parse error"}
	}

	switch call.Name {
	case "refresh_index":
		ctx, err := indexer.ScanProject(projectRoot)
		if err != nil {
			return nil, &RPCError{Code: 1, Message: err.Error()}
		}
		lastContext = ctx
		if err := indexer.SaveContext(projectRoot, ctx); err != nil {
			log.Printf("Failed to save refreshed context: %v", err)
			// We don't fail the RPC, but we warn
		}
		return map[string]interface{}{
			"content": []map[string]string{
				{
					"type": "text",
					"text": fmt.Sprintf(
						"Index refreshed and saved to disk. Files: %d, Go: %d, Py: %d",
						ctx.Stats.Files,
						ctx.Stats.GoFiles,
						ctx.Stats.PyFiles,
					),
				},
			},
		}, nil
	case "get_summary":
		if err := ensureContext(); err != nil {
			return nil, err
		}

		// Return a summarized text
		text := fmt.Sprintf(
			"Project Root: %s\nStats: %+v\nStructure (Top Level):\n",
			lastContext.Root,
			lastContext.Stats,
		)
		for _, node := range lastContext.Structure {
			text += fmt.Sprintf("- %s (%s)\n", node.Name, node.Type)
		}

		return map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": text},
			},
		}, nil
	default:
		return nil, &RPCError{Code: -32601, Message: "Tool not found"}
	}
}

func handleResourceRead(params json.RawMessage) (interface{}, *RPCError) {
	var read struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &read); err != nil {
		return nil, &RPCError{Code: -32700, Message: "Parse error"}
	}

	if read.URI == "context://summary" {
		if err := ensureContext(); err != nil {
			return nil, err
		}

		bytes, err := json.MarshalIndent(lastContext, "", "  ")
		if err != nil {
			return nil, &RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("Internal error: failed to marshal context: %v", err),
			}
		}
		return map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      "context://summary",
					"mimeType": "application/json",
					"text":     string(bytes),
				},
			},
		}, nil
	}
	return nil, &RPCError{Code: -32602, Message: "Invalid params"}
}

func ensureContext() *RPCError {
	if lastContext == nil {
		// Try to load from disk first
		if ctx, err := indexer.LoadContext(projectRoot); err == nil {
			lastContext = ctx
			log.Printf("Context lazily loaded from disk")
			return nil
		}

		ctx, err := indexer.ScanProject(projectRoot)
		if err != nil {
			return &RPCError{Code: 1, Message: err.Error()}
		}
		lastContext = ctx
		indexer.SaveContext(projectRoot, ctx) // Try to save
	}
	return nil
}
