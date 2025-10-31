package cricinfo

import (
	"strings"
)

// DeriveSessions mirrors the prototype logic in src/scrapers/session.py.
// It accepts the innings number (1-based) and a map of meta sections -> text,
// specifically looking for key "Hours of play (local time)".
// It returns (battingSession, bowlingSession). When the key is missing or the
// value cannot be parsed as expected, it returns empty strings.
//
// Notes about the original logic:
//   - It expects a comma-separated string with at least 4 segments describing
//     multiple intervals. In some cases a missing comma before "Interval" is
//     fixed by inserting one (first occurrence).
//   - For innings 1, the batting and bowling sessions are taken from positions 1
//     and 3 respectively; for other innings it is the reverse.
//   - The session token is extracted as the fourth whitespace-separated token
//     within those segments (index 3), e.g., "Session 1: Morning" -> "Morning".
func DeriveSessions(inning int, hours map[string]string) (string, string) {
	if hours == nil {
		return "", ""
	}
	text, ok := hours["Hours of play (local time)"]
	if !ok || strings.TrimSpace(text) == "" {
		return "", ""
	}

	sessionArray := strings.Split(text, ",")
	if len(sessionArray) == 3 { // add missing comma to fix data (first occurrence)
		textFixed := strings.Replace(text, " Interval", ", Interval", 1)
		sessionArray = strings.Split(textFixed, ",")
	}

	getToken := func(idx int) string {
		if idx < 0 || idx >= len(sessionArray) {
			return ""
		}
		part := strings.TrimSpace(sessionArray[idx])
		parts := strings.Fields(part)
		if len(parts) >= 4 {
			return parts[3]
		}
		return ""
	}

	battingSession := getToken(3)
	bowlingSession := getToken(1)
	if inning == 1 {
		battingSession = getToken(1)
		bowlingSession = getToken(3)
	}
	return battingSession, bowlingSession
}
