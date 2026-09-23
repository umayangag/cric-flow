package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mlResponse builds the *http.Response logMLNon2xx reads: a status and a JSON body, as
// ml-service would actually send them.
func mlResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

// GO-12: a 422 whose "detail" is FastAPI's own default validation-error array -- what a
// pydantic field_validator on a request model raises when nothing in this service
// overrides the handler -- must relay as a structured, actionable error, not fall through
// to respondErr's generic 500 INTERNAL.
func TestLogMLNon2xx_FastAPIValidationArray_RelaysAsStructuredError(t *testing.T) {
	t.Parallel()
	resp := mlResponse(422, `{"detail":[{"loc":["body","constraints","team_size"],`+
		`"msg":"team_size is 11: no other side size is supported by the served models","type":"value_error"}]}`)

	err := logMLNon2xx(resp, "/xi/optimize")

	var mlErr *mlServiceError
	require.True(t, errors.As(err, &mlErr), "must be a *mlServiceError, not a bare error respondErr would answer as 500")
	assert.Equal(t, 422, mlErr.Status)
	assert.Equal(t, "VALIDATION_ERROR", mlErr.Code)
	assert.Contains(t, mlErr.Message, "body.constraints.team_size")
	assert.Contains(t, mlErr.Message, "no other side size is supported")
}

// A 4xx with several validation issues joins them, dropping none.
func TestLogMLNon2xx_FastAPIValidationArray_JoinsEveryIssue(t *testing.T) {
	t.Parallel()
	resp := mlResponse(422, `{"detail":[
		{"loc":["body","format"],"msg":"field required","type":"missing"},
		{"loc":["body","pool_player_ids"],"msg":"list has too few items","type":"too_short"}
	]}`)

	err := logMLNon2xx(resp, "/xi/optimize")

	var mlErr *mlServiceError
	require.True(t, errors.As(err, &mlErr))
	assert.Contains(t, mlErr.Message, "body.format: field required")
	assert.Contains(t, mlErr.Message, "body.pool_player_ids: list has too few items")
}

// The existing structured shape -- {"detail": {"code","message","hint"}}, which SERVE-06's
// 409 TRAIN_ALREADY_RUNNING and this service's own 4xx/503 refusals already send -- must
// keep relaying exactly as it did before this fix.
func TestLogMLNon2xx_StructuredDetail_StillRelaysCodeMessageHint(t *testing.T) {
	t.Parallel()
	resp := mlResponse(409, `{"detail":{"code":"TRAIN_ALREADY_RUNNING","message":"a training run is already in progress","hint":"wait for it to finish"}}`)

	err := logMLNon2xx(resp, "/train/start")

	var mlErr *mlServiceError
	require.True(t, errors.As(err, &mlErr))
	assert.Equal(t, 409, mlErr.Status)
	assert.Equal(t, "TRAIN_ALREADY_RUNNING", mlErr.Code)
	assert.Equal(t, "a training run is already in progress", mlErr.Message)
	assert.Equal(t, "wait for it to finish", mlErr.Hint)
}

// A plain string detail keeps its own path too: a bare error, not a *mlServiceError,
// because it is not a caller-actionable structured shape.
func TestLogMLNon2xx_StringDetail_ReturnsABareError(t *testing.T) {
	t.Parallel()
	resp := mlResponse(400, `{"detail":"model not loaded"}`)

	err := logMLNon2xx(resp, "/xi/predict-win")

	var mlErr *mlServiceError
	assert.False(t, errors.As(err, &mlErr))
	assert.Contains(t, err.Error(), "model not loaded")
}

// The VALIDATION_ERROR fallback is for a caller's own mistake (4xx). A 5xx with the same
// unstructured shape is ml-service's own failure, not the caller's, so it must not be
// relabelled as a validation error -- it keeps falling through to the generic path, which
// respondErr maps to 502 (relayStatus), not a fabricated caller-facing code.
func TestLogMLNon2xx_UnstructuredDetailOn5xx_DoesNotBecomeValidationError(t *testing.T) {
	t.Parallel()
	resp := mlResponse(500, `{"detail":[{"loc":["body"],"msg":"unexpected failure","type":"internal"}]}`)

	err := logMLNon2xx(resp, "/xi/optimize")

	var mlErr *mlServiceError
	assert.False(t, errors.As(err, &mlErr))
	assert.Contains(t, err.Error(), "500")
}
