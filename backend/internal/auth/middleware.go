package auth

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type contextKey int

const claimsKey contextKey = 0

// FromContext retrieves the Claims stored by AuthMiddleware.
// Returns nil when the request was not authenticated.
func FromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// AuthMiddleware validates the Bearer access token in the Authorization header.
// It also verifies the token's session ID against the sessions table, which
// allows immediate revocation without waiting for the access token to expire.
//
// On success the validated Claims are stored in the request context and the
// next handler is called.  On failure a 401 JSON response is written.
//
// To protect the full API:
//
//	r.Use(auth.AuthMiddleware(cfg, sessions))
func AuthMiddleware(cfg Config, sessions *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			claims, err := cfg.ValidateAccessToken(raw)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			// One SQLite read per request; WAL mode supports concurrent readers.
			row, err := sessions.Get(r.Context(), claims.ID)
			if err != nil || row.Revoked || row.ExpiresAt.Before(time.Now()) {
				http.Error(w, `{"error":"session expired or revoked"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns a middleware that permits only requests whose JWT role
// is in the provided list.  Must be chained after AuthMiddleware.
//
// Defined roles and their intended access levels:
//
//	superadmin — full access including user management and config writes
//	operator   — read + trigger reloads + downstream SCADA commands
//	viewer     — read-only access to status, history, and monitor streams
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := FromContext(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if _, ok := allowed[claims.Role]; !ok {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
