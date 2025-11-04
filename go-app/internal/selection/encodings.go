package selection

// encodeSession clamps session to 1..3 with default 2.
func encodeSession(s int) int {
	if s < 1 {
		return 2
	}
	if s > 3 {
		return 3
	}
	return s
}

// encodeViscosity maps arbitrary int to 0/1 (<=0 -> 0, else 1).
func encodeViscosity(v int) int {
	if v <= 0 {
		return 0
	}
	return 1
}
