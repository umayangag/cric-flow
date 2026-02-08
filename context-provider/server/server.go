package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

type Server struct {
	ProjectRoot string
	LastContext *indexer.ProjectContext
	mu          sync.RWMutex
}

func NewServer(root string) *Server {
	return &Server{
		ProjectRoot: root,
	}
}

func (s *Server) LoadContext() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize context (Load from disk or Scan)
	if ctx, err := indexer.LoadContext(s.ProjectRoot); err == nil {
		s.LastContext = ctx
		log.Printf("Context loaded from disk (%d files)", ctx.Stats.Files)
	} else {
		log.Printf("No existing context found or load failed: %v. Scanning now...", err)
		// Perform initial scan
		ctx, err := indexer.ScanProject(s.ProjectRoot)
		if err != nil {
			log.Printf("Initial scan failed: %v", err)
			return err
		}

		s.LastContext = ctx
		if err := indexer.SaveContext(s.ProjectRoot, ctx); err != nil {
			log.Printf("Failed to save context to disk: %v", err)
		} else {
			log.Printf("Context scanned and saved to disk.")
		}
	}
	return nil
}

func (s *Server) HandleRequest(w io.Writer, req *Request) {
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
		res, err = s.handleToolCall(req.Params)
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
		res, err = s.handleResourceRead(req.Params)
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
		s.sendResponse(w, req.ID, res, err)
	}
}

func (s *Server) handleToolCall(params json.RawMessage) (interface{}, *RPCError) {
	var call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &RPCError{Code: -32700, Message: "Parse error"}
	}

	switch call.Name {
	case "refresh_index":
		ctx, err := indexer.ScanProject(s.ProjectRoot)
		if err != nil {
			return nil, &RPCError{Code: 1, Message: err.Error()}
		}
		s.mu.Lock()
		s.LastContext = ctx
		s.mu.Unlock()

		if err := indexer.SaveContext(s.ProjectRoot, ctx); err != nil {
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
		if err := s.ensureContext(); err != nil {
			return nil, err
		}

		s.mu.RLock()
		ctx := s.LastContext
		s.mu.RUnlock()

		// Return a summarized text
		var textBuilder strings.Builder
		fmt.Fprintf(&textBuilder, "Project Root: %s\nStats: %+v\nStructure (Top Level):\n",
			ctx.Root,
			ctx.Stats,
		)
		for _, node := range ctx.Structure {
			fmt.Fprintf(&textBuilder, "- %s (%s)\n", node.Name, node.Type)
		}
		text := textBuilder.String()

		return map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": text},
			},
		}, nil
	default:
		return nil, &RPCError{Code: -32601, Message: "Tool not found"}
	}
}

func (s *Server) handleResourceRead(params json.RawMessage) (interface{}, *RPCError) {
	var read struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &read); err != nil {
		return nil, &RPCError{Code: -32700, Message: "Parse error"}
	}

	if read.URI == "context://summary" {
		if err := s.ensureContext(); err != nil {
			return nil, err
		}

		s.mu.RLock()
		ctx := s.LastContext
		s.mu.RUnlock()

		jsonBytes, err := json.MarshalIndent(ctx, "", "  ")
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
					"text":     string(jsonBytes),
				},
			},
		}, nil
	}
	return nil, &RPCError{Code: -32602, Message: "Invalid params"}
}

func (s *Server) ensureContext() *RPCError {
	s.mu.RLock()
	if s.LastContext != nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double check
	if s.LastContext != nil {
		return nil
	}

	// Try to load from disk first
	if ctx, err := indexer.LoadContext(s.ProjectRoot); err == nil {
		s.LastContext = ctx
		log.Printf("Context lazily loaded from disk")
		return nil
	}

	ctx, err := indexer.ScanProject(s.ProjectRoot)
	if err != nil {
		return &RPCError{Code: 1, Message: err.Error()}
	}
	s.LastContext = ctx
	if err := indexer.SaveContext(s.ProjectRoot, ctx); err != nil {
		log.Printf("Failed to save lazy context: %v", err)
	}
	return nil
}

func (s *Server) sendResponse(w io.Writer, id *json.RawMessage, result interface{}, rpcErr *RPCError) {
	response := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   rpcErr,
	}
	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error: failed to marshal response for request %v: %v", id, err)
		// Attempt to send a valid JSON-RPC error response back to the client.
		errResponse := Response{
			JSONRPC: "2.0",
			ID:      id,
			Error: &RPCError{
				Code:    -32603, // Internal error
				Message: fmt.Sprintf("Internal error: failed to marshal response: %v", err),
			},
		}
		errorBytes, _ := json.Marshal(errResponse)
		fmt.Fprintf(w, "%s\n", errorBytes)
		return
	}
	fmt.Fprintf(w, "%s\n", responseBytes)
}

func FindProjectRoot(wd string) string {
	// Logic: find go.work to determine project root by walking up from the current directory.
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir { // Reached filesystem root
			return wd // Fallback to current directory
		}
		dir = parent
	}
}
