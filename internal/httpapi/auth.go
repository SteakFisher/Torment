package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// requireAPIKey serves next only when the request's Authorization header is "Bearer <key>" for one of
// keys, and responds 401 otherwise. Keys are compared as SHA-256 sums in constant time, so the response
// time reveals neither which bytes matched nor the key's length.
func requireAPIKey(keys []string, next http.Handler) http.Handler {
	sums := make([][sha256.Size]byte, len(keys))
	for i, k := range keys {
		sums[i] = sha256.Sum256([]byte(k))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		sum := sha256.Sum256([]byte(key))
		match := 0
		for _, s := range sums {
			match |= subtle.ConstantTimeCompare(sum[:], s[:])
		}
		if !ok || match == 0 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
