// Package predictions is the record of what this system has claimed: every prediction it
// issued, kept as it was served (P2-3).
//
// Nothing here computes a prediction and nothing here scores one. The prediction path
// hands this package a finished answer; `internal/db` implements the storage; P2-4 reads
// it back and scores it against the matches the importer has since brought in. The
// package exists as its own thing so the store's shape is stated once, away from both the
// handler that writes it and the SQL that holds it.
//
// The record is *not* a cache (migration 0007 dropped one of those and says why). A row
// here cannot be recomputed: it names a rating state that the next retrain replaces, and
// the claim it holds was made before the match was played. That is the whole reason for
// keeping the served payload whole rather than a summary of it.
package predictions

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound reports that the record holds no prediction with the id asked for. It is a
// sentinel rather than a nil result so a handler answers 404 with the id, instead of 200
// with nothing in it.
var ErrNotFound = errors.New("no prediction with that id")

// NewID mints the identifier a prediction is filed and read back under.
//
// A UUID rather than a sequence: the id is generated before the insert, because it is
// inside the payload the row holds, so it cannot be something the database hands back.
func NewID() string { return uuid.NewString() }

// Prediction is one issued answer as the store holds it: the two documents, and the
// columns a resolver joins on and a record sorts by.
//
// Every column beside the documents is also *inside* one of them. They are columns
// because a join and a sort should not have to parse a payload, not because the store
// knows anything the answer did not already say.
type Prediction struct {
	// ID and IssuedAt are generated before the insert, because both are inside Payload:
	// the row holds the bytes that left the server, and those bytes name the row.
	ID       string
	IssuedAt time.Time

	// RunID and RatingsThrough are the rating state the answer was computed from, read
	// off the answer itself (P1-5) rather than off a status call, which describes
	// whatever is loaded at the moment of the call.
	RunID          string
	RatingsThrough time.Time

	// The fixture, as the importer will present it when the match is played. Both
	// opposition ids are club ids -- COALESCE(opposition.canonical_id, id) -- written as
	// such by the prediction path and read back as such by the store, so that a club
	// renamed after the answer was filed is still the same club to a reader (GO-02).
	FormatCode        string
	Team1OppositionID int64
	Team2OppositionID int64
	Gender            string
	MatchDate         time.Time

	// SelectionObjective is "win", "ratings" or "fixed". It is the one column that says
	// what kind of answer this is: `fixed` is an eleven the caller pinned in Play mode —
	// a scenario, which a record lists and never scores (roadmap § 5, clause 5).
	SelectionObjective string

	// The headline claim and the model behind it, which is what a Brier is computed over.
	WinProbabilityTeam1  float64
	WinProbabilitySource string

	// SimulatorSharedFactor says whether the simulator that served this answer carried a
	// shared match factor (B-12). Nil where the answer was stored before the field existed
	// or where no simulator ran (a format with no innings length) -- the track record reads
	// the payload to tell those two apart and reports the first as its own population.
	SimulatorSharedFactor *bool

	// Request is the body the caller sent, and Payload is the answer it received. Both
	// are stored whole so a scorer written later never finds that the field it needs was
	// not one of the columns somebody thought of.
	Request json.RawMessage
	Payload json.RawMessage
}

// Recorder files one issued answer.
//
// One method, because recording is the whole of what the prediction path needs from the
// record: the handler must be able to say, on the answer, whether it was filed, and it
// must not be able to do anything else to the store on the way past.
type Recorder interface {
	Record(ctx context.Context, prediction Prediction) error
}

// Query is one page of the record, newest first.
type Query struct {
	Limit  int
	Offset int
}

// Page is one page of the record with the count the page was taken out of, so a surface
// can say "20 of 143" rather than "20".
type Page struct {
	Predictions []Prediction
	Total       int
}

// Reader is the record's read side: one stored answer, or a page of them.
//
// It is separate from Recorder because the two are used by different handlers and a
// handler should be given only what it needs — the prediction path can file an answer and
// cannot read the record back, which is the smaller of the two capabilities.
type Reader interface {
	Get(ctx context.Context, id string) (*Prediction, error)
	List(ctx context.Context, query Query) (Page, error)
	// All returns the whole record, oldest first, payloads included. The track record
	// needs every row at once: which forecast of a fixture is the last one issued is a
	// question about all of them, and the scores are read out of the payloads.
	All(ctx context.Context) ([]Prediction, error)
}

// Store is both halves, which is what the database-backed implementation is.
type Store interface {
	Recorder
	Reader
}

// RecordBlock is what an answer says about its own filing (§8.7).
//
// It travels on the prediction because a failure to record must be visible to whoever
// received the answer, not only in a server log: a track record with a silent hole in it
// is the tipster's page § 1 says the harness is the antidote to. `stored` is a boolean
// and not a status word because there are exactly two states and no third one is coming —
// the row is there or the reason it is not is beside it.
type RecordBlock struct {
	Stored bool `json:"stored"`
	// ID and IssuedAt name the row, and are absent when there is no row to name.
	ID       string `json:"id,omitempty"`
	IssuedAt string `json:"issued_at,omitempty"`
	// Reason is why the answer was not recorded, in the store's own words. Present only
	// on a failure.
	Reason string `json:"reason,omitempty"`
}
