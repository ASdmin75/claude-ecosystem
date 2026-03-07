package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

// UserKey is the context key used to store the authenticated username.
const UserKey contextKey = "user"

// Middleware provides HTTP middleware for authentication.
// It tries PASETO token validation first, then falls back to bearer token validation.
type Middleware struct {
	paseto *PASETOManager
	bearer *BearerAuth
}

// NewMiddleware creates a new authentication middleware.
func NewMiddleware(paseto *PASETOManager, bearer *BearerAuth) *Middleware {
	return &Middleware{
		paseto: paseto,
		bearer: bearer,
	}
}

// Handler returns an http.Handler that checks the Authorization header.
// It tries PASETO first, then bearer. On success it sets the username in
// the request context under UserKey. On failure it returns 401 Unauthorized.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			// No "Bearer " prefix found.
			http.Error(w, "invalid authorization format", http.StatusUnauthorized)
			return
		}

		// Try PASETO validation first.
		if m.paseto != nil {
			claims, err := m.paseto.ValidateToken(token)
			if err == nil {
				ctx := context.WithValue(r.Context(), UserKey, claims.Subject)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Fall back to bearer token validation.
		if m.bearer != nil && m.bearer.Validate(token) {
			ctx := context.WithValue(r.Context(), UserKey, "bearer")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
