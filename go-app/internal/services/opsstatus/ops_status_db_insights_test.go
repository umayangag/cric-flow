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
