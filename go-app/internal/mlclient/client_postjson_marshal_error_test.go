package mlclient

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

type badJSON struct{}

func (badJSON) MarshalJSON() ([]byte, error) { return nil, errors.New("marshal boom") }

// Directly exercise postJSON's marshal error branch.
func TestPostJSON_MarshalError(t *testing.T) {
	c := newTestClient("http://invalid", httptest.NewServer(nil).Client())
	// No server request should be made because marshal fails first.
	var out any
	err := c.postJSON(context.Background(), "/unused", badJSON{}, &out)
	if err == nil {
		t.Fatalf("expected marshal error, got nil")
	}
}
