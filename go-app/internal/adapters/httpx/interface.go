// Package httpx defines a minimal HTTP client interface for adapters and services.
package httpx

import "net/http"

// HTTPClient is the minimal contract we rely on for making HTTP requests.
//
//go:generate mockery --name HTTPClient --output internal/mocks --case underscore
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
