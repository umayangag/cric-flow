// Package teamlineage reads the reviewed mapping of franchise renames.
//
// A club that renames appears in Cricsheet under both names, so it becomes two opposition
// rows and every team-level feature -- Elo, form, head to head, venue familiarity -- starts
// again at the boundary. The mapping that joins them back up is committed data
// (configs/team_lineage.json), not something computed at import time: a detector run on
// every import would merge two genuinely different clubs the first time a coincidence
// cleared its threshold, and a wrong merge is invisible.
//
// See docs/IDENTITY_PR_CHECKLIST.md I-4, and
// scripts/experiments/xi/team_lineage_candidates.py for the one-off that proposed the list.
package teamlineage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the mapping's name under configs/.
const FileName = "team_lineage.json"

// EnvVar overrides where the mapping is read from.
const EnvVar = "GO_APP_TEAM_LINEAGE"

// Rename is one club's change of name, for one gender. A team is (name, gender), and a
// club may rename its men's side and its women's side separately, so each is its own entry.
type Rename struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Gender string `json:"gender"`
	Note   string `json:"note"`
}

// Mapping is the reviewed set of renames.
type Mapping struct {
	Version string   `json:"version"`
	Renames []Rename `json:"renames"`
}

// Key identifies a team: the name it played under, and its gender.
type Key struct {
	Name   string
	Gender string
}

// Successor returns the team a superseded name became, and whether there is one.
func (m Mapping) Successor(name, gender string) (Key, bool) {
	for _, r := range m.Renames {
		if r.From == name && r.Gender == gender {
			return Key{Name: r.To, Gender: r.Gender}, true
		}
	}
	return Key{}, false
}

// Validate rejects a mapping that cannot mean what it says.
//
// The chain rule is the one that matters: if A became B and B became C, then a single hop
// leaves A pointing at a club that no longer exists under that name, and "the club" stops
// being one query. Rather than resolve chains silently -- which is where a mapping quietly
// starts meaning something nobody reviewed -- this refuses them, and whoever adds the
// second rename collapses it by hand.
func (m Mapping) Validate() error {
	sources := map[Key]bool{}
	targets := map[Key]bool{}
	for _, r := range m.Renames {
		if strings.TrimSpace(r.From) == "" || strings.TrimSpace(r.To) == "" || strings.TrimSpace(r.Gender) == "" {
			return fmt.Errorf("rename %+v: from, to and gender are all required", r)
		}
		if r.From == r.To {
			return fmt.Errorf("rename %+v: a club cannot succeed itself", r)
		}
		key := Key{Name: r.From, Gender: r.Gender}
		if sources[key] {
			return fmt.Errorf("%q (%s) is named as the predecessor twice; a club became one thing", r.From, r.Gender)
		}
		sources[key] = true
		targets[Key{Name: r.To, Gender: r.Gender}] = true
	}
	for key := range targets {
		if sources[key] {
			return fmt.Errorf(
				"%q (%s) is both a successor and a predecessor: collapse the chain into one hop by hand",
				key.Name, key.Gender)
		}
	}
	return nil
}

// Load reads the mapping from GO_APP_TEAM_LINEAGE, or from configs/ searched upwards from
// the working directory, matching how config.json is found.
//
// A missing file is not an error: a deployment with no renames recorded is a valid one, and
// failing the import over it would cost the whole dataset for a file that adds nine rows of
// bookkeeping. It is logged, because silently having no lineage is how the column would
// come to be empty without anyone noticing.
func Load() (Mapping, error) {
	path := os.Getenv(EnvVar)
	if path == "" {
		path = find()
	}
	if path == "" {
		slog.Warn("teamlineage: no mapping found; renamed clubs will stay separate",
			slog.String("looked_for", filepath.Join("configs", FileName)),
			slog.String("env", EnvVar))
		return Mapping{}, nil
	}
	return LoadFile(path)
}

// LoadFile reads and validates one mapping file.
func LoadFile(path string) (Mapping, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- operator-supplied config path
	if err != nil {
		slog.Error("teamlineage: read failed", slog.String("path", path), slog.Any("err", err))
		return Mapping{}, fmt.Errorf("read %s: %w", path, err)
	}
	var m Mapping
	if err := json.Unmarshal(b, &m); err != nil {
		slog.Error("teamlineage: parse failed", slog.String("path", path), slog.Any("err", err))
		return Mapping{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		slog.Error("teamlineage: mapping is not usable", slog.String("path", path), slog.Any("err", err))
		return Mapping{}, fmt.Errorf("%s: %w", path, err)
	}
	slog.Info("teamlineage: loaded", slog.String("path", path), slog.Int("renames", len(m.Renames)))
	return m, nil
}

func find() string {
	for _, dir := range []string{".", "..", filepath.Join("..", "..")} {
		p := filepath.Join(dir, "configs", FileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
