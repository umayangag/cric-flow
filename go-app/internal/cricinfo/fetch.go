//go:build legacy_cricinfo
package cricinfo

import (
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

// Client is a small HTTP client wrapper with retry/backoff and optional rate limiting.
type Client struct {
	HTTP      *http.Client
	Limiter   <-chan time.Time // if non-nil, receive before each request (rate limiting)
	UserAgent string
	MaxRetry  int
	Backoff   func(attempt int) time.Duration
}

// NewClient returns a Client with sane defaults.
func NewClient() *Client {
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &Client{
		HTTP:     &http.Client{Transport: tr, Timeout: 20 * time.Second},
		Limiter:  time.Tick(250 * time.Millisecond), // ~4 req/s
		MaxRetry: 3,
		Backoff: func(attempt int) time.Duration {
			if attempt <= 0 {
				return 0
			}
			// exponential backoff with jitter-like floor
			d := time.Duration(250*attempt) * time.Millisecond
			if d > 2*time.Second {
				d = 2 * time.Second
			}
			return d
		},
		UserAgent: "cric-app-scraper (+github.com/umayangag/cric-app)",
	}
}

// Get fetches a URL with retry/backoff. It returns the response body as a ReadCloser; caller must Close.
func (c *Client) Get(url string) (io.ReadCloser, error) {
	if c.HTTP == nil {
		c = NewClient()
	}
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetry; attempt++ {
		if c.Limiter != nil {
			<-c.Limiter
		}
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		if c.UserAgent != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(c.Backoff(attempt))
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp.Body, nil
		}
		lastErr = errors.New(resp.Status)
		// drain and close to reuse connection
		_ = resp.Body.Close()
		time.Sleep(c.Backoff(attempt))
	}
	return nil, lastErr
}
