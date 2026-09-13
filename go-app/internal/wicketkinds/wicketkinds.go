// Package wicketkinds reads the vocabulary of wicket kinds and what each is to the
// scorecard.
//
// A wicket in a Cricsheet file is one of fourteen kinds, and they are not all the same
// thing: six are the bowler's (bowled, caught, caught and bowled, hit wicket, lbw,
// stumped), six are dismissals nobody bowls (run out, retired out, obstructing the field,
// handled the ball, hit the ball twice, timed out), and two are not dismissals at all -- a
// batter who retires hurt or retires not out may come back, and the scorecard does not
// count him as a wicket lost. Until IMPORT-06 was fixed the importer credited every kind
// to the bowler and counted every kind as a wicket lost.
//
// The vocabulary is committed data (configs/wicket_kinds.json), read by this package and
// by the rating pass (ml/xi/wicketkinds.py), so the two languages cannot carry different
// sets: the importer's bowling_data.wickets and the pass's wickets target are one rule.
// A kind that is not in the file is an error rather than a guess, because the only way a
// new kind arrives is Cricsheet adding one, and the right answer to that is a reviewed
// edit to the file.
package wicketkinds

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the vocabulary's name under configs/.
const FileName = "wicket_kinds.json"

// EnvVar overrides where the vocabulary is read from.
const EnvVar = "GO_APP_WICKET_KINDS"

// Class is what a kind of wicket is to the scorecard.
type Class int

const (
	// CreditedToBowler is a dismissal that goes into the bowler's wickets column.
	CreditedToBowler Class = iota + 1
	// DismissalNotCredited is a dismissal that is nobody's: the innings loses a wicket,
	// the bowler's figures do not move.
	DismissalNotCredited
	// NotOut is a batter who left the crease without being dismissed and may return.
	NotOut
)

// IsDismissal reports whether the innings lost a wicket.
func (c Class) IsDismissal() bool {
	return c == CreditedToBowler || c == DismissalNotCredited
}

// String names the class the way the vocabulary file does.
func (c Class) String() string {
	switch c {
	case CreditedToBowler:
		return "credited_to_bowler"
	case DismissalNotCredited:
		return "dismissal_not_credited"
	case NotOut:
		return "not_out"
	default:
		return fmt.Sprintf("Class(%d)", int(c))
	}
}

// Kind is one wicket kind as the vocabulary spells it, and its class.
type Kind struct {
	Name  string
	Class Class
}

// Vocabulary is the reviewed set of wicket kinds.
type Vocabulary struct {
	Version              string   `json:"version"`
	CreditedToBowler     []string `json:"credited_to_bowler"`
	DismissalNotCredited []string `json:"dismissal_not_credited"`
	NotOut               []string `json:"not_out"`
}

// Kind resolves the kind a file spells -- case and surrounding space aside -- to the
// vocabulary's spelling and class. The archive has always spelled every kind exactly as
// the file does, so the normalisation is defensive; the error is not.
func (v Vocabulary) Kind(raw string) (Kind, error) {
	name := normalise(raw)
	for class, names := range v.byClass() {
		for _, candidate := range names {
			if candidate == name {
				return Kind{Name: candidate, Class: class}, nil
			}
		}
	}
	return Kind{}, fmt.Errorf("wicket kind %q is not in the vocabulary (%s)", raw, FileName)
}

// Validate rejects a vocabulary that could classify a kind two ways or none.
func (v Vocabulary) Validate() error {
	seen := map[string]Class{}
	for class, names := range v.byClass() {
		if len(names) == 0 {
			return fmt.Errorf("the %s list is empty", class)
		}
		for _, name := range names {
			if name != normalise(name) || name == "" {
				return fmt.Errorf("%q is not in the vocabulary's own spelling (lower case, trimmed)", name)
			}
			if previous, dup := seen[name]; dup {
				return fmt.Errorf("%q is listed twice (%s and %s); a kind is one thing", name, previous, class)
			}
			seen[name] = class
		}
	}
	return nil
}

func (v Vocabulary) byClass() map[Class][]string {
	return map[Class][]string{
		CreditedToBowler:     v.CreditedToBowler,
		DismissalNotCredited: v.DismissalNotCredited,
		NotOut:               v.NotOut,
	}
}

func normalise(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// Load reads the vocabulary from GO_APP_WICKET_KINDS, or from configs/ searched upwards
// from the working directory, matching how config.json and the team lineage are found.
//
// Unlike the lineage, a missing file is an error: nothing can say what a run out is to the
// bowler without it, and an import that guessed would be the defect this file exists to
// close.
func Load() (Vocabulary, error) {
	path := os.Getenv(EnvVar)
	if path == "" {
		path = find()
	}
	if path == "" {
		slog.Error("wicketkinds: no vocabulary found",
			slog.String("looked_for", filepath.Join("configs", FileName)),
			slog.String("env", EnvVar))
		return Vocabulary{}, fmt.Errorf("no %s found under configs/ (set %s)", FileName, EnvVar)
	}
	return LoadFile(path)
}

// LoadFile reads and validates one vocabulary file.
func LoadFile(path string) (Vocabulary, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- operator-supplied config path
	if err != nil {
		slog.Error("wicketkinds: read failed", slog.String("path", path), slog.Any("err", err))
		return Vocabulary{}, fmt.Errorf("read %s: %w", path, err)
	}
	var v Vocabulary
	if err := json.Unmarshal(b, &v); err != nil {
		slog.Error("wicketkinds: parse failed", slog.String("path", path), slog.Any("err", err))
		return Vocabulary{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := v.Validate(); err != nil {
		slog.Error("wicketkinds: vocabulary is not usable", slog.String("path", path), slog.Any("err", err))
		return Vocabulary{}, fmt.Errorf("%s: %w", path, err)
	}
	slog.Info("wicketkinds: loaded",
		slog.String("path", path),
		slog.Int("kinds", len(v.CreditedToBowler)+len(v.DismissalNotCredited)+len(v.NotOut)))
	return v, nil
}

func find() string {
	for _, dir := range []string{".", "..", filepath.Join("..", ".."), filepath.Join("..", "..", "..")} {
		p := filepath.Join(dir, "configs", FileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
