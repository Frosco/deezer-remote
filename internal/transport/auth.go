package transport

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireToken returns middleware that admits requests carrying either
// `Authorization: Bearer <expected>` or `?t=<expected>`. Mismatches return
// HTTP 401 with a plain-text body. Constant-time compare avoids leaking
// token bytes via timing.
func RequireToken(expected string) func(http.Handler) http.Handler {
	want := []byte(expected)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if matchHeader(r, want) || matchQuery(r, want) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

func matchHeader(r *http.Request, want []byte) bool {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(h, "Bearer ")), want) == 1
}

func matchQuery(r *http.Request, want []byte) bool {
	got := r.URL.Query().Get("t")
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), want) == 1
}
