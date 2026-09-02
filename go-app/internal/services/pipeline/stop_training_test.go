package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mlStopServer stands in for ml-service's stop endpoint, recording what it was asked.
func mlStopServer(t *testing.T, status int, body string) (path *string, apiKey *string) {
	t.Helper()
	gotPath, gotKey := new(string), new(string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		*gotKey = r.Header.Get("X-API-Key")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	t.Setenv("ML_SERVICE_URL", server.URL)
	return gotPath, gotKey
}

func TestStopMLTraining_ReportsTheStepsMLServiceConfirmedStopped(t *testing.T) {
	path, _ := mlStopServer(t, http.StatusOK, `{"status":"ok","stopped":["retrain"]}`)

	stopped, err := StopMLTraining(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"retrain"}, stopped)
	assert.Equal(t, MLStopPath, *path)
}

// Nothing running is a true answer, not a failure: a Stop pressed on an idle pipeline is
// an ordinary thing to do.
func TestStopMLTraining_NothingRunningIsNotAnError(t *testing.T) {
	mlStopServer(t, http.StatusOK, `{"status":"ok","stopped":[]}`)

	stopped, err := StopMLTraining(context.Background())

	require.NoError(t, err)
	assert.Empty(t, stopped)
}

func TestStopMLTraining_SendsTheAdminKey(t *testing.T) {
	_, key := mlStopServer(t, http.StatusOK, `{"stopped":[]}`)
	t.Setenv("API_KEY", "dev-local-key")

	_, err := StopMLTraining(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "dev-local-key", *key)
}

func TestStopMLTraining_AnErrorStatusIsAnError(t *testing.T) {
	mlStopServer(t, http.StatusInternalServerError, `{"detail":{"code":"BOOM"}}`)

	_, err := StopMLTraining(context.Background())

	require.Error(t, err, "a stop that did not happen must not read as one")
}

// The body is the only evidence the process is gone, so an unreadable one is a failure
// rather than something to shrug off the way an unreadable run summary is.
func TestStopMLTraining_AnUnreadableBodyIsAnError(t *testing.T) {
	mlStopServer(t, http.StatusOK, `not json`)

	_, err := StopMLTraining(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreadable stop response")
}

// The heart of D-11's go-app half: the context being stopped is usually the one about to
// be cancelled, so a stop that inherited its cancellation would be a stop that never
// happened — which is precisely the state the console used to report success from.
func TestStopMLTraining_SurvivesACancelledCallerContext(t *testing.T) {
	mlStopServer(t, http.StatusOK, `{"stopped":["retrain"]}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stopped, err := StopMLTraining(ctx)

	require.NoError(t, err, "a stop must outlive the request it is stopping")
	assert.Equal(t, []string{"retrain"}, stopped)
}

func TestStopMLTraining_AnUnreachableServiceIsAnError(t *testing.T) {
	// A port nothing is listening on: the far side being down must not read as "stopped".
	t.Setenv("ML_SERVICE_URL", "http://127.0.0.1:1")

	_, err := StopMLTraining(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ask ml-service to stop training")
}
