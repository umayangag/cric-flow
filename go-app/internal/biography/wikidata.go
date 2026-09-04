package biography

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SPARQLEndpoint is the Wikidata Query Service. It is public, free and needs no account;
// its only condition is a User-Agent that identifies the caller and a request rate a
// human would not be ashamed of, which is why UserAgent and the pause between batches are
// both required rather than optional.
const SPARQLEndpoint = "https://query.wikidata.org/sparql"

// Wikidata properties the pass reads. Named rather than inlined because the numbers are
// meaningless on sight and a transposed digit would silently fetch a different fact.
const (
	// propertyCricinfoID is "Cricinfo player ID": the join key.
	propertyCricinfoID = "P2697"
	// propertyDateOfBirth is "date of birth".
	propertyDateOfBirth = "P569"
	// propertyDateOfDeath is "date of death".
	propertyDateOfDeath = "P570"
	// propertyWorkPeriodEnd is "end of work period": the career end date, where stated.
	propertyWorkPeriodEnd = "P2032"
	// propertyPlayingHand is "playing hand", the sport-specific handedness.
	propertyPlayingHand = "P741"
	// propertyHandedness is "handedness", the general one, read only where the
	// sport-specific property is absent.
	propertyHandedness = "P552"
	// propertyBowlingStyle is "bowling style".
	propertyBowlingStyle = "P2545"
)

// Lookup is what Wikidata answered for one ESPNcricinfo id. It is the decoded shape of a
// SPARQL result, before any player is attached to it.
type Lookup struct {
	CricinfoID      string     `json:"cricinfo_id"`
	QID             string     `json:"qid,omitempty"`
	BirthDate       *time.Time `json:"birth_date,omitempty"`
	DeathDate       *time.Time `json:"death_date,omitempty"`
	CareerEndDate   *time.Time `json:"career_end_date,omitempty"`
	BattingHandRaw  string     `json:"batting_hand_raw,omitempty"`
	BowlingStyleRaw string     `json:"bowling_style_raw,omitempty"`
}

// Found reports whether an item carries this id.
func (l Lookup) Found() bool { return strings.TrimSpace(l.QID) != "" }

// BuildQuery returns the SPARQL for one batch of ESPNcricinfo ids.
//
// One query per batch rather than one per player: the entity API would need 13,000
// requests where this needs thirty, and thirty polite requests is the difference between
// a backfill that can be run on a whim and one that needs a maintenance window.
//
// Ids are filtered to alphanumerics before they are interpolated. The register is a
// third-party file and a quotation mark in it would otherwise be a SPARQL injection into
// a query this process signs its own name to.
func BuildQuery(cricinfoIDs []string) string {
	var values strings.Builder
	for _, id := range cricinfoIDs {
		safe := safeIdentifier(id)
		if safe == "" {
			continue
		}
		values.WriteString(`"`)
		values.WriteString(safe)
		values.WriteString(`" `)
	}
	return fmt.Sprintf(`SELECT ?ci ?item ?dob ?dod ?end ?playLabel ?handLabel ?styleLabel WHERE {
  VALUES ?ci { %s}
  ?item wdt:%s ?ci .
  OPTIONAL { ?item wdt:%s ?dob }
  OPTIONAL { ?item wdt:%s ?dod }
  OPTIONAL { ?item wdt:%s ?end }
  OPTIONAL { ?item wdt:%s ?play }
  OPTIONAL { ?item wdt:%s ?hand }
  OPTIONAL { ?item wdt:%s ?style }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}`,
		strings.TrimSpace(values.String()),
		propertyCricinfoID,
		propertyDateOfBirth,
		propertyDateOfDeath,
		propertyWorkPeriodEnd,
		propertyPlayingHand,
		propertyHandedness,
		propertyBowlingStyle,
	)
}

// safeIdentifier strips everything but letters and digits from a register id. An id that
// loses characters to this is an id the register spelled in a way ESPNcricinfo does not,
// and querying the stripped form would be a guess; the caller sees an empty string and
// leaves the person unmatched.
func safeIdentifier(id string) string {
	trimmed := strings.TrimSpace(id)
	for _, r := range trimmed {
		isDigit := r >= '0' && r <= '9'
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isDigit && !isLetter {
			return ""
		}
	}
	return trimmed
}

// sparqlResponse is the shape the query service answers in.
type sparqlResponse struct {
	Results struct {
		Bindings []map[string]struct {
			Value string `json:"value"`
		} `json:"bindings"`
	} `json:"results"`
}

// DecodeLookups turns a SPARQL response into one Lookup per ESPNcricinfo id.
//
// A person can produce several bindings — two bowling styles, a duplicated item — so the
// merge is deterministic by construction rather than by trusting the server's row order:
// candidate values are collected, sorted, and the first is taken. Two runs over the same
// data therefore write the same rows, which is what makes the coverage figure a
// measurement someone else can reproduce.
func DecodeLookups(body io.Reader) (map[string]Lookup, error) {
	var response sparqlResponse
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decoding the SPARQL response: %w", err)
	}

	candidates := make(map[string]map[string][]string)
	for _, binding := range response.Results.Bindings {
		id := strings.TrimSpace(binding["ci"].Value)
		if id == "" {
			continue
		}
		if _, ok := candidates[id]; !ok {
			candidates[id] = make(map[string][]string)
		}
		for _, key := range []string{"item", "dob", "dod", "end", "playLabel", "handLabel", "styleLabel"} {
			value := strings.TrimSpace(binding[key].Value)
			if value == "" || contains(candidates[id][key], value) {
				continue
			}
			candidates[id][key] = append(candidates[id][key], value)
		}
	}

	lookups := make(map[string]Lookup, len(candidates))
	for id, fields := range candidates {
		lookups[id] = Lookup{
			CricinfoID:      id,
			QID:             qidFromURI(first(fields["item"])),
			BirthDate:       parseWikidataDate(first(fields["dob"])),
			DeathDate:       parseWikidataDate(first(fields["dod"])),
			CareerEndDate:   parseWikidataDate(first(fields["end"])),
			BattingHandRaw:  firstNonEmpty(first(fields["playLabel"]), first(fields["handLabel"])),
			BowlingStyleRaw: first(fields["styleLabel"]),
		}
	}
	return lookups, nil
}

