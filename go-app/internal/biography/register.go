package biography

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// RegisterURL is where Cricsheet publishes the people register: one row per person, the
// same identifier the match files carry in info.registry, and that person's id on every
// other site that has one. It is the bridge to Wikidata, and it is free to fetch.
const RegisterURL = "https://cricsheet.org/register/people.csv"

// Register columns this package reads. The register carries a dozen more (BCCI, Cricbuzz,
// Opta, Pulse); only the ESPNcricinfo keys are here because only they have a Wikidata
// property to join on.
const (
	registerColumnIdentifier = "identifier"
	registerColumnName       = "name"
)

// registerCricinfoColumns are the ESPNcricinfo id columns, in preference order. A person
// with more than one is a person ESPNcricinfo has listed twice; both are tried, because
// Wikidata may carry either.
var registerCricinfoColumns = []string{"key_cricinfo", "key_cricinfo_2", "key_cricinfo_3"}

// RegisterEntry is one person in Cricsheet's register.
type RegisterEntry struct {
	// Identifier is the value player.external_id holds.
	Identifier string
	// Name is the register's spelling, used only in the report so an unmatched player
	// can be looked up by a human.
	Name string
	// CricinfoIDs are the ESPNcricinfo ids for this person, in preference order. Empty
	// for the small number of people ESPNcricinfo does not list.
	CricinfoIDs []string
}

// ParseRegister reads Cricsheet's people.csv into entries keyed by Cricsheet identifier.
//
// It reads the header rather than fixed column positions: the register has gained columns
// over time (key_pulse_2 is newer than key_cricinfo), and a positional parser would
// silently start reading the wrong field the next time it does.
func ParseRegister(r io.Reader) (map[string]RegisterEntry, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading the register header: %w", err)
	}
	index := make(map[string]int, len(header))
	for i, column := range header {
		index[strings.TrimSpace(column)] = i
	}
	identifierAt, ok := index[registerColumnIdentifier]
	if !ok {
		return nil, errors.New("the register has no " + registerColumnIdentifier + " column")
	}

	entries := make(map[string]RegisterEntry)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading a register row: %w", err)
		}
		identifier := field(record, identifierAt)
		if identifier == "" {
			continue
		}
		entries[identifier] = RegisterEntry{
			Identifier:  identifier,
			Name:        field(record, index[registerColumnName]),
			CricinfoIDs: cricinfoIDs(record, index),
		}
	}
	return entries, nil
}

// cricinfoIDs collects the non-empty ESPNcricinfo ids on one register row, in preference
// order and without duplicates.
func cricinfoIDs(record []string, index map[string]int) []string {
	var ids []string
	for _, column := range registerCricinfoColumns {
		at, ok := index[column]
		if !ok {
			continue
		}
		value := field(record, at)
		if value == "" || contains(ids, value) {
			continue
		}
		ids = append(ids, value)
	}
	return ids
}

// field reads a column, tolerating a short row rather than panicking on one. The register
// is a third-party file; a truncated line should cost that person's ids, not the run.
func field(record []string, at int) string {
	if at < 0 || at >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[at])
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
