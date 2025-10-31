package cricsheet

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

// Structures matching Cricsheet v1.1 JSON (subset we need)

type Match struct {
	Info    Info      `json:"info"`
	Innings []Innings `json:"innings"`
}

type Info struct {
	BallsPerOver int      `json:"balls_per_over"`
	Dates        []string `json:"dates"`
	MatchType    string   `json:"match_type"`
	Teams        []string `json:"teams"`
	Venue        string   `json:"venue"`
	City         string   `json:"city"`
	Season       string   `json:"season"`
	Event        *Event   `json:"event"`
	Toss         *Toss    `json:"toss"`
	Outcome      *Outcome `json:"outcome"`
}

type Event struct {
	MatchNumber *int `json:"match_number"`
}

type Toss struct {
	Winner string `json:"winner"`
}

type Outcome struct {
	Winner string `json:"winner"`
}

type Innings struct {
	Team  string `json:"team"`
	Overs []Over `json:"overs"`
}

type Over struct {
	Over       int        `json:"over"`
	Deliveries []Delivery `json:"deliveries"`
}

type Delivery struct {
	Batter     string         `json:"batter"`
	Bowler     string         `json:"bowler"`
	NonStriker string         `json:"non_striker"`
	Runs       RunInfo        `json:"runs"`
	Extras     map[string]int `json:"extras"`
	Wickets    []Wicket       `json:"wickets"`
}

type RunInfo struct {
	Batter int `json:"batter"`
	Extras int `json:"extras"`
	Total  int `json:"total"`
}

type Wicket struct {
	PlayerOut string   `json:"player_out"`
	Kind      string   `json:"kind"`
	Fielders  []string `json:"fielders"`
}

// Parse decodes a Cricsheet JSON match from reader.
func Parse(r io.Reader) (*Match, error) {
	dec := json.NewDecoder(r)
	var m Match
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// StableMatchID returns a deterministic int64 based on date + team names.
func StableMatchID(dateISO, teamA, teamB string) int64 {
	arr := []byte(fmt.Sprintf("%s|%s|%s", dateISO, teamA, teamB))
	h := md5.Sum(arr)
	hex10 := hex.EncodeToString(h[:])[:10]
	var v uint64
	fmt.Sscanf(hex10, "%x", &v)
	// bound into 12-digit space, then cast to int64
	v = (v % 900000000000) + 100000000000
	return int64(v)
}
