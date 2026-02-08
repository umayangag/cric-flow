package main

import (
	"encoding/json"
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
