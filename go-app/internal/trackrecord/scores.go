package trackrecord

// The arithmetic, kept to what the harness does.
//
// ml/xi/sim_harness.py is the reference: `_brier` is the mean squared error, `reliability`
// is ten equal-width bins with mean predicted against observed frequency and n per bin, and
// `_interval_report` counts an actual total inside its 10-90 range with both ends
// inclusive. This file re-states them in Go rather than calling ml-service for them
// because they are three lines each, the record is a go-app read of go-app's tables, and
// a round trip to hand tens of rows to a service that would hand back three means would
// put a network hop and a second contract between a stored row and its score. A test
// pins one bin, the Brier and the base-rate Brier against the harness's own functions on
// a shared fixture, so the two cannot drift apart unnoticed.

// brier is the mean of (p - y)^2. Nil over no rows.
func brier(p, y []float64) *float64 {
	if len(y) == 0 {
		return nil
	}
	total := 0.0
	for i := range y {
		d := p[i] - y[i]
		total += d * d
	}
	mean := total / float64(len(y))
	return &mean
}

// mean of a slice; nil over no rows.
func mean(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	result := total / float64(len(values))
	return &result
}

// winScore is the Brier, the rows' own base rate and the base rate's Brier over n rows.
func winScore(p, y []float64) WinScore {
	score := WinScore{N: len(y), Brier: brier(p, y), BaseRate: mean(y)}
	if score.BaseRate != nil {
		constant := make([]float64, len(y))
		for i := range constant {
			constant[i] = *score.BaseRate
		}
		score.BaseRateBrier = brier(constant, y)
	}
	return score
}

// reliabilityEdges are numpy's linspace(0, 1, bins + 1): each edge is k times the step,
// computed the same way so the bin a probability falls in is the bin the harness puts it
// in -- including 0.3, which sits just below the third edge (0.30000000000000004) in both.
func reliabilityEdges(bins int) []float64 {
	step := 1.0 / float64(bins)
	edges := make([]float64, bins+1)
	for k := range edges {
		edges[k] = float64(k) * step
	}
	edges[bins] = 1.0
	return edges
}

// reliabilityBin is np.digitize(p, edges[1:-1]) clipped to the last bin: the number of
// interior edges at or below p.
func reliabilityBin(p float64, edges []float64) int {
	bins := len(edges) - 1
	bin := 0
	for k := 1; k < bins; k++ {
		if edges[k] <= p {
			bin = k
		}
	}
	return bin
}

// reliability is the harness's curve: mean predicted against observed per equal-width
// bin, empty bins omitted. An empty input is an empty curve.
func reliability(p, y []float64, bins int) []ReliabilityBin {
	edges := reliabilityEdges(bins)
	sumP := make([]float64, bins)
	sumY := make([]float64, bins)
	count := make([]int, bins)
	for i := range p {
		b := reliabilityBin(p[i], edges)
		sumP[b] += p[i]
		sumY[b] += y[i]
		count[b]++
	}
	curve := make([]ReliabilityBin, 0, bins)
	for b := 0; b < bins; b++ {
		if count[b] == 0 {
			continue
		}
		n := float64(count[b])
		curve = append(curve, ReliabilityBin{
			Lo: edges[b], Hi: edges[b+1], N: count[b],
			Predicted: sumP[b] / n, Observed: sumY[b] / n,
		})
	}
	return curve
}

// covered is the harness's inclusive test: the actual total inside the served 10-90 range.
func covered(actual int, served Range) bool {
	total := float64(actual)
	return total >= served.P10 && total <= served.P90
}

// coverageScore is covered out of n; nil coverage over no rows.
func coverageScore(n, hits int) CoverageScore {
	score := CoverageScore{N: n, Covered: hits}
	if n > 0 {
		share := float64(hits) / float64(n)
		score.Coverage = &share
	}
	return score
}

// overlap counts the named players who took the field for the side they were named for.
func overlap(named []int64, side int64, fielded map[int64]int64) int {
	matched := 0
	for _, id := range named {
		if fielded[id] == side {
			matched++
		}
	}
	return matched
}
