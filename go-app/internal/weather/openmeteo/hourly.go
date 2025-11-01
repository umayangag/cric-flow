package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type HourlyRecord struct {
	Time      time.Time
	TempC     *int
	FeelsC    *int // optional, if requested
	WindKmh   *int
	GustKmh   *int
	RainMm    *int
	Humidity  *int
	Cloud     *int
	Pressure  *int // hPa
}

type hourlyResp struct {
	Hourly struct {
		Time                []string  `json:"time"`
		Temperature2m       []float64 `json:"temperature_2m"`
		ApparentTemperature []float64 `json:"apparent_temperature"`
		RelativeHumidity2m  []int     `json:"relative_humidity_2m"`
		Precipitation       []float64 `json:"precipitation"`
		CloudCover          []int     `json:"cloud_cover"`
		PressureMsl         []float64 `json:"pressure_msl"`
		WindSpeed10m        []float64 `json:"wind_speed_10m"`
		WindGusts10m        []float64 `json:"wind_gusts_10m"`
	} `json:"hourly"`
}

// Hourly fetches hourly weather from Open‑Meteo Archive API for [from,to] (inclusive days) in given timezone string.
func (c *Client) Hourly(ctx context.Context, lat, lon float64, from, to time.Time, tz string) ([]HourlyRecord, error) {
	if err := c.throttle(ctx); err != nil { return nil, err }
	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%f", lat))
	params.Set("longitude", fmt.Sprintf("%f", lon))
	params.Set("start_date", from.Format("2006-01-02"))
	params.Set("end_date", to.Format("2006-01-02"))
	params.Set("timezone", tz)
	params.Set("hourly", "temperature_2m,apparent_temperature,relative_humidity_2m,precipitation,cloud_cover,pressure_msl,wind_speed_10m,wind_gusts_10m")
	u := "https://archive-api.open-meteo.com/v1/archive?" + params.Encode()
	var lastErr error
	var recs []HourlyRecord
	for attempt := 1; attempt <= c.MaxRetry; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "cric-info-scrapers/1.0")
		resp, err := c.HTTP.Do(req)
		if err != nil { lastErr = err; continue }
		func() {
			defer resp.Body.Close()
			if resp.StatusCode >= 500 { lastErr = fmt.Errorf("hourly 5xx: %d", resp.StatusCode); return }
			if resp.StatusCode != 200 { lastErr = fmt.Errorf("hourly status: %d", resp.StatusCode); return }
			b, _ := io.ReadAll(resp.Body)
			var hr hourlyResp
			if err := json.Unmarshal(b, &hr); err != nil { lastErr = err; return }
			n := len(hr.Hourly.Time)
			out := make([]HourlyRecord, 0, n)
			for i := 0; i < n; i++ {
				ts, _ := time.Parse(time.RFC3339, hr.Hourly.Time[i])
				var t, feels, wind, gust, rain, hum, cloud, press *int
				if i < len(hr.Hourly.Temperature2m) { v := int(hr.Hourly.Temperature2m[i] + 0.5); t = &v }
				if i < len(hr.Hourly.ApparentTemperature) { v := int(hr.Hourly.ApparentTemperature[i] + 0.5); feels = &v }
				if i < len(hr.Hourly.WindSpeed10m) { v := int(hr.Hourly.WindSpeed10m[i] + 0.5); wind = &v }
				if i < len(hr.Hourly.WindGusts10m) { v := int(hr.Hourly.WindGusts10m[i] + 0.5); gust = &v }
				if i < len(hr.Hourly.Precipitation) { v := int(hr.Hourly.Precipitation[i] + 0.5); rain = &v }
				if i < len(hr.Hourly.RelativeHumidity2m) { v := hr.Hourly.RelativeHumidity2m[i]; hum = &v }
				if i < len(hr.Hourly.CloudCover) { v := hr.Hourly.CloudCover[i]; cloud = &v }
				if i < len(hr.Hourly.PressureMsl) { v := int(hr.Hourly.PressureMsl[i] + 0.5); press = &v }
				out = append(out, HourlyRecord{ Time: ts, TempC: t, FeelsC: feels, WindKmh: wind, GustKmh: gust, RainMm: rain, Humidity: hum, Cloud: cloud, Pressure: press })
			}
			lastErr = nil
			recs = out
		}()
		if lastErr == nil {
			return recs, nil
		}
	}
	return nil, lastErr
}
