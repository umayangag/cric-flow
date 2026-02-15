package db

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// fakeRow implements rowScanner and writes preset values into pointers on Scan.
type fakeRow struct{ scan func(dest ...any) error }

func (f fakeRow) Scan(dest ...any) error { return f.scan(dest...) }

// fakeQuerier implements simpleQuerier and returns configured fake rows depending on the SQL.
type fakeQuerier struct {
	// record last passed SQL and args for assertions
	lastSQL  string
	lastArgs []any

	// knobs to control returned values
	// player table
	battingConsistency float64
	hasBatCons         bool
	bowlingConsistency float64
	hasBowlCons        bool

	// player_form_data (precomputed)
	preBatForm  float64
	hasPreBat   bool
	preBowlForm float64
	hasPreBowl  bool

	// fallbacks (averages up to cutoff)
	avgRuns    float64
	hasAvgRuns bool
	avgWkts    float64
	hasAvgWkts bool
	avgEcon    float64
	hasAvgEcon bool
}

func (q *fakeQuerier) QueryRow(_ context.Context, sql string, args ...any) rowScanner {
	q.lastSQL = sql
	q.lastArgs = args
	switch {
	case contains(sql, "FROM player WHERE id"):
		// return batting_consistency, bowling_consistency
		return fakeRow{scan: func(dest ...any) error {
			// dest[0], dest[1] are *sql.NullFloat64, but we can mimic by writing via helper
			// We can't import database/sql here to set NullFloat64 fields properly. Instead,
			// rely on the provider treating non-Valid values when Scan error occurs. To keep
			// behavior simple, we assign zero values only when flags set by using a small trick:
			// The tests will mainly assert presence of precomputed/fallback keys; consistency is smoke.
			return writeNullableFloat(dest, q.hasBatCons, q.battingConsistency, q.hasBowlCons, q.bowlingConsistency)
		}}
	case contains(sql, "FROM player_form_data"):
		return fakeRow{scan: func(dest ...any) error {
			return writeNullableFloat(dest, q.hasPreBat, q.preBatForm, q.hasPreBowl, q.preBowlForm)
		}}
	case contains(sql, "FROM batting_data bd"):
		return fakeRow{scan: func(dest ...any) error {
			return writeSingleNullableFloat(dest, q.hasAvgRuns, q.avgRuns)
		}}
	case contains(sql, "FROM bowling_data bw"):
		return fakeRow{scan: func(dest ...any) error {
			// two floats: wickets, econ
			return writeDoubleNullableFloat(dest, q.hasAvgWkts, q.avgWkts, q.hasAvgEcon, q.avgEcon)
		}}
	default:
		return fakeRow{scan: func(_ ...any) error { return nil }}
	}
}

// contains is a minimal substring check (avoids strings import to keep test compact).
func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	// naive search sufficient for tests
	L, l := len(s), len(sub)
	if l == 0 {
		return 0
	}
	for i := 0; i <= L-l; i++ {
		if s[i:i+l] == sub {
			return i
		}
	}
	return -1
}

// Helpers to assign into sql.NullFloat64-like structs via reflection.
// We avoid importing database/sql; we just set fields if present.
func setNullFloat64(ptr any, valid bool, val float64) {
	rv := reflect.ValueOf(ptr)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	v := rv.Elem()
	// Expect a struct with fields Float64 (float64) and Valid (bool)
	f := v.FieldByName("Float64")
	ok1 := f.IsValid() && f.CanSet()
	g := v.FieldByName("Valid")
	ok2 := g.IsValid() && g.CanSet()
	if ok1 && ok2 {
		if valid {
			f.SetFloat(val)
			g.SetBool(true)
		} else {
			// leave zero and set Valid=false
			g.SetBool(false)
		}
	}
}

func writeNullableFloat(dest []any, v1 bool, f1 float64, v2 bool, f2 float64) error {
	if len(dest) >= 1 {
		setNullFloat64(dest[0], v1, f1)
	}
	if len(dest) >= 2 {
		setNullFloat64(dest[1], v2, f2)
	}
	return nil
}

