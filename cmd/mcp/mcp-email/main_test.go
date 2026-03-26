package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/safepath"
	"gopkg.in/gomail.v2"
)

func TestAttachFiles_ValidPath(t *testing.T) {
	dir := t.TempDir()
	var err error
	pathValidator, err = safepath.New(dir)
	if err != nil {
		t.Fatalf("safepath.New: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(dir, "report.xlsx")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	m := gomail.NewMessage()
	args := map[string]any{
		"attachments": []any{testFile},
	}

	if err := attachFiles(m, args, "attachments"); err != nil {
		t.Fatalf("attachFiles returned error for valid path: %v", err)
	}
}

func TestAttachFiles_TraversalRejected(t *testing.T) {
	dir := t.TempDir()
	var err error
	pathValidator, err = safepath.New(dir)
	if err != nil {
		t.Fatalf("safepath.New: %v", err)
	}

	m := gomail.NewMessage()
	args := map[string]any{
		"attachments": []any{filepath.Join(dir, "..", "..", "etc", "passwd")},
	}

	err = attachFiles(m, args, "attachments")
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
	if !strings.Contains(err.Error(), "attachment path rejected") {
		t.Errorf("expected 'attachment path rejected' error, got: %v", err)
	}
}

func TestAttachFiles_AbsolutePathOutside(t *testing.T) {
	dir := t.TempDir()
	var err error
	pathValidator, err = safepath.New(dir)
	if err != nil {
		t.Fatalf("safepath.New: %v", err)
	}

	m := gomail.NewMessage()
	args := map[string]any{
		"attachments": []any{"/etc/passwd"},
	}

	err = attachFiles(m, args, "attachments")
	if err == nil {
		t.Fatal("expected error for absolute path outside allowed dir")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("expected 'access denied' error, got: %v", err)
	}
}
