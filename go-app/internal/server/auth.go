package server

import (
	"log"
	"net/http"
	"os"
)

// authMiddleware checks for a valid API key in the X-API-Key header.
// The expected key is read from the API_KEY environment variable.
// If API_KEY is not set, the middleware is disabled (allowing all requests).
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedKey := os.Getenv("API_KEY")
		if expectedKey == "" {
			next.ServeHTTP(w, r)
			return
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
