package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/safepath"
	"github.com/mark3labs/mcp-go/mcp"
)

func setupDownloadTest(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()

	// Set up the download validator
	var err error
	downloadValidator, err = safepath.New(dir)
	if err != nil {
		t.Fatalf("safepath.New: %v", err)
	}

	// Mock HTTP server serving a small file
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test file content"))
	}))

	// Set up httpClient to use the test server
	httpClient = ts.Client()

	return dir, ts
}

func TestDownloadFile_ValidPath(t *testing.T) {
	dir, ts := setupDownloadTest(t)
	defer ts.Close()

	dest := filepath.Join(dir, "output.bin")
	written, err := downloadOneFile(context.Background(), ts.URL+"/file", dest)
	if err != nil {
		t.Fatalf("downloadOneFile: %v", err)
	}
	if written != 17 { // len("test file content")
		t.Errorf("expected 17 bytes, got %d", written)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != "test file content" {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestDownloadFile_TraversalRejected(t *testing.T) {
	dir, ts := setupDownloadTest(t)
	defer ts.Close()

	// Try to escape the allowed directory
	malicious := filepath.Join(dir, "..", "..", "etc", "evil.bin")
	_, err := downloadOneFile(context.Background(), ts.URL+"/file", malicious)
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
	if !strings.Contains(err.Error(), "path validation") {
		t.Errorf("expected 'path validation' error, got: %v", err)
	}
}

func TestDownloadFile_AbsolutePathOutside(t *testing.T) {
	_, ts := setupDownloadTest(t)
	defer ts.Close()

	_, err := downloadOneFile(context.Background(), ts.URL+"/file", "/tmp/evil.bin")
	if err == nil {
		t.Fatal("expected error for absolute path outside allowed dir")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("expected 'access denied' error, got: %v", err)
	}
}

func TestBatchDownload_MixedPaths(t *testing.T) {
	dir, ts := setupDownloadTest(t)
	defer ts.Close()

	validPath := filepath.Join(dir, "good.bin")
	maliciousPath := filepath.Join(dir, "..", "..", "etc", "evil.bin")

	result, err := batchDownloadHandler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"files": []any{
					map[string]any{"url": ts.URL + "/file1", "path": validPath},
					map[string]any{"url": ts.URL + "/file2", "path": maliciousPath},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("batchDownloadHandler: %v", err)
	}

	// Result should indicate 1 success, 1 failure
	text := ""
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "1 success") {
		t.Errorf("expected 1 success in result: %s", text)
	}
	if !strings.Contains(text, "1 failed") {
		t.Errorf("expected 1 failure in result: %s", text)
	}

	// Valid file should exist
	if _, err := os.Stat(validPath); err != nil {
		t.Errorf("valid file should exist: %v", err)
	}
}
