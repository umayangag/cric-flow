// Package std provides a thin wrapper around net/http.Client that implements httpx.HTTPClient.
package std

import (
	"net/http"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/httpx"
)

// Client wraps a *http.Client to satisfy httpx.HTTPClient.
type Client struct{ Client *http.Client }

// Ensure implementation
var _ httpx.HTTPClient = (*Client)(nil)

// New returns an httpx.HTTPClient backed by the provided *http.Client.
func New(c *http.Client) *Client { return &Client{Client: c} }

// Do delegates to the underlying http.Client.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}
