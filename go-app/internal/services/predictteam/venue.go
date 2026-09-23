package predictteam

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// VenueSummary says which venue the numbers in a response were produced at.
//
// It is on the wire for the same reason `team1_side` and `toss` are (§8.7): the venue is a
// model input, and an answer produced without one is a different answer. A caller who
// names no venue gets a venue-blind prediction, which is legitimate and now says so; a
// caller who names one either gets it resolved or gets a refusal. What is gone is the
// third case -- a named venue that quietly became no venue at all.
type VenueSummary struct {
	// Resolved is true where a named venue was found and its id was sent to every model.
	Resolved bool `json:"resolved"`
	// VenueID and Name are the venue that was used, present only when one was.
	VenueID int64  `json:"venue_id,omitempty"`
	Name    string `json:"name,omitempty"`
	// Note explains an unresolved answer. It is absent on a resolved one, where there is
	// nothing to explain.
	Note string `json:"note,omitempty"`
}

// venueBlindNote is what an answer says when no venue was named. The prediction is still
// made -- the models marginalise over venues they were not given -- and the note is what
// keeps that from being silent.
const venueBlindNote = "no venue was named; every model read this fixture without one"

// UnknownVenueError reports a venue the caller named that this database does not hold.
//
// It is an error rather than a note because the two honest readings of a misspelt ground
// are "you meant one we know" and "we have never seen this one", and neither is served by
// predicting at a venue nobody asked about. It used to be neither: the lookup was a
// get-or-create, so a typo inserted a new venue row and the prediction ran at a ground
// with no history, while a database failure fell through to no venue at all -- three
// different things reaching the caller as one 200 (GO-08).
type UnknownVenueError struct {
	Name string
}

func (e *UnknownVenueError) Error() string {
	return fmt.Sprintf("no venue named %q is known", e.Name)
}

// VenueLookup finds the venue a caller's name identifies, reporting whether there is one.
// It is a parameter so the resolution can be tested without a database, and it is
// read-only by contract: the prediction path must never create the venue it fails to find.
//
// Identity is the folded name, not the spelling: since IMPORT-08 the lookup matches
// `venue.normalized_name`, so a caller who writes "M.Chinnaswamy Stadium" reaches the
// ground stored as "M Chinnaswamy Stadium". Folding is not guessing -- "County Ground,
// Derby" still resolves to Derby's ground and to no other.
type VenueLookup func(ctx context.Context, name string) (int64, bool, error)

// resolveVenue turns the venue a caller named into the venue the answer was produced at,
// keeping the three outcomes apart: no venue named, a venue found, and a venue named that
// cannot be resolved. A lookup failure is the fourth and is returned as itself, so a
// database fault can never be served as "no venue".
func resolveVenue(ctx context.Context, name string, lookup VenueLookup) (VenueSummary, error) {
	if name == "" {
		return VenueSummary{Note: venueBlindNote}, nil
	}
	id, found, err := lookup(ctx, name)
	if err != nil {
		slog.Error("predictteam.PredictTeams venue lookup failed",
			slog.String("venue", name), slog.Any("err", err))
		return VenueSummary{}, fmt.Errorf("resolve venue %q: %w", name, err)
	}
	if !found {
		slog.Warn("predictteam.PredictTeams refused an unknown venue", slog.String("venue", name))
		return VenueSummary{}, &UnknownVenueError{Name: name}
	}
	return VenueSummary{Resolved: true, VenueID: id, Name: name}, nil
}

// productionVenueLookup is the lookup the served path uses: a read of the venue table on
// the folded identity key, and nothing else.
func productionVenueLookup(ctx context.Context, name string) (int64, bool, error) {
	return db.FindVenueIDByName(ctx, name)
}
