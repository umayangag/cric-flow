package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCorsMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		method         string
		envOrigin      string
		wantOrigin     string
		wantStatus     int
		wantInnerCall  bool
	}{
		{
			name:          "get_request_default_origin",
			method:        http.MethodGet,
			envOrigin:     "",
			wantOrigin:    "http://localhost:5173",
			wantStatus:    http.StatusOK,
			wantInnerCall: true,
		},
		{
			name:          "get_request_custom_origin",
			method:        http.MethodGet,
			envOrigin:     "https://example.com",
			wantOrigin:    "https://example.com",
			wantStatus:    http.StatusOK,
			wantInnerCall: true,
		},
		{
			name:          "options_preflight_returns_no_content",
			method:        http.MethodOptions,
			envOrigin:     "",
			wantOrigin:    "http://localhost:5173",
			wantStatus:    http.StatusNoContent,
			wantInnerCall: false,
		},
		{
			name:          "post_request_passes_through",
			method:        http.MethodPost,
			envOrigin:     "",
			wantOrigin:    "http://localhost:5173",
			wantStatus:    http.StatusOK,
			wantInnerCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FRONTEND_ORIGIN", tt.envOrigin)

			handler := corsMiddleware(inner)
			req := httptest.NewRequest(tt.method, "/api/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
			assert.Equal(t, "Origin", rec.Header().Get("Vary"))
			assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
			assert.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Methods"))
			assert.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Headers"))
		})
	}
}
