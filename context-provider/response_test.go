package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResponseMarshalling(t *testing.T) {
	// Test successful marshalling
	t.Run("Success", func(t *testing.T) {
		res := Response{
			JSONRPC: "2.0",
			Result:  map[string]string{"foo": "bar"},
		}
		_, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	// Test failure
	t.Run("Failure", func(t *testing.T) {
		// Create a channel, which is not marshallable
		res := Response{
			JSONRPC: "2.0",
			Result:  make(chan int),
		}
		_, err := json.Marshal(res)
		if err == nil {
			t.Fatal("Expected error for unmarshallable type, got nil")
		}
	})
}

func TestSendResponse_MarshalingError(t *testing.T) {
	// Setup
	var buf bytes.Buffer
	idRaw := json.RawMessage(`1`)

	// Create a channel, which cannot be marshaled to JSON
	unmarshallableResult := make(chan int)

	// Call sendResponse
	sendResponse(&buf, &idRaw, unmarshallableResult, nil)

	output := buf.String()

	// Verify we got a JSON-RPC error response
	if output == "" {
		t.Fatal("Expected error response, got empty output")
	}

	if !strings.Contains(output, "-32603") {
		t.Errorf("Expected error code -32603, got: %s", output)
	}
	if !strings.Contains(output, "failed to marshal response") {
		t.Errorf("Expected error message about marshaling failure, got: %s", output)
	}
}

func TestSendResponse_Success(t *testing.T) {
	// Setup
	var buf bytes.Buffer
	idRaw := json.RawMessage(`1`)
	result := map[string]string{"status": "ok"}

	// Call sendResponse
	sendResponse(&buf, &idRaw, result, nil)

	output := buf.String()

	// Verify we got a valid JSON-RPC response
	if output == "" {
		t.Fatal("Expected response, got empty output")
	}

	var resp Response
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.ID == nil {
		t.Error("Expected ID in response")
	}
	if string(*resp.ID) != "1" {
		t.Errorf("Expected ID 1, got %s", string(*resp.ID))
	}
	
	// Check result
	resMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be a map")
	}
	if resMap["status"] != "ok" {
		t.Errorf("Expected status ok, got %v", resMap["status"])
	}
	
	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
}

func TestHandleRequest(t *testing.T) {
	// Test ping
	t.Run("Ping", func(t *testing.T) {
		var buf bytes.Buffer
		idRaw := json.RawMessage(`1`)
		req := Request{
			JSONRPC: "2.0",
			ID:      &idRaw,
			Method:  "ping",
		}
		
		handleRequest(&buf, &req)
		
		output := buf.String()
		if output == "" {
			t.Fatal("Expected response, got empty output")
		}
		
		var resp Response
		if err := json.Unmarshal([]byte(output), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		
		if resp.Error != nil {
			t.Errorf("Expected no error, got %v", resp.Error)
		}
		
		// Check if result is empty map
		resMap, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatal("Expected result to be a map")
		}
		if len(resMap) != 0 {
			t.Errorf("Expected empty result, got %v", resMap)
		}
	})

	// Test initialize
	t.Run("Initialize", func(t *testing.T) {
		var buf bytes.Buffer
		idRaw := json.RawMessage(`2`)
		req := Request{
			JSONRPC: "2.0",
			ID:      &idRaw,
			Method:  "initialize",
		}
		
		handleRequest(&buf, &req)
		
		output := buf.String()
		var resp Response
		if err := json.Unmarshal([]byte(output), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		
		resMap, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatal("Expected result to be a map")
		}
		
		if resMap["protocolVersion"] != "2024-11-05" {
			t.Errorf("Expected protocolVersion 2024-11-05, got %v", resMap["protocolVersion"])
		}
	})
	
	// Test unknown method
	t.Run("UnknownMethod", func(t *testing.T) {
		var buf bytes.Buffer
		idRaw := json.RawMessage(`3`)
		req := Request{
			JSONRPC: "2.0",
			ID:      &idRaw,
			Method:  "unknown/method",
		}
		
		handleRequest(&buf, &req)
		
		output := buf.String()
		var resp Response
		if err := json.Unmarshal([]byte(output), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		
		if resp.Error == nil {
			t.Fatal("Expected error, got nil")
		}
		if resp.Error.Code != -32601 {
			t.Errorf("Expected error code -32601, got %d", resp.Error.Code)
		}
	})
}
