package std

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Do_DelegatesToUnderlying(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusNoContent)
		_, _ = io.WriteString(w, "")
	}))
	defer ts.Close()

	underlying := ts.Client() // configured for this test server
	c := New(underlying)

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want status 204 got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Test") != "ok" {
		t.Fatalf("expected header X-Test=ok got %q", resp.Header.Get("X-Test"))
	}
}
