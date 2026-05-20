package httpinfra

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// ValidateBearerToken checks the Authorization header for a valid Bearer token
// using constant-time comparison to prevent timing attacks.
func ValidateBearerToken(r *http.Request, expected string) bool {
	if expected == "" {
		return false
	}
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) == 1
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
