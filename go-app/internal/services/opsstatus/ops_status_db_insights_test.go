package opsstatus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- Test fakes ----
type fakeInsightsProbe struct {
	latestByFmt map[string]time.Time
	latestErr   map[string]error
	countByFmt  map[string]int64
	countErr    map[string]error
}

func (f fakeInsightsProbe) LatestMatchDateByFormat(_ context.Context, format string) (time.Time, error) {
	if f.latestErr != nil {
		if err, ok := f.latestErr[format]; ok {
			return time.Time{}, err
		}
	}
	if t, ok := f.latestByFmt[format]; ok {
		return t, nil
	}
	return time.Time{}, nil
}

func (f fakeInsightsProbe) CountMatchesByFormat(_ context.Context, format string) (int64, error) {
	if n, ok := f.countByFmt[format]; ok {
		return n, nil
	}
	return 0, nil
}

func (f fakeInsightsProbe) CountMatchesSinceByFormat(_ context.Context, format string, _ time.Time) (int64, error) {
	if f.countErr != nil {
		if err, ok := f.countErr[format]; ok {
			return 0, err
		}
	}
	if n, ok := f.countByFmt[format]; ok {
		return n, nil
	}
	return 0, nil
}

// ---- Tests ----

func TestBuildDBFreshnessSection_Table(t *testing.T) {
	now := time.Date(2026, 1, 24, 12, 0, 0, 0, time.UTC)
	formats := CricketFormatCodes

	mk := func(deltas map[string]int) fakeInsightsProbe {
		m := map[string]time.Time{}
		for _, f := range formats {
			if days, ok := deltas[f]; ok {
				m[f] = now.AddDate(0, 0, -days)
			}
		}
		return fakeInsightsProbe{latestByFmt: m}
	}

	testCases := []struct {
		name   string
		probe  fakeInsightsProbe
		wantSt map[string]string // per-format status
		wantOv string
	}{
		{
			name:   "all_ok_within_7_days",
			probe:  mk(map[string]int{"TEST": 0, "ODI": 3, "T20I": 7, "T20": 1}),
			wantSt: map[string]string{"TEST": "ok", "ODI": "ok", "T20I": "ok", "T20": "ok"},
			wantOv: "ok",
		},
		{
			name:   "stale_when_8_to_30_days",
			probe:  mk(map[string]int{"TEST": 8, "ODI": 15, "T20I": 30, "T20": 1}),
			wantSt: map[string]string{"TEST": "stale", "ODI": "stale", "T20I": "stale", "T20": "ok"},
			wantOv: "stale",
		},
		{
			name:   "missing_when_over_30_or_none",
			probe:  mk(map[string]int{"TEST": 31}),
			wantSt: map[string]string{"TEST": "missing", "ODI": "missing", "T20I": "missing", "T20": "missing"},
			wantOv: "missing",
		},
		{
			name: "unknown_on_errors",
			probe: func() fakeInsightsProbe {
				p := mk(nil)
				p.latestErr = map[string]error{"TEST": assertErr{}}
				return p
			}(),
			wantSt: map[string]string{"TEST": "unknown", "ODI": "missing", "T20I": "missing", "T20": "missing"},
			wantOv: "unknown",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got := BuildDBFreshnessSection(context.Background(), tc.probe, now)
			// validate structure
			fm, ok := got["formats"].(map[string]any)
			require.True(t, ok)
			// per-format statuses
			for k, want := range tc.wantSt {
				m, ok := fm[k].(map[string]any)
				require.True(t, ok)
 			st := m["status"].(string)
 				require.Equal(t, want, st)
			}
			// overall
				ov, ok := got["overall"].(map[string]any)
				require.True(t, ok, "overall missing")
				require.Equal(t, tc.wantOv, ov["status"].(string))
		})
	}
}

func TestBuildDBCompletenessSection_Table(t *testing.T) {
	now := time.Date(2026, 1, 24, 12, 0, 0, 0, time.UTC)
	p := fakeInsightsProbe{
		countByFmt: map[string]int64{
			"TEST": 2,
			"ODI":  1,
			"T20I": 0,
			"T20":  5,
		},
	}
	got := BuildDBCompletenessSection(context.Background(), p, now)
	fm := got["formats"].(map[string]any)
	want := map[string]string{"TEST": "ok", "ODI": "ok", "T20I": "missing", "T20": "ok"}
	for k, w := range want {
		m := fm[k].(map[string]any)
 	st := m["status"].(string)
 	require.Equal(t, w, st)
	}
	ov := got["overall"].(map[string]any)
	require.Equal(t, "missing", ov["status"].(string))
	n, _ := ov["matches_last_30d"].(int64)
	require.Equal(t, int64(8), n)
}

func TestBuildDBCompletenessSection_ErrorUnknown(t *testing.T) {
	now := time.Date(2026, 1, 24, 12, 0, 0, 0, time.UTC)
	p := fakeInsightsProbe{
		countErr: map[string]error{"ODI": assertErr{}},
	}
	got := BuildDBCompletenessSection(context.Background(), p, now)
	fm := got["formats"].(map[string]any)
	m := fm["ODI"].(map[string]any)
	st2 := m["status"].(string)
	require.Equal(t, "unknown", st2)
}
