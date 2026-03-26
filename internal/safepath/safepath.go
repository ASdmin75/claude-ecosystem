// Package safepath provides path validation to prevent directory traversal attacks.
// It ensures that file paths resolve within explicitly allowed directories.
package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Validator checks that paths fall within allowed directories.
type Validator struct {
	allowedDirs []string
}

// New creates a Validator from a list of directory paths.
// Each directory is resolved to an absolute, cleaned path.
// If no dirs are provided, the current working directory is used as default.
func New(dirs ...string) (*Validator, error) {
	v := &Validator{}

	if len(dirs) == 0 {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("safepath: cannot determine working directory: %w", err)
		}
		v.allowedDirs = []string{filepath.Clean(wd)}
		return v, nil
	}

	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			return nil, fmt.Errorf("safepath: invalid directory %q: %w", d, err)
		}
		v.allowedDirs = append(v.allowedDirs, filepath.Clean(abs))
	}

	if len(v.allowedDirs) == 0 {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("safepath: cannot determine working directory: %w", err)
		}
		v.allowedDirs = []string{filepath.Clean(wd)}
	}

	return v, nil
}

// NewFromEnv creates a Validator by reading a colon-separated environment variable.
// Falls back to the current working directory if the env var is empty.
func NewFromEnv(envVar string) (*Validator, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return New()
	}
	return New(strings.Split(val, ":")...)
}

// Validate returns the cleaned absolute path if it falls within an allowed
// directory, or an error otherwise.
func (v *Validator) Validate(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	abs = filepath.Clean(abs)

	for _, dir := range v.allowedDirs {
		if strings.HasPrefix(abs, dir+string(filepath.Separator)) || abs == dir {
			return abs, nil
		}
	}
	return "", fmt.Errorf("access denied: path %s is outside allowed directories", abs)
}

// AllowedDirs returns a copy of the allowed directories.
func (v *Validator) AllowedDirs() []string {
	cp := make([]string, len(v.allowedDirs))
	copy(cp, v.allowedDirs)
	return cp
}
