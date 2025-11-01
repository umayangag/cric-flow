package openmeteo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type GeocodeResult struct {
	Name       string
	Country    string
	Latitude   float64
	Longitude  float64
	Timezone   string
	Confidence float32
}

type geocodeResp struct {
	Results []struct {
		Name       string  `json:"name"`
		Country    string  `json:"country"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Timezone   string  `json:"timezone"`
		Elevation  float64 `json:"elevation"`
		Feature    string  `json:"feature_code"`
		Rank       float32 `json:"rank"`
		Admin1     string  `json:"admin1"`
		Admin2     string  `json:"admin2"`
		Admin3     string  `json:"admin3"`
		Admin4     string  `json:"admin4"`
	} `json:"results"`
}

// Resolve queries Open‑Meteo Geocoding API and returns the best match using simple heuristics.
func (c *Client) Resolve(ctx context.Context, name, city, country string) (*GeocodeResult, error) {
	if err := c.throttle(ctx); err != nil { return nil, err }
	q := strings.TrimSpace(name)
	if city != "" { q += ", " + city }
	if country != "" { q += ", " + country }
	params := url.Values{}
	params.Set("name", q)
	params.Set("count", "10")
	params.Set("language", "en")
	params.Set("format", "json")
	u := "https://geocoding-api.open-meteo.com/v1/search?" + params.Encode()
	var lastErr error
	var bestRes *GeocodeResult
	for attempt := 1; attempt <= c.MaxRetry; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "cric-info-scrapers/1.0")
		resp, err := c.HTTP.Do(req)
		if err != nil { lastErr = err; continue }
		func() {
			defer resp.Body.Close()
			if resp.StatusCode >= 500 { lastErr = fmt.Errorf("geocode 5xx: %d", resp.StatusCode); return }
			if resp.StatusCode != 200 { lastErr = fmt.Errorf("geocode status: %d", resp.StatusCode); return }
			b, _ := io.ReadAll(resp.Body)
			var gr geocodeResp
			if err := json.Unmarshal(b, &gr); err != nil { lastErr = err; return }
			if len(gr.Results) == 0 { lastErr = errors.New("no geocode results"); return }
			// Heuristic: prefer exact-ish country match if provided, else highest rank
			bestIdx := 0
			bestRank := float32(-1)
			for i, r := range gr.Results {
				if country != "" && strings.EqualFold(country, r.Country) {
					if r.Rank > bestRank { bestRank = r.Rank; bestIdx = i }
					continue
				}
				if country == "" && r.Rank > bestRank { bestRank = r.Rank; bestIdx = i }
			}
			r := gr.Results[bestIdx]
			lastErr = nil
			res := &GeocodeResult{ Name: r.Name, Country: r.Country, Latitude: r.Latitude, Longitude: r.Longitude, Timezone: r.Timezone, Confidence: r.Rank }
			lastErr = nil
			bestRes = res
		}()
		if lastErr == nil {
			return bestRes, nil
		}
	}
	return nil, lastErr
}