func writeSingleNullableFloat(dest []any, v bool, f float64) error {
	if len(dest) >= 1 {
		setNullFloat64(dest[0], v, f)
	}
	return nil
}

func writeDoubleNullableFloat(dest []any, v1 bool, f1 float64, v2 bool, f2 float64) error {
	if len(dest) >= 1 {
		setNullFloat64(dest[0], v1, f1)
	}
	if len(dest) >= 2 {
		setNullFloat64(dest[1], v2, f2)
	}
	return nil
}

func TestFallbackAggregates_PopulateFormAndAux(t *testing.T) {
	// Arrange: swap in fake querier and restore after (no precomputed tables; aggregate-only)
	orig := featureQuerier
	t.Cleanup(func() { featureQuerier = orig })

	fq := &fakeQuerier{
		avgRuns: 10.0, hasAvgRuns: true,
		avgWkts: 1.0, hasAvgWkts: true,
		avgEcon: 7.0, hasAvgEcon: true,
	}
	featureQuerier = fq

	cutoff := time.Date(2024, 10, 30, 0, 0, 0, 0, time.UTC)
	p := &DefaultFeatureProvider{}
	got, err := p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, []int64{1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	feats := got[1]
	if feats == nil {
		t.Fatalf("missing features for player 1")
	}
	// batting_form/ bowling_form come from aggregate fallbacks (avg_runs, avg_wickets)
	if feats["batting_form"] != 10.0 {
		t.Fatalf("batting_form = %v, want 10", feats["batting_form"])
	}
	if feats["bowling_form"] != 1.0 {
		t.Fatalf("bowling_form = %v, want 1", feats["bowling_form"])
	}
	if feats["avg_runs"] != 10.0 {
		t.Fatalf("avg_runs = %v, want 10", feats["avg_runs"])
	}
}

func TestCutoffArgs_PropagatedToFallbackQueries(t *testing.T) {
	// Arrange
	orig := featureQuerier
	t.Cleanup(func() { featureQuerier = orig })

	fq := &fakeQuerier{
		hasAvgRuns: true, avgRuns: 20.0,
		hasAvgWkts: true, avgWkts: 2.0,
		hasAvgEcon: true, avgEcon: 6.5,
	}
	featureQuerier = fq

	cutoff := time.Date(2024, 11, 5, 9, 0, 0, 0, time.UTC)
	p := &DefaultFeatureProvider{}
	_, err := p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, []int64{7})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// The lastArgs should end with the cutoff for the last query; ensure at least one call used this cutoff
	// Since fakeQuerier updates lastArgs per call, we at least assert a time.Time was passed and equals cutoff
	if len(fq.lastArgs) == 0 {
		t.Fatalf("expected fallback queries to be invoked")
	}
	// find any time.Time in args and compare
	var found time.Time
	for _, a := range fq.lastArgs {
		if tt, ok := a.(time.Time); ok {
			found = tt
			break
		}
	}
	if found.IsZero() || !found.Equal(cutoff) {
		t.Fatalf("cutoff arg not propagated: got %v want %v", found, cutoff)
	}
}

func TestPartialData_ReturnsPartialMaps(t *testing.T) {
	orig := featureQuerier
	t.Cleanup(func() { featureQuerier = orig })

	// No precomputed, only bowling econ present; batting avg missing
	fq := &fakeQuerier{hasAvgEcon: true, avgEcon: 7.5}
	featureQuerier = fq

	cutoff := time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC)
	p := &DefaultFeatureProvider{}
	got, err := p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, []int64{99})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	feats := got[99]
	if feats == nil {
		t.Fatalf("missing features for player 99")
	}
	if _, ok := feats["avg_economy"]; !ok {
		t.Fatalf("expected avg_economy present")
	}
	if _, ok := feats["batting_form"]; ok {
		t.Fatalf("did not expect batting_form present")
	}
}