// first returns the lexicographically smallest candidate, or empty when there is none.
// Smallest is not "best" — it is simply an order the server cannot change under us.
func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return sorted[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// qidFromURI turns "http://www.wikidata.org/entity/Q9200" into "Q9200".
func qidFromURI(uri string) string {
	if uri == "" {
		return ""
	}
	return uri[strings.LastIndex(uri, "/")+1:]
}

// parseWikidataDate reads an XSD dateTime into a date, returning nil for anything it
// cannot parse.
//
// Wikidata stores a year-precision date as the first of January, and the wdt: shortcut
// this query uses does not carry the precision qualifier. So a birth date read here can
// be a year that has been rendered as a day. That is a real limitation of the source and
// it is recorded rather than worked around: X-1b's age feature is a year-scale quantity
// and a January default costs it at most half a year, but anything that needs a true day
// must not read this field.
func parseWikidataDate(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	day := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
	return &day
}

// SPARQLClient queries the Wikidata Query Service.
//
// It is a struct rather than a package-level function because the backfill has to be able
// to run against a fixture server in a test, and because the pause between batches is a
// policy the caller sets rather than a constant buried in a request.
type SPARQLClient struct {
	Endpoint  string
	UserAgent string
	HTTP      *http.Client
	// Pause is how long to wait after each request. The query service asks callers to
	// keep the rate reasonable and answers a burst with 429; one second between batches
	// of five hundred is roughly one request per five hundred players, which no operator
	// will notice and no service will throttle.
	Pause time.Duration
	// Attempts bounds how many times one batch is tried. Zero means DefaultAttempts. A
	// public query service returns the odd 502 or 429 under no fault of the caller's, and
	// a run that gave up on the first one would need an operator to restart it by hand
	// most times it was run — which is the difference between "resumable" and "reliable".
	Attempts int
}

// Retry policy. Three attempts with a doubling wait covers the transient failures the
// query service actually produces (a 502 from its front end, a 429 from its rate limiter)
// without turning a real outage into a long silence.
const (
	// DefaultAttempts is how many times one batch is tried before the run stops.
	DefaultAttempts = 3
	// retryBackoff is the wait before the second attempt; it doubles from there.
	retryBackoff = 2 * time.Second
)

// Query runs one batch and returns the lookups it found, keyed by ESPNcricinfo id. Ids
// with no item are absent from the result: the caller knows what it asked for and records
// the misses itself.
//
// A retryable failure is retried with a doubling wait; anything else fails immediately,
// because a 400 from a malformed query will fail identically however many times it is
// sent.
func (c *SPARQLClient) Query(ctx context.Context, cricinfoIDs []string) (map[string]Lookup, error) {
	attempts := c.Attempts
	if attempts <= 0 {
		attempts = DefaultAttempts
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		lookups, err := c.queryOnce(ctx, cricinfoIDs)
		if err == nil {
			return lookups, nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == attempts {
			return nil, err
		}
		wait := retryBackoff << (attempt - 1)
		slog.Warn("wikidata: retrying a batch",
			slog.Int("attempt", attempt), slog.Int("of", attempts),
			slog.Duration("waiting", wait), slog.Any("err", err))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

// retryableError marks a failure worth trying again: a transport error, or a status the
// service uses for "not now" rather than "not ever".
type retryableError struct{ err error }

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var retryable retryableError
	return errors.As(err, &retryable)
}

// queryOnce sends one request.
func (c *SPARQLClient) queryOnce(ctx context.Context, cricinfoIDs []string) (map[string]Lookup, error) {
	if strings.TrimSpace(c.UserAgent) == "" {
		return nil, errors.New("a User-Agent identifying the caller is required by the query service")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = SPARQLEndpoint
	}

	form := url.Values{"query": {BuildQuery(cricinfoIDs)}, "format": {"json"}}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/sparql-results+json")
	request.Header.Set("User-Agent", c.UserAgent)

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		// A connection that failed to open or died mid-flight is the transient case par
		// excellence, so it is worth another attempt.
		return nil, retryableError{err}
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		statusErr := fmt.Errorf("query service answered %d: %s",
			response.StatusCode, strings.TrimSpace(string(detail)))
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			return nil, retryableError{statusErr}
		}
		return nil, statusErr
	}
	lookups, err := DecodeLookups(response.Body)
	if err != nil {
		return nil, err
	}
	if c.Pause > 0 {
		select {
		case <-ctx.Done():
			return lookups, ctx.Err()
		case <-time.After(c.Pause):
		}
	}
	return lookups, nil
}
