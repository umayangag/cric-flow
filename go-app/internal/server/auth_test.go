package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthMiddleware(t *testing.T) {
	// Simple handler that just returns 200 OK
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handlerToTest := authMiddleware(nextHandler)

	t.Run("Use default key when API_KEY is not set", func(t *testing.T) {
		os.Unsetenv("API_KEY")

		req := httptest.NewRequest(http.MethodGet, "/any-endpoint", nil)
		req.Header.Set("X-API-Key", "dev-local-key")
		rr := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Unauthorized when X-API-Key header is missing", func(t *testing.T) {
		os.Setenv("API_KEY", "secret-key")
		defer os.Unsetenv("API_KEY")

		req := httptest.NewRequest(http.MethodGet, "/any-endpoint", nil)
		rr := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Unauthorized when X-API-Key is incorrect", func(t *testing.T) {
		os.Setenv("API_KEY", "secret-key")
		defer os.Unsetenv("API_KEY")

		req := httptest.NewRequest(http.MethodGet, "/any-endpoint", nil)
		req.Header.Set("X-API-Key", "wrong-key")
		rr := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Authorized when X-API-Key is correct", func(t *testing.T) {
		os.Setenv("API_KEY", "secret-key")
		defer os.Unsetenv("API_KEY")

		req := httptest.NewRequest(http.MethodGet, "/any-endpoint", nil)
		req.Header.Set("X-API-Key", "secret-key")
		rr := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
