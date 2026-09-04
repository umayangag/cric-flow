package biography_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// sparqlBody is a two-person answer in the shape the query service returns, including the
// duplicate bindings a person with two bowling styles produces.
const sparqlBody = `{"results":{"bindings":[
 {"ci":{"value":"35320"},"item":{"value":"http://www.wikidata.org/entity/Q9200"},
  "dob":{"value":"1973-04-24T00:00:00Z"},"playLabel":{"value":"right-handedness"},
  "styleLabel":{"value":"leg break"}},
 {"ci":{"value":"35320"},"item":{"value":"http://www.wikidata.org/entity/Q9200"},
  "dob":{"value":"1973-04-24T00:00:00Z"},"styleLabel":{"value":"off break"}},
 {"ci":{"value":"8166"},"item":{"value":"http://www.wikidata.org/entity/Q311806"},
  "dod":{"value":"2022-03-04T00:00:00Z"},"end":{"value":"2013-01-01T00:00:00Z"},
  "handLabel":{"value":"left-handedness"}}
]}}`

// TestBuildQuery_InterpolatesOnlyAlphanumericIDs is the injection guard: the register is a
// third-party file, and a quotation mark in it must not reach the query.
func TestBuildQuery_InterpolatesOnlyAlphanumericIDs(t *testing.T) {
	t.Parallel()

	query := biography.BuildQuery([]string{"35320", `"} UNION {?item ?p ?o} #`, "8166"})

	assert.Contains(t, query, `"35320"`)
	assert.Contains(t, query, `"8166"`)
	assert.NotContains(t, query, "UNION")
	assert.Contains(t, query, "wdt:P2697", "the join property is the Cricinfo player id")
}

// TestDecodeLookups_MergesRepeatedBindingsDeterministically is what makes two runs over
// the same data write the same rows.
func TestDecodeLookups_MergesRepeatedBindingsDeterministically(t *testing.T) {
	t.Parallel()

	lookups, err := biography.DecodeLookups(strings.NewReader(sparqlBody))

	require.NoError(t, err)
	require.Len(t, lookups, 2)

	tendulkar := lookups["35320"]
	assert.Equal(t, "Q9200", tendulkar.QID)
	require.NotNil(t, tendulkar.BirthDate)
	assert.Equal(t, "1973-04-24", tendulkar.BirthDate.Format(time.DateOnly))
	assert.Equal(t, "right-handedness", tendulkar.BattingHandRaw)
	assert.Equal(t, "leg break", tendulkar.BowlingStyleRaw,
		"two styles resolve by sorted order, not by the order the server happened to send")

	warne := lookups["8166"]
	require.NotNil(t, warne.DeathDate)
	require.NotNil(t, warne.CareerEndDate)
	assert.Equal(t, "2013-01-01", warne.CareerEndDate.Format(time.DateOnly))
	assert.Equal(t, "left-handedness", warne.BattingHandRaw,
		"the general handedness property is read when the sport-specific one is absent")
}

// TestDecodeLookups_RejectsAnUnparseableBody surfaces a proxy's HTML error page as an
// error rather than as an empty, successful-looking result.
func TestDecodeLookups_RejectsAnUnparseableBody(t *testing.T) {
	t.Parallel()

	_, err := biography.DecodeLookups(strings.NewReader("<html>502</html>"))

	require.Error(t, err)
}

// TestSPARQLClientQuery_SendsTheQueryAndDecodesTheAnswer exercises the whole request.
func TestSPARQLClientQuery_SendsTheQueryAndDecodesTheAnswer(t *testing.T) {
	t.Parallel()

	var sawUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUserAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(sparqlBody))
	}))
	t.Cleanup(server.Close)

	client := &biography.SPARQLClient{Endpoint: server.URL, UserAgent: "cric-flow-test"}
	lookups, err := client.Query(context.Background(), []string{"35320", "8166"})

	require.NoError(t, err)
	assert.Len(t, lookups, 2)
	assert.Equal(t, "cric-flow-test", sawUserAgent,
		"the query service asks callers to identify themselves")
}

// TestSPARQLClientQuery_RefusesToRunWithoutAUserAgent keeps the run from being the
// anonymous traffic the service asks callers not to send.
func TestSPARQLClientQuery_RefusesToRunWithoutAUserAgent(t *testing.T) {
	t.Parallel()

	client := &biography.SPARQLClient{Endpoint: "http://example.invalid"}
	_, err := client.Query(context.Background(), []string{"1"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "User-Agent")
}

// TestSPARQLClientQuery_RetriesATransientFailure is the difference between a run that is
// resumable and one that is reliable: the public service returns the odd 502 through no
// fault of the caller's.
func TestSPARQLClientQuery_RetriesATransientFailure(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(sparqlBody))
	}))
	t.Cleanup(server.Close)

	client := &biography.SPARQLClient{
		Endpoint: server.URL, UserAgent: "cric-flow-test", Attempts: 2,
	}
	lookups, err := client.Query(context.Background(), []string{"35320"})

	require.NoError(t, err)
	assert.Len(t, lookups, 2)
	assert.EqualValues(t, 2, calls.Load())
}

// TestSPARQLClientQuery_DoesNotRetryAMalformedQuery: a 400 will fail identically however
// many times it is sent, so retrying it only delays the error.
func TestSPARQLClientQuery_DoesNotRetryAMalformedQuery(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	client := &biography.SPARQLClient{Endpoint: server.URL, UserAgent: "cric-flow-test"}
	_, err := client.Query(context.Background(), []string{"35320"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.EqualValues(t, 1, calls.Load())
}

// TestSPARQLClientQuery_GivesUpAfterTheLastAttempt reports the failure rather than
// retrying forever.
func TestSPARQLClientQuery_GivesUpAfterTheLastAttempt(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	client := &biography.SPARQLClient{
		Endpoint: server.URL, UserAgent: "cric-flow-test", Attempts: 1,
	}
	_, err := client.Query(context.Background(), []string{"35320"})

	require.Error(t, err)
	assert.EqualValues(t, 1, calls.Load())
}
