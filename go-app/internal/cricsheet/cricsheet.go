package cricsheet

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
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
	Season       Season   `json:"season"`
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
	Wickets    *Wickets       `json:"wickets,omitempty"`
}
type (
	Wickets []Wicket
	RunInfo struct {
		Batter int `json:"batter"`
		Extras int `json:"extras"`
		Total  int `json:"total"`
	}
)

type Wicket struct {
	PlayerOut string      `json:"player_out"`
	Kind      string      `json:"kind"`
	Fielders  *Collection `json:"fielders,omitempty"`
}

type Collection []string

type Season string

// UnmarshalJSON allows Season to decode from string, number, or null.
// - "2007/08" -> "2007/08"
// - 2012 -> "2012"
// - 2012.0 -> "2012"
// - null -> ""
func (s *Season) UnmarshalJSON(data []byte) error {
	// null -> empty string
	if string(data) == "null" {
		*s = Season("")
		return nil
	}
	// Try as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = Season(str)
		return nil
	}
	// Try as int
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*s = Season(strconv.Itoa(i))
		return nil
	}
	// Try as float; format cleanly (drop trailing .0 when integer)
	var f float64
	if err := json.Unmarshal(data, &f); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		if float64(int(f)) == f {
			*s = Season(strconv.Itoa(int(f)))
		} else {
			*s = Season(strconv.FormatFloat(f, 'f', -1, 64))
		}
		return nil
	}
	// Fallback: empty string
	*s = Season("")
	return nil
}

// UnmarshalJSON allows `Collection` to flexibly decode from various Cricsheet encodings:
// - null or missing -> nil
// - ["A", "B"] -> []string{"A", "B"}
// - [{"name":"A"}, {"name":"B"}] -> []string{"A", "B"}
// - {"name":"A"} -> []string{"A"}
// - "A" -> []string{"A"}
// Some datasets mix strings and objects within the same array; we will
// extract any available string or object.name values and ignore the rest.
func (c *Collection) UnmarshalJSON(data []byte) error {
	// Handle null explicitly
	if string(data) == "null" {
		*c = nil
		return nil
	}
	// Try simple []string first
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*c = Collection(ss)
		return nil
	}
	// Try []object with name field
	type named struct {
		Name string `json:"name"`
	}
	var objs []named
	if err := json.Unmarshal(data, &objs); err == nil {
		out := make([]string, 0, len(objs))
		for _, o := range objs {
			if o.Name != "" {
				out = append(out, o.Name)
			}
		}
		*c = Collection(out)
		return nil
	}
	// Try single object
	var one named
	if err := json.Unmarshal(data, &one); err == nil && one.Name != "" {
		*c = Collection{one.Name}
		return nil
	}
	// Try single string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*c = Collection{s}
		return nil
	}
	// Try generic []interface{} and pull what we can
	var anyArr []any
	if err := json.Unmarshal(data, &anyArr); err == nil {
		out := make([]string, 0, len(anyArr))
		for _, v := range anyArr {
			switch t := v.(type) {
			case string:
				if t != "" {
					out = append(out, t)
				}
			case map[string]any:
				if n, ok := t["name"].(string); ok && n != "" {
					out = append(out, n)
				}
			}
		}
		*c = Collection(out)
		return nil
	}
	// Fallback: empty collection
	*c = Collection{}
	return nil
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
	if _, err := fmt.Sscanf(hex10, "%x", &v); err != nil {
		// Fallback to zero if parsing fails; unlikely given fixed hex source
		v = 0
	}
	// bound into 12-digit space, then cast to int64
	v = (v % 900000000000) + 100000000000
	return int64(v)
}
