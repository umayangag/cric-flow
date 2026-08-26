package dataacquire

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveSource_RejectsOffAllowlistHosts is the SSRF guard. Each case is a URL
// that would otherwise turn "fetch a dataset" into "make the server request this".
func TestResolveSource_RejectsOffAllowlistHosts(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"cloud metadata service":     "https://169.254.169.254/latest/meta-data/",
		"loopback":                   "https://127.0.0.1:8080/admin",
		"private range":              "https://10.0.0.5/internal",
		"suffix lookalike":           "https://cricsheet.org.attacker.example/all_json.zip",
		"prefix lookalike":           "https://notcricsheet.org/all_json.zip",
		"subdomain not on the list":  "https://evil.cricsheet.org/all_json.zip",
		"userinfo hiding the host":   "https://cricsheet.org@attacker.example/all_json.zip",
		"plaintext":                  "http://cricsheet.org/downloads/all_json.zip",
		"file scheme":                "file:///etc/passwd",
		"scheme-relative, no scheme": "//cricsheet.org/downloads/all_json.zip",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveSource("", raw)
			require.Error(t, err, "%s must be refused", raw)
		})
	}
}

func TestResolveSource_AcceptsAllowlistedURL(t *testing.T) {
	t.Parallel()
	src, err := ResolveSource("", "https://cricsheet.org/downloads/t20s_json.zip")
	require.NoError(t, err)
	assert.Equal(t, "t20s_json.zip", src.Filename)
	assert.Empty(t, src.FeedID, "an explicit url is not a feed")
}

func TestResolveSource_NamedFeedsGoThroughTheSameCheck(t *testing.T) {
	t.Parallel()
	for _, feed := range Feeds() {
		t.Run(feed.ID, func(t *testing.T) {
			t.Parallel()
			src, err := ResolveSource(feed.ID, "")
			require.NoError(t, err, "shipped feed %s must satisfy the allowlist", feed.ID)
			assert.Equal(t, feed.ID, src.FeedID)
			assert.NotEmpty(t, src.Filename)
		})
	}
}

func TestResolveSource_RequiresExactlyOneSource(t *testing.T) {
	t.Parallel()
	_, err := ResolveSource("", "")
	assert.Error(t, err, "neither a feed nor a url")

	_, err = ResolveSource("all", "https://cricsheet.org/downloads/t20s_json.zip")
	assert.Error(t, err, "both a feed and a url is ambiguous, not a default")

	_, err = ResolveSource("not-a-feed", "")
	assert.Error(t, err)
}

// TestArchiveFilename_Rejects covers the layer above zip-slip: the archive's own name
// comes from a URL path, and a URL path may contain anything.
func TestArchiveFilename_Rejects(t *testing.T) {
	t.Parallel()
	bad := map[string]string{
		"percent-encoded traversal": "https://cricsheet.org/downloads/%2e%2e%2fescape.zip",
		"encoded separator":         "https://cricsheet.org/downloads/sub%2Fnested.zip",
		"no filename":               "https://cricsheet.org/downloads/",
		"bare root":                 "https://cricsheet.org/",
		"not a zip":                 "https://cricsheet.org/downloads/all_json.tar.gz",
		"no extension":              "https://cricsheet.org/downloads/all_json",
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(raw)
			require.NoError(t, err)
			_, err = ArchiveFilename(u)
			assert.Error(t, err, "%s must not yield a filename", raw)
		})
	}
}

// TestArchiveFilename_NeverEscapesTheStagingDirectory states the property the caller
// actually depends on. A dotted path is not necessarily an attack — path.Clean
// collapses ../../../etc/passwd.zip to the plain name passwd.zip, which lands inside
// staging and is harmless. What must never happen is a result that, joined to the
// staging directory, points somewhere else.
func TestArchiveFilename_NeverEscapesTheStagingDirectory(t *testing.T) {
	t.Parallel()
	const staging = "/var/data/_staging"
	raws := []string{
		"https://cricsheet.org/downloads/../../../etc/passwd.zip",
		"https://cricsheet.org/downloads/./all_json.zip",
		"https://cricsheet.org/a/b/c/../../t20s_json.zip",
		"https://cricsheet.org/downloads/all_json.zip",
	}
	for _, raw := range raws {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		name, err := ArchiveFilename(u)
		require.NoError(t, err, "%s should clean to a plain name", raw)

		assert.NotContains(t, name, "/", "%s produced a separator", raw)
		assert.NotContains(t, name, "..", "%s produced a dotted segment", raw)
		joined := filepath.Clean(filepath.Join(staging, name))
		assert.Equal(t, staging, filepath.Dir(joined), "%s escaped staging as %s", raw, joined)
	}
}

func TestArchiveFilename_KeepsAPlainArchiveName(t *testing.T) {
	t.Parallel()
	u, err := url.Parse("https://cricsheet.org/downloads/all_json.zip")
	require.NoError(t, err)
	name, err := ArchiveFilename(u)
	require.NoError(t, err)
	assert.Equal(t, "all_json.zip", name)
}

func TestHostAllowed_StripsPortAndTrailingDot(t *testing.T) {
	t.Parallel()
	assert.True(t, hostAllowed("cricsheet.org"))
	assert.True(t, hostAllowed("cricsheet.org:443"))
	assert.True(t, hostAllowed("CricSheet.ORG"))
	assert.True(t, hostAllowed("cricsheet.org."), "the FQDN root dot names the same host")
	assert.False(t, hostAllowed("cricsheet.org.evil.example"))
	assert.False(t, hostAllowed(""))
}
