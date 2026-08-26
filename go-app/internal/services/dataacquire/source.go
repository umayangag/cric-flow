// Package dataacquire downloads Cricsheet archives into the staging directory.
//
// It exists because getting a new dataset onto the box was the one pipeline step that
// could not be done without a terminal (ops plan A-1). Everything else here follows
// from the two scope decisions made before the plan: the source is a URL the server
// fetches, and the deployment is localhost now but remote later. The second is why the
// defences are here from the first commit rather than retrofitted — an allowlist added
// after the fact has to be argued for against a working feature.
package dataacquire

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Feed is a named Cricsheet archive. Naming feeds server-side is what lets the UI
// offer a picker instead of a URL box, which in turn is what makes the allowlist the
// normal path rather than an obstacle.
type Feed struct {
	// ID is the stable name the API accepts.
	ID string `json:"id"`
	// Label is what the operator sees.
	Label string `json:"label"`
	// URL is the archive location. It must satisfy the same allowlist as a
	// user-supplied URL — the check is not skipped for being ours.
	URL string `json:"url"`
	// Description says what is in the archive.
	Description string `json:"description"`
}

// feeds is the set of archives that can be fetched by name.
var feeds = []Feed{
	{
		ID:          "all",
		Label:       "All matches",
		URL:         "https://cricsheet.org/downloads/all_json.zip",
		Description: "Every match Cricsheet publishes, in JSON. The largest archive.",
	},
	{
		ID:          "t20s",
		Label:       "Men's T20 internationals",
		URL:         "https://cricsheet.org/downloads/t20s_json.zip",
		Description: "Men's T20 internationals only.",
	},
	{
		ID:          "odis",
		Label:       "Men's ODIs",
		URL:         "https://cricsheet.org/downloads/odis_json.zip",
		Description: "Men's one-day internationals only.",
	},
	{
		ID:          "tests",
		Label:       "Men's Tests",
		URL:         "https://cricsheet.org/downloads/tests_json.zip",
		Description: "Men's Test matches only.",
	},
	{
		ID:          "ipl",
		Label:       "Indian Premier League",
		URL:         "https://cricsheet.org/downloads/ipl_json.zip",
		Description: "All IPL seasons.",
	},
}

// Feeds returns the named feeds, in presentation order.
func Feeds() []Feed { return append([]Feed(nil), feeds...) }

// FeedByID returns the named feed.
func FeedByID(id string) (Feed, bool) {
	for _, f := range feeds {
		if strings.EqualFold(f.ID, id) {
			return f, true
		}
	}
	return Feed{}, false
}

// allowedHosts is the SSRF allowlist: the only hosts the server will fetch from.
//
// A server-side fetch of a user-supplied URL is an SSRF primitive. On localhost that
// is uninteresting; the moment this runs anywhere else it is a way to reach the
// metadata service, an internal admin port, or anything else the box can route to.
// The allowlist is therefore not a hardening pass to do later — it is the feature's
// definition. Redirects are re-checked against it (see fetch.go), because a 302 to
// 169.254.169.254 defeats a check done only on the URL the operator typed.
var allowedHosts = []string{
	"cricsheet.org",
	"www.cricsheet.org",
}

// ErrHostNotAllowed is returned for a URL outside the allowlist.
var ErrHostNotAllowed = errors.New("host is not on the Cricsheet allowlist")

// AllowedHosts returns the allowlist, so the UI can state the rule rather than let
// the operator discover it by being refused.
func AllowedHosts() []string { return append([]string(nil), allowedHosts...) }

// hostAllowed reports whether the host — with any port stripped — is on the allowlist.
// Matching is exact rather than by suffix: a suffix rule makes
// "cricsheet.org.attacker.example" a match, which is the classic way these lists fail.
func hostAllowed(host string) bool {
	h := strings.ToLower(host)
	if i := strings.LastIndex(h, ":"); i > -1 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	for _, allowed := range allowedHosts {
		if h == allowed {
			return true
		}
	}
	return false
}

// Source is a validated archive location, ready to fetch.
type Source struct {
	// FeedID names the feed when the source came from one, else "".
	FeedID string
	// URL is the validated absolute URL.
	URL *url.URL
	// Filename is the name the archive is stored under in staging.
	Filename string
}

// ResolveSource turns a feed name or an explicit URL into a validated Source.
//
// Exactly one of feedID and rawURL must be given. Both paths go through the same
// allowlist check: a named feed is a convenience, not an exemption.
func ResolveSource(feedID, rawURL string) (Source, error) {
	feedID = strings.TrimSpace(feedID)
	rawURL = strings.TrimSpace(rawURL)

	switch {
	case feedID == "" && rawURL == "":
		return Source{}, errors.New("give either a feed name or a url")
	case feedID != "" && rawURL != "":
		return Source{}, errors.New("give a feed name or a url, not both")
	}

	if feedID != "" {
		feed, ok := FeedByID(feedID)
		if !ok {
			return Source{}, fmt.Errorf("unknown feed %q; known feeds: %s", feedID, strings.Join(FeedIDs(), ", "))
		}
		src, err := sourceFromURL(feed.URL)
		if err != nil {
			return Source{}, err
		}
		src.FeedID = feed.ID
		return src, nil
	}
	return sourceFromURL(rawURL)
}

// FeedIDs returns the known feed names, sorted, for error messages.
func FeedIDs() []string {
	out := make([]string, 0, len(feeds))
	for _, f := range feeds {
		out = append(out, f.ID)
	}
	sort.Strings(out)
	return out
}

// sourceFromURL parses and validates one URL.
func sourceFromURL(raw string) (Source, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Source{}, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" {
		return Source{}, fmt.Errorf("url must be https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return Source{}, errors.New("url has no host")
	}
	if !hostAllowed(u.Host) {
		return Source{}, fmt.Errorf("%w: %s (allowed: %s)", ErrHostNotAllowed, u.Host, strings.Join(allowedHosts, ", "))
	}
	name, err := ArchiveFilename(u)
	if err != nil {
		return Source{}, err
	}
	return Source{URL: u, Filename: name}, nil
}
