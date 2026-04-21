package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// authMiddleware validates API key authentication.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Sidekick-Key")
		if key == "" {
			key = extractBearerToken(r)
		}

		if subtle.ConstantTimeCompare([]byte(key), []byte(s.apiKey)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts a token from the Authorization: Bearer header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
