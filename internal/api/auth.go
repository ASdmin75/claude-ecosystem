package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

const tokenDuration = 24 * time.Hour

// loginRateLimiter implements a simple per-IP rate limiter for login attempts.
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	window   time.Duration
	max      int
}

func newLoginRateLimiter(window time.Duration, max int) *loginRateLimiter {
	return &loginRateLimiter{
		attempts: make(map[string][]time.Time),
		window:   window,
		max:      max,
	}
}

func (rl *loginRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Clean old entries
	recent := rl.attempts[ip][:0]
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.max {
		rl.attempts[ip] = recent
		return false
	}

	rl.attempts[ip] = append(recent, now)
	return true
}

var loginLimiter = newLoginRateLimiter(15*time.Minute, 10)

// loginRequest is the JSON body for POST /api/v1/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// tokenResponse is returned on successful login or refresh.
type tokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleLogin authenticates a user and returns a PASETO token.
// POST /api/v1/auth/login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Rate limit login attempts per IP.
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = fwd
	}
	if !loginLimiter.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := s.authStore.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		s.logger.Warn("login failed: user not found", "username", req.Username)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.logger.Warn("login failed: bad password", "username", req.Username)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := s.paseto.CreateToken(user.Username, tokenDuration)
	if err != nil {
		s.logger.Error("failed to create token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	expiresAt := time.Now().UTC().Add(tokenDuration)
	s.logger.Info("user logged in", "username", req.Username)
	writeJSON(w, http.StatusOK, tokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	})
}

// handleRefresh issues a new PASETO token for the authenticated user.
// POST /api/v1/auth/refresh
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	username, ok := r.Context().Value(auth.UserKey).(string)
	if !ok || username == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	token, err := s.paseto.CreateToken(username, tokenDuration)
	if err != nil {
		s.logger.Error("failed to create refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	expiresAt := time.Now().UTC().Add(tokenDuration)
	writeJSON(w, http.StatusOK, tokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	})
}
