package server

import (
	"log"
	"net/http"
	"os"
)

// authMiddleware checks for a valid API key in the X-API-Key header.
// The expected key is read from the API_KEY environment variable.
// If API_KEY is not set, the middleware blocks all requests for safety (fail-secure).
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedKey := os.Getenv("API_KEY")
		if expectedKey == "" {
			// In local development, use a hardcoded default key if API_KEY is not set.
			// This matches the key used by the frontend for local dev login.
			expectedKey = "dev-local-key"
			log.Printf("API_KEY environment variable not set, using default dev-local-key")
		}

		clientKey := r.Header.Get("X-API-Key")
		if clientKey == "" || clientKey != expectedKey {
			log.Printf("Unauthorized access attempt from %s to %s", r.RemoteAddr, r.URL.Path)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
