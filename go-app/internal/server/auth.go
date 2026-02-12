package server

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// authMiddleware checks for a valid API key in the X-API-Key header.
// The expected key is read from the API_KEY environment variable.
// If API_KEY is not set, the middleware blocks all requests for safety (fail-secure).
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		expectedKey := strings.TrimSpace(os.Getenv("API_KEY"))
		if expectedKey == "" {
			slog.Error("API_KEY environment variable not set. Access denied.")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		clientKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if clientKey == "" || clientKey != expectedKey {
			slog.Warn("Unauthorized access attempt",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote", r.RemoteAddr),
				slog.Bool("has_key", clientKey != ""),
				slog.Int("client_key_len", len(clientKey)),
				slog.Int("expected_key_len", len(expectedKey)),
			)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
