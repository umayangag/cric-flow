package openmeteo

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Client holds shared http client, rate limiting and retry config.
type Client struct {
	HTTP     *http.Client
	Rate     int // requests per second (min 1)
	MaxRetry int // max attempts per request (>=1)
	mu       sync.Mutex
	lastTick time.Time
}

func NewClient(ratePerSec, maxRetry int) *Client {
	if ratePerSec <= 0 {
		ratePerSec = 1
	}
	if maxRetry <= 0 {
		maxRetry = 3
	}
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}, Rate: ratePerSec, MaxRetry: maxRetry}
}

// throttle enforces a simple spacing between requests based on Rate.
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	minGap := time.Second / time.Duration(c.Rate)
	now := time.Now()
	gap := c.lastTick.Add(minGap).Sub(now)
	if gap > 0 {
		t := time.NewTimer(gap)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	c.lastTick = time.Now()
	return nil
}
