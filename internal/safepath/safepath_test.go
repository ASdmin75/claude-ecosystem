package safepath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNew_DefaultsToWorkingDir(t *testing.T) {
	v, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	wd, _ := os.Getwd()
	dirs := v.AllowedDirs()
	if len(dirs) != 1 || dirs[0] != filepath.Clean(wd) {
		t.Fatalf("expected [%s], got %v", wd, dirs)
	}
}

func TestNew_WithDirs(t *testing.T) {
	dir := t.TempDir()
	v, err := New(dir)
	if err != nil {
		t.Fatalf("New(%s) error: %v", dir, err)
	}
	dirs := v.AllowedDirs()
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}
}

func TestNew_SkipsEmptyStrings(t *testing.T) {
	dir := t.TempDir()
	v, err := New("", dir, "  ")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if len(v.AllowedDirs()) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(v.AllowedDirs()))
	}
}

func TestNewFromEnv_Empty(t *testing.T) {
	t.Setenv("TEST_SAFEPATH_DIRS", "")
	v, err := NewFromEnv("TEST_SAFEPATH_DIRS")
	if err != nil {
		t.Fatalf("NewFromEnv error: %v", err)
	}
	wd, _ := os.Getwd()
	if v.AllowedDirs()[0] != filepath.Clean(wd) {
		t.Fatal("expected working directory as default")
	}
}

func TestNewFromEnv_ColonSeparated(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	t.Setenv("TEST_SAFEPATH_DIRS", d1+":"+d2)
	v, err := NewFromEnv("TEST_SAFEPATH_DIRS")
	if err != nil {
		t.Fatalf("NewFromEnv error: %v", err)
	}
	if len(v.AllowedDirs()) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(v.AllowedDirs()))
	}
}

func TestValidate_WithinAllowedDir(t *testing.T) {
	dir := t.TempDir()
	v, _ := New(dir)

	path := filepath.Join(dir, "subdir", "file.txt")
	result, err := v.Validate(path)
	if err != nil {
		t.Fatalf("Validate(%s) unexpected error: %v", path, err)
	}
	if result != path {
		t.Fatalf("expected %s, got %s", path, result)
	}
}

func TestValidate_ExactDirMatch(t *testing.T) {
	dir := t.TempDir()
	v, _ := New(dir)

	result, err := v.Validate(dir)
	if err != nil {
		t.Fatalf("Validate(%s) unexpected error: %v", dir, err)
	}
	if result != dir {
		t.Fatalf("expected %s, got %s", dir, result)
	}
}

func TestValidate_OutsideAllowedDir(t *testing.T) {
	dir := t.TempDir()
	v, _ := New(dir)

	_, err := v.Validate("/etc/passwd")
	if err == nil {
		t.Fatal("expected error for path outside allowed dir")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected 'access denied' error, got: %v", err)
	}
}

func TestValidate_TraversalAttack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific path test")
	}

	dir := t.TempDir()
	v, _ := New(dir)

	// Attempt to escape via ../
	malicious := filepath.Join(dir, "..", "..", "etc", "passwd")
	_, err := v.Validate(malicious)
	if err == nil {
		t.Fatalf("expected error for traversal path: %s", malicious)
	}
}

func TestValidate_ComplexTraversal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific path test")
	}

	dir := t.TempDir()
	v, _ := New(dir)

	// /tmp/xxx/../../../etc/passwd
	malicious := dir + "/../../../etc/passwd"
	_, err := v.Validate(malicious)
	if err == nil {
		t.Fatal("expected error for complex traversal path")
	}
}

func TestValidate_MultipleDirs(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	v, _ := New(d1, d2)

	// Path in d1 should work
	p1 := filepath.Join(d1, "file.txt")
	if _, err := v.Validate(p1); err != nil {
		t.Fatalf("expected path in d1 to be valid: %v", err)
	}

	// Path in d2 should work
	p2 := filepath.Join(d2, "file.txt")
	if _, err := v.Validate(p2); err != nil {
		t.Fatalf("expected path in d2 to be valid: %v", err)
	}

	// Path outside both should fail
	if _, err := v.Validate("/etc/passwd"); err == nil {
		t.Fatal("expected error for path outside both dirs")
	}
}

func TestValidate_PrefixAttack(t *testing.T) {
	// Ensure /tmp/allowed doesn't match /tmp/allowedx/file
	dir := t.TempDir()
	v, _ := New(dir)

	// Create a sibling dir name that starts with the same prefix
	sibling := dir + "x"
	path := filepath.Join(sibling, "file.txt")
	_, err := v.Validate(path)
	if err == nil {
		t.Fatalf("expected error for prefix-similar path: %s", path)
	}
}

func TestValidate_RelativeInput(t *testing.T) {
	// Relative paths should be resolved against cwd
	v, _ := New(".")
	wd, _ := os.Getwd()

	result, err := v.Validate("somefile.txt")
	if err != nil {
		t.Fatalf("Validate(relative) unexpected error: %v", err)
	}
	expected := filepath.Join(wd, "somefile.txt")
	if result != expected {
		t.Fatalf("expected %s, got %s", expected, result)
	}
}
