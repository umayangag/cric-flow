// Package features implements leakage-free, as-of-date feature calculators.
package features

import (
	"math"
	"sort"
	"time"
)

// Innings represents a single batting or bowling performance with a timestamp.
type Innings struct {
	Date  time.Time
	Value float64 // generic scalar performance measure (e.g., runs for batting, wickets for bowling)
}

// ByDateAsc sorts innings by date ascending.
type ByDateAsc []Innings

func (a ByDateAsc) Len() int           { return len(a) }
func (a ByDateAsc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDateAsc) Less(i, j int) bool { return a[i].Date.Before(a[j].Date) }

// EWM computes an exponentially weighted mean over the provided innings assuming they are sorted by date.
// alpha in (0,1]; higher alpha gives more weight to recent innings.
// Returns (value, effectiveN) where effectiveN is the sum of weights.
func EWM(inn []Innings, alpha float64) (float64, float64) {
	if len(inn) == 0 {
		return 0, 0
	}
	if alpha <= 0 {
		alpha = 0.3
	}
	w := 0.0
	m := 0.0
	for _, in := range inn {
		w = alpha + (1-alpha)*w
		m = m + (in.Value-m)*(alpha/w) // numerically stable incremental EWM
	}
	return m, w
}

// LastNMean computes the mean of the last N innings (by date asc assumed).
func LastNMean(inn []Innings, N int) (float64, int) {
	if N <= 0 {
		N = 10
	}
	n := len(inn)
	if n == 0 {
		return 0, 0
	}
	start := 0
	if n > N {
		start = n - N
	}
	sum := 0.0
	cnt := 0
	for i := start; i < n; i++ {
		sum += inn[i].Value
		cnt++
	}
	if cnt == 0 {
		return 0, 0
	}
	return sum / float64(cnt), cnt
}

// Consistency computes a dispersion metric; by default coefficient of variation (std/mean) on the last N innings.
// Returns (consistency, n).
func Consistency(inn []Innings, N int) (float64, int) {
	if len(inn) == 0 {
		return 0, 0
	}
	// Take last N
	n := len(inn)
	start := 0
	if N > 0 && n > N {
		start = n - N
	}
	vals := make([]float64, 0, n-start)
	for i := start; i < n; i++ {
		vals = append(vals, inn[i].Value)
	}
	mean := 0.0
	for _, v := range vals {
		mean += v
	}
	mean /= float64(len(vals))
	if mean == 0 {
		// fall back to std
		mean = 1
	}
	varSum := 0.0
	for _, v := range vals {
		d := v - mean
		varSum += d * d
	}
	std := math.Sqrt(varSum / float64(len(vals)))
	cv := std / math.Max(mean, 1e-6)
	return cv, len(vals)
}

// SortAndClip sorts by date ascending and filters any entries >= cutoff (i.e., keeps strictly before cutoff).
func SortAndClip(inn []Innings, cutoff time.Time) []Innings {
	cp := make([]Innings, 0, len(inn))
	for _, in := range inn {
		if in.Date.Before(cutoff) {
			cp = append(cp, in)
		}
	}
	sort.Sort(ByDateAsc(cp))
	return cp
}
