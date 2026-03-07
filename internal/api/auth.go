package api

import (
	"net/http"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

const tokenDuration = 24 * time.Hour

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
