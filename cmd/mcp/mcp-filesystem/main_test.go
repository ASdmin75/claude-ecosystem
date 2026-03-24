package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidatePath_EmptyAllowedDirs(t *testing.T) {
	// Save and restore allowedDirs.
	orig := allowedDirs
	defer func() { allowedDirs = orig }()

	allowedDirs = nil

	abs, err := validatePath("/any/random/path")
	if err != nil {
		t.Fatalf("expected no error with empty allowedDirs, got: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Errorf("expected absolute path, got %q", abs)
	}
}

func TestValidatePath_WithinAllowedDir(t *testing.T) {
	orig := allowedDirs
	defer func() { allowedDirs = orig }()

	allowed := t.TempDir()
	allowedDirs = []string{allowed}

	sub := filepath.Join(allowed, "subdir", "file.txt")
	got, err := validatePath(sub)
	if err != nil {
		t.Fatalf("expected path within allowed dir to succeed, got: %v", err)
	}
	if got != filepath.Clean(sub) {
		t.Errorf("got %q, want %q", got, filepath.Clean(sub))
	}
}

func TestValidatePath_ExactAllowedDir(t *testing.T) {
	orig := allowedDirs
	defer func() { allowedDirs = orig }()

	allowed := t.TempDir()
	allowedDirs = []string{allowed}

	got, err := validatePath(allowed)
	if err != nil {
		t.Fatalf("expected exact allowed dir to succeed, got: %v", err)
	}
	if got != filepath.Clean(allowed) {
		t.Errorf("got %q, want %q", got, filepath.Clean(allowed))
	}
}

func TestValidatePath_OutsideAllowedDir(t *testing.T) {
	orig := allowedDirs
	defer func() { allowedDirs = orig }()

	allowedDirs = []string{"/home/allowed"}

	_, err := validatePath("/etc/passwd")
	if err == nil {
		t.Fatal("expected error for path outside allowed dirs")
	}
}

func TestValidatePath_TraversalAttempt(t *testing.T) {
	orig := allowedDirs
	defer func() { allowedDirs = orig }()

	allowed := t.TempDir()
	allowedDirs = []string{allowed}

	// Attempt to escape via ".."
	traversal := filepath.Join(allowed, "..", "etc", "passwd")
	_, err := validatePath(traversal)
	if err == nil {
		t.Fatalf("expected error for path traversal attempt: %s", traversal)
	}
}

func TestValidatePath_TraversalEtcPasswd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific path test")
	}
	orig := allowedDirs
	defer func() { allowedDirs = orig }()

	allowedDirs = []string{"/home/allowed"}

	_, err := validatePath("/home/allowed/../etc/passwd")
	if err == nil {
		t.Fatal("expected error for /home/allowed/../etc/passwd traversal")
	}
}

func TestValidatePath_MultipleAllowedDirs(t *testing.T) {
	orig := allowedDirs
	defer func() { allowedDirs = orig }()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	allowedDirs = []string{dir1, dir2}

	// Path in second allowed dir should work.
	sub := filepath.Join(dir2, "file.txt")
	_, err := validatePath(sub)
	if err != nil {
		t.Fatalf("expected path in second allowed dir to succeed, got: %v", err)
	}
}
