package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOAuth2TokenManager_Authenticate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if body["client_id"] != "test-id" || body["client_secret"] != "test-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
			return
		}

		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer server.Close()

	tm := newOAuth2TokenManager("test-id", "test-secret", server.URL, "", &http.Client{Timeout: 5 * time.Second})
	if err := tm.authenticate(); err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}

	if tm.accessToken != "access-123" {
		t.Errorf("expected access-123, got %s", tm.accessToken)
	}
	if tm.refreshToken != "refresh-456" {
		t.Errorf("expected refresh-456, got %s", tm.refreshToken)
	}
	if tm.expiresAt.IsZero() {
		t.Error("expiresAt should be set")
	}
}

func TestOAuth2TokenManager_Refresh(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if body["grant_type"] == "refresh_token" && body["refresh_token"] == "refresh-456" {
			json.NewEncoder(w).Encode(tokenResponse{
				AccessToken:  "access-new",
				RefreshToken: "refresh-new",
				ExpiresIn:    3600,
			})
			return
		}

		// Initial auth
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	tm := newOAuth2TokenManager("test-id", "test-secret", server.URL, server.URL, &http.Client{Timeout: 5 * time.Second})
	if err := tm.authenticate(); err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}

	if err := tm.refresh(); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if tm.accessToken != "access-new" {
		t.Errorf("expected access-new, got %s", tm.accessToken)
	}
	if tm.refreshToken != "refresh-new" {
		t.Errorf("expected refresh-new, got %s", tm.refreshToken)
	}
}

func TestOAuth2TokenManager_GetToken_ProactiveRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "fresh-token",
			ExpiresIn:   3600,
		})
	}))
	defer server.Close()

	tm := newOAuth2TokenManager("id", "secret", server.URL, "", &http.Client{Timeout: 5 * time.Second})
	tm.accessToken = "old-token"
	tm.expiresAt = time.Now().Add(10 * time.Second) // expires in 10s < 30s threshold

	token, err := tm.getToken()
	if err != nil {
		t.Fatalf("getToken failed: %v", err)
	}
	if token != "fresh-token" {
		t.Errorf("expected fresh-token, got %s", token)
	}
}

func TestOAuth2TokenManager_GetToken_Valid(t *testing.T) {
	tm := &oauth2TokenManager{
		accessToken: "valid-token",
		expiresAt:   time.Now().Add(5 * time.Minute),
	}

	token, err := tm.getToken()
	if err != nil {
		t.Fatalf("getToken failed: %v", err)
	}
	if token != "valid-token" {
		t.Errorf("expected valid-token, got %s", token)
	}
}

func TestOAuth2TokenManager_AuthenticateBadCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer server.Close()

	tm := newOAuth2TokenManager("bad", "creds", server.URL, "", &http.Client{Timeout: 5 * time.Second})
	err := tm.authenticate()
	if err == nil {
		t.Fatal("expected error for bad credentials")
	}
}

func TestOAuth2TokenManager_RefreshFallsBackToReAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if body["grant_type"] == "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}

		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "re-authed-token",
			ExpiresIn:   3600,
		})
	}))
	defer server.Close()

	tm := newOAuth2TokenManager("id", "secret", server.URL, server.URL, &http.Client{Timeout: 5 * time.Second})
	tm.accessToken = "old"
	tm.refreshToken = "expired-refresh"

	if err := tm.refresh(); err != nil {
		t.Fatalf("refresh should fall back to re-auth: %v", err)
	}
	if tm.accessToken != "re-authed-token" {
		t.Errorf("expected re-authed-token, got %s", tm.accessToken)
	}
}
