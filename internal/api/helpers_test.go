package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadJSON_RejectsOversizedBody(t *testing.T) {
	// Create a body larger than maxRequestBodySize (1MB).
	body := bytes.NewReader(make([]byte, maxRequestBodySize+1))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")

	var target map[string]any
	err := readJSON(req, &target)
	if err == nil {
		t.Fatal("expected error for oversized body")
	}
	// http.MaxBytesReader returns an error containing "http: request body too large".
	if !strings.Contains(err.Error(), "too large") && !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadJSON_ValidBody(t *testing.T) {
	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")

	var target struct {
		Name string `json:"name"`
	}
	if err := readJSON(req, &target); err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	if target.Name != "test" {
		t.Errorf("name: got %q, want %q", target.Name, "test")
	}
}

func TestReadJSON_NilBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = nil

	var target map[string]any
	err := readJSON(req, &target)
	if err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestReadJSON_RejectsUnknownFields(t *testing.T) {
	body := strings.NewReader(`{"name":"test","unknown_field":true}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)

	var target struct {
		Name string `json:"name"`
	}
	err := readJSON(req, &target)
	if err == nil {
		t.Fatal("expected error for unknown fields")
	}
}

func TestWriteJSON_SecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want %q", got, "nosniff")
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options: got %q, want %q", got, "DENY")
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", got, "application/json")
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control: got %q, want %q", got, "no-store")
	}
}

func TestWriteJSON_StatusCode(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"id": "1"})

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestWriteJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNoContent)
	}
	// Body should be empty when data is nil.
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "something went wrong") {
		t.Errorf("body missing error message: %s", w.Body.String())
	}
}
