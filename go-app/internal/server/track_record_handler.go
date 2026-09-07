package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/trackrecord"
)

// matchLookup is the record's view of the match tables, on the same terms as the
// prediction reader: nil is the process default, tests set a fake.
func (a *App) matchLookup() trackrecord.MatchLookup {
	if a != nil && a.matchLookupStore != nil {
		return a.matchLookupStore
	}
	return db.NewMatchLookup()
}

// trackRecordHandler answers GET /api/track-record: every stored prediction in its
// state, the scored ones against what happened, computed on this read (P2-4).
//
// It reads the whole record every time. There is no cache and no stored state because
// the states move on their own -- an import brings a match, and the next read finds it --
// and a copy would be exactly the thing that could say "unresolved" after the import.
func (a *App) trackRecordHandler(w http.ResponseWriter, r *http.Request) {
	stored, err := a.predictionReader().All(r.Context())
	if err != nil {
		slog.Error("trackRecord: reading the record failed", slog.Any("err", err))
		respondErr(w, err)
		return
	}
	record, err := trackrecord.Build(r.Context(), stored, a.matchLookup(), time.Now().UTC())
	if err != nil {
		slog.Error("trackRecord: resolving the record against the match tables failed", slog.Any("err", err))
		respondErr(w, err)
		return
	}
	slog.Info("trackRecord: computed",
		slog.Int("total", record.Total),
		slog.Int("scored", record.States[trackrecord.StateScored]),
		slog.Int("unresolved", record.States[trackrecord.StateUnresolved]))
	writeJSON(w, http.StatusOK, record)
}
