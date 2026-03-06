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
		m += (in.Value - m) * (alpha / w) // numerically stable incremental EWM
	}
	return m, w
}

// Consistency computes a dispersion metric; by default coefficient of variation (std/mean) on the last N innings.
// Returns (consistency, n).
func Consistency(inn []Innings, n int) (float64, int) {
	if len(inn) == 0 {
		return 0, 0
	}
	// Take last n
	ln := len(inn)
	start := 0
	if n > 0 && ln > n {
		start = ln - n
	}
	vals := make([]float64, 0, ln-start)
	for i := start; i < ln; i++ {
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

// Momentum computes the slope of recent performance over the last n innings.
// Returns (slope, nUsed). Slope = (last - first) / (n-1) for last n values; 0 if n < 2.
// Positive slope = improving form.
func Momentum(inn []Innings, n int) (float64, int) {
	if len(inn) == 0 {
		return 0, 0
	}
	ln := len(inn)
	start := 0
	if n > 0 && ln > n {
		start = ln - n
	}
	vals := make([]float64, 0, ln-start)
	for i := start; i < ln; i++ {
		vals = append(vals, inn[i].Value)
	}
	if len(vals) < 2 {
		return 0, len(vals)
	}
	first, last := vals[0], vals[len(vals)-1]
	slope := (last - first) / float64(len(vals)-1)
	return slope, len(vals)
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

// RawStats holds multi-scale windowed statistics computed from an innings history.
// Used instead of formula-derived form/consistency so the ML model can learn optimal weightings.
// All fields are 0 when there is no data for that window.
type RawStats struct {
	MeanW3, MeanW5, MeanW10, MeanW20 float64
	StdW5, StdW10                    float64
	MaxW10, MinW10, MedianW10        float64
	Last1, Last2, Last3              float64
	CareerMean                       float64
	CareerCount                      int
	PctZeroW10                       float64
	TrendW5                          float64
	DaysSinceLast                    float64
	InningsInLast90D                 int
}

// ScanDest returns a slice of pointers to the struct fields, in the order they appear in the database schema.
// Used by db/exportqueries for rows.Scan so field order stays co-located with the struct.
func (rs *RawStats) ScanDest() []any {
	return []any{
		&rs.MeanW3, &rs.MeanW5, &rs.MeanW10, &rs.MeanW20, &rs.StdW5, &rs.StdW10, &rs.MaxW10, &rs.MinW10, &rs.MedianW10,
		&rs.Last1, &rs.Last2, &rs.Last3, &rs.CareerMean, &rs.CareerCount, &rs.PctZeroW10, &rs.TrendW5, &rs.DaysSinceLast, &rs.InningsInLast90D,
	}
}

// Values returns a slice of the struct's field values, in schema order.
// Used by db for Exec so arguments stay in sync with RawStats.
func (rs *RawStats) Values() []any {
	return []any{
		rs.MeanW3, rs.MeanW5, rs.MeanW10, rs.MeanW20, rs.StdW5, rs.StdW10, rs.MaxW10, rs.MinW10, rs.MedianW10,
		rs.Last1, rs.Last2, rs.Last3, rs.CareerMean, rs.CareerCount, rs.PctZeroW10, rs.TrendW5, rs.DaysSinceLast, rs.InningsInLast90D,
	}
}

// WindowedStats computes multi-scale summary statistics from innings (assumed sorted by date ascending).
// asOf is used for days_since_last and innings_in_last_90d. Returns zero-valued RawStats when inn is empty.
func WindowedStats(inn []Innings, asOf time.Time) RawStats {
	var out RawStats
	if len(inn) == 0 {
		return out
	}
	vals := make([]float64, len(inn))
	for i := range inn {
		vals[i] = inn[i].Value
	}
	n := len(vals)

	// Last 1, 2, 3 (most recent = last in sorted order)
	out.Last1 = vals[n-1]
	if n >= 2 {
		out.Last2 = vals[n-2]
	}
	if n >= 3 {
		out.Last3 = vals[n-3]
	}

	// Career
	out.CareerCount = n
	var sum float64
	for _, v := range vals {
		sum += v
	}
	out.CareerMean = sum / float64(n)

	// Days since last and innings in last 90 days.
	// DaysSinceLast is fractional days (e.g. 2.5 = 2 days 12 hours); ML can use as-is or round if whole days are preferred.
	lastDate := inn[n-1].Date
	out.DaysSinceLast = asOf.Sub(lastDate).Hours() / 24
	const days90 = 90 * 24
	count90 := 0
	for i := n - 1; i >= 0; i-- {
		if asOf.Sub(inn[i].Date).Hours() <= days90 {
			count90++
		} else {
			break
		}
	}
	out.InningsInLast90D = count90

	// Windowed stats: take last k values
	k3 := min(n, 3)
	if k3 > 0 {
		s3 := vals[n-k3 : n]
		out.MeanW3 = mean(s3)
	}
	k5 := min(n, 5)
	if k5 > 0 {
		s5 := vals[n-k5 : n]
		m5 := mean(s5)
		out.MeanW5 = m5
		out.StdW5 = std(s5, m5)
		if k5 >= 2 {
			out.TrendW5 = (s5[k5-1] - s5[0]) / float64(k5-1)
		}
	}
	k10 := min(n, 10)
	if k10 > 0 {
		s10 := vals[n-k10 : n]
		m10 := mean(s10)
		out.MeanW10 = m10
		out.StdW10 = std(s10, m10)
		minV, maxV := s10[0], s10[0]
		zeros := 0
		for _, v := range s10 {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
			if v == 0 {
				zeros++
			}
		}
		out.MinW10 = minV
		out.MaxW10 = maxV
		out.PctZeroW10 = float64(zeros) / float64(k10)
		sorted10 := make([]float64, len(s10))
		copy(sorted10, s10)
		sort.Float64s(sorted10)
		out.MedianW10 = median(sorted10)
	}
	k20 := min(n, 20)
	if k20 > 0 {
		s20 := vals[n-k20 : n]
		out.MeanW20 = mean(s20)
	}
	return out
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func std(v []float64, mean float64) float64 {
	if len(v) < 2 {
		return 0
	}
	var sumSq float64
	for _, x := range v {
		d := x - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(v)))
}

func median(sorted []float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}
