package predictteam

import "fmt"

// ServedRatings names the rating state a prediction was answered from: the run the models
// and ratings were loaded from, and the last match date those ratings include (P1-5).
//
// It is on every prediction because a number copied, exported or screenshotted out of the
// product loses everything but itself, and the one thing that has to travel with it is the
// date it describes (roadmap § 2.1). Both fields are required on the wire: a prediction
// without its date is the defect this type closes, so there is no omitempty to hide behind.
//
// The values come from ml-service's own answers rather than from a separate /xi/status
// read. A status read describes whatever is loaded at the moment of the read, and a reload
// can land between a prediction and that read; the stamp on each answer describes the
// store that computed it. ml-service reads both off the same manifest and state that
// /xi/status reports, so the payload and the status cannot disagree about a store.
type ServedRatings struct {
	RunID          string `json:"run_id"`
	RatingsThrough string `json:"ratings_through"`
}

// ServedRunChangedError reports a prediction assembled across a reload: the calls that
// produced it were answered from two different rating states.
//
// It is refused rather than stamped with either, because neither stamp would be true of
// the whole answer — the XI from one run beside a probability from another — and the
// caller's remedy is to run the prediction again, which the handler says (§8.7).
type ServedRunChangedError struct {
	Was ServedRatings
	Now ServedRatings
}

func (e *ServedRunChangedError) Error() string {
	return fmt.Sprintf(
		"the served run changed while this prediction was being assembled: run %s (ratings through %s) "+
			"then run %s (ratings through %s)",
		e.Was.RunID, e.Was.RatingsThrough, e.Now.RunID, e.Now.RatingsThrough)
}

// Adopt records the stamp one ml-service answer carried.
//
// The first stamp is taken as the prediction's; every later one must agree with it. An
// answer with no stamp is refused outright rather than treated as "unknown", because an
// unknown date on the wire is exactly a dateless prediction with a different spelling.
//
// Exported because the auction's projection assembles as many answers as it has grounds
// (P3-2) and needs the same check: one projection stamped with one run, or refused.
func (s *ServedRatings) Adopt(answer ServedRatings) error {
	if answer.RunID == "" || answer.RatingsThrough == "" {
		return fmt.Errorf("ml-service answered without naming the run and the date it served from")
	}
	if s.RunID == "" && s.RatingsThrough == "" {
		*s = answer
		return nil
	}
	if *s == answer {
		return nil
	}
	return &ServedRunChangedError{Was: *s, Now: answer}
}
