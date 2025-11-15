package phase

import "strings"

const (
	PhasePowerplay = "powerplay"
	PhaseMiddle    = "middle"
	PhaseDeath     = "death"
	PhaseAll       = "all"
)

// PhaseForCode determines the innings phase for a given canonical format code.
// Supported codes: "T20", "T20I", "ODI", "TEST" (case-insensitive).
// ballSeq is the 1-based index of legal deliveries in the innings.
// inningsLength is the total number of legal deliveries in the innings when known (0 if unknown).
func PhaseForCode(formatCode string, ballSeq, inningsLength int) string {
	code := strings.ToUpper(strings.TrimSpace(formatCode))
	switch code {
	case "T20", "T20I":
		return phaseT20(ballSeq, inningsLength)
	case "ODI":
		return phaseODI(ballSeq, inningsLength)
	case "TEST":
		return PhaseAll
	default:
		// Unknown formats treated as single bucket to avoid misclassification.
		return PhaseAll
	}
}

// PhaseFor maps a numeric format identifier to phases. This assumes the seed order from
// migrations (0004_format_dimension.sql): 1=TEST, 2=ODI, 3=T20, 4=T20I. If your DB differs,
// prefer PhaseForCode.
func PhaseFor(formatID, ballSeq, inningsLength int) string {
	switch formatID {
	case 3, 4: // T20/T20I
		return phaseT20(ballSeq, inningsLength)
	case 2: // ODI
		return phaseODI(ballSeq, inningsLength)
	case 1: // TEST
		return PhaseAll
	default:
		return PhaseAll
	}
}

// T20 rules (6 balls/over):
//   - Powerplay: first 36 legal balls (overs 1–6)
//   - Death: last 30 legal balls (overs 16–20) — anchored to start at ball 91.
//     For shortened innings (<91 legal balls), we do not classify any ball as death.
//   - Middle: all between.
func phaseT20(ballSeq, inningsLength int) string {
	if ballSeq <= 36 {
		return PhasePowerplay
	}
	// Death window starts at ball 91, but only if innings is long enough.
	if ballSeq >= 91 && (inningsLength == 0 || 91 <= inningsLength) {
		return PhaseDeath
	}
	return PhaseMiddle
}

// ODI rules (6 balls/over):
//   - Powerplay (PP1): first 60 legal balls (overs 1–10)
//   - Death: last 60 legal balls (overs 41–50) — anchored to start at ball 241.
//     For shortened innings (<241 legal balls), we do not classify any ball as death.
//   - Middle: all between.
func phaseODI(ballSeq, inningsLength int) string {
	if ballSeq <= 60 {
		return PhasePowerplay
	}
	if ballSeq >= 241 && (inningsLength == 0 || 241 <= inningsLength) {
		return PhaseDeath
	}
	return PhaseMiddle
}
