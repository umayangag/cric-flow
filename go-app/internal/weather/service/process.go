package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/weather/openmeteo"
)

// ProcessResult describes the outcome of processing a weather job.
type ProcessResult struct {
	MatchID     int64
	VenueNorm   string
	Lat, Lon    float64
	Timezone    string
	Sessions    int
	Upserts     int
	CacheOnlyHit bool
}


// ProcessJob resolves venue coordinates (DB cache or geocode), fetches hourly weather
// and upserts closest-hour records for each session in the job.
// If data cannot be fetched due to cache-only policy, returns an error so caller can reschedule.
func ProcessJob(ctx context.Context, job *db.WeatherJob, noop bool) (*ProcessResult, error) {
	if job == nil {
		return nil, errors.New("nil job")
	}
	cfg := config.Load()
	rate := 1
	maxRetry := 3
	if cfg.Weather.RateLimitPerSec > 0 { rate = cfg.Weather.RateLimitPerSec }
	if cfg.Weather.MaxAttempts > 0 { maxRetry = cfg.Weather.MaxAttempts }
	cli := openmeteo.NewClient(rate, maxRetry)

	// 1) Lookup venue
	v, err := db.LookupVenueByNormalizedName(ctx, job.NormalizedVenue)
	if err != nil {
		// Not found; if cache-only or noop, ask for reschedule
		if cfg.Weather.GeocodeCacheOnly || noop {
			return nil, fmt.Errorf("venue not cached for %s", job.NormalizedVenue)
		}
	}

	var lat, lon float64
	var tz string
	if v != nil && v.Latitude != nil && v.Longitude != nil {
		lat = *v.Latitude
		lon = *v.Longitude
		if v.Timezone != nil { tz = *v.Timezone }
	}

	// 2) Geocode if missing and allowed
	if (lat == 0 && lon == 0) && !(cfg.Weather.GeocodeCacheOnly || noop) {
		city := ""
		country := ""
		if job.City != nil { city = *job.City }
		if job.Country != nil { country = *job.Country }
		res, err := cli.Resolve(ctx, job.NormalizedVenue, city, country)
		if err != nil {
			return nil, fmt.Errorf("geocode failed: %w", err)
		}
		lat, lon = res.Latitude, res.Longitude
		tz = res.Timezone
		// Upsert venue geocode
		var disp string
		if v != nil && v.DisplayName != "" { disp = v.DisplayName } else { disp = job.NormalizedVenue }
		_, _ = db.UpsertVenueGeocode(ctx, job.NormalizedVenue, disp, job.City, job.Country, lat, lon, tz, "open-meteo-geocoding", res.Confidence)
	}

	if lat == 0 && lon == 0 {
		// Still no coordinates: reschedule
		return nil, fmt.Errorf("no coordinates for venue %s (cache-only=%v noop=%v)", job.NormalizedVenue, cfg.Weather.GeocodeCacheOnly, noop)
	}
	if tz == "" { tz = "auto" }

	// 3) Determine date range
	from := time.Now()
	if d, err := db.GetMatchDate(ctx, job.MatchID); err == nil && d != nil {
		from = *d
	}
	to := from

	// 4) Fetch hourly series (skip if noop to minimize API calls)
	var series []openmeteo.HourlyRecord
	if !noop {
		series, err = cli.Hourly(ctx, lat, lon, from, to, tz)
		if err != nil {
			return nil, fmt.Errorf("hourly fetch failed: %w", err)
		}
	}

	// 5) Decode sessions and select nearest hour
	var sess []sessionSpec
	_ = json.Unmarshal(job.SessionsJSON, &sess)
	if len(sess) == 0 {
		// default to two innings
		sess = []sessionSpec{{Label: "inning1"}, {Label: "inning2"}}
	}
	upserts := 0
	for _, s := range sess {
		// Pick a default local time if missing: 12:00
		at := s.At
		if at == nil {
			def := from.Add(12 * time.Hour)
			at = &def
		}
		var chosen *openmeteo.HourlyRecord
		if len(series) > 0 {
			chosen = nearest(series, *at)
		}
		// Map to db.Weather; if noop or no series, upsert placeholders (nil values) to ensure row exists
		var w db.Weather
		w.MatchID = job.MatchID
		w.Session = s.Label
		if chosen != nil {
			w.Temp = chosen.TempC
			w.Feels = chosen.FeelsC
			w.Wind = chosen.WindKmh
			w.Gust = chosen.GustKmh
			w.Rain = chosen.RainMm
			w.Humidity = chosen.Humidity
			w.Cloud = chosen.Cloud
			w.Pressure = chosen.Pressure
		}
		if err := db.UpsertWeather(ctx, &w); err != nil {
			return nil, fmt.Errorf("upsert weather failed: %w", err)
		}
		upserts++
	}

	return &ProcessResult{MatchID: job.MatchID, VenueNorm: job.NormalizedVenue, Lat: lat, Lon: lon, Timezone: tz, Sessions: len(sess), Upserts: upserts, CacheOnlyHit: noop || cfg.Weather.GeocodeCacheOnly}, nil
}

func nearest(series []openmeteo.HourlyRecord, t time.Time) *openmeteo.HourlyRecord {
	if len(series) == 0 { return nil }
	bestIdx := 0
	bestDur := absDuration(series[0].Time.Sub(t))
	for i := 1; i < len(series); i++ {
		d := absDuration(series[i].Time.Sub(t))
		if d < bestDur {
			bestDur = d
			bestIdx = i
		}
	}
	return &series[bestIdx]
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 { return -d }
	return d
}
