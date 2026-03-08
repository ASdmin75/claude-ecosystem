package config

import (
	"os"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	// Create a temp .env file.
	content := `# comment
TESTDOTENV_HOST=smtp.example.com
TESTDOTENV_PORT=587
TESTDOTENV_QUOTED="hello world"
TESTDOTENV_SINGLE='single'
`
	f, err := os.CreateTemp("", "dotenv-test-*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	// Clean up env after test.
	defer os.Unsetenv("TESTDOTENV_HOST")
	defer os.Unsetenv("TESTDOTENV_PORT")
	defer os.Unsetenv("TESTDOTENV_QUOTED")
	defer os.Unsetenv("TESTDOTENV_SINGLE")

	if err := LoadDotEnv(f.Name()); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	tests := []struct {
		key, want string
	}{
		{"TESTDOTENV_HOST", "smtp.example.com"},
		{"TESTDOTENV_PORT", "587"},
		{"TESTDOTENV_QUOTED", "hello world"},
		{"TESTDOTENV_SINGLE", "single"},
	}
	for _, tt := range tests {
		if got := os.Getenv(tt.key); got != tt.want {
			t.Errorf("env %s = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestLoadDotEnvDoesNotOverrideExisting(t *testing.T) {
	os.Setenv("TESTDOTENV_EXISTING", "original")
	defer os.Unsetenv("TESTDOTENV_EXISTING")

	f, err := os.CreateTemp("", "dotenv-test-*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("TESTDOTENV_EXISTING=overridden\n")
	f.Close()

	if err := LoadDotEnv(f.Name()); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	if got := os.Getenv("TESTDOTENV_EXISTING"); got != "original" {
		t.Errorf("env TESTDOTENV_EXISTING = %q, want %q (should not be overridden)", got, "original")
	}
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	// Missing file should not error.
	if err := LoadDotEnv("/nonexistent/.env"); err != nil {
		t.Errorf("LoadDotEnv for missing file should return nil, got: %v", err)
	}
}

func TestExpandEnvVars(t *testing.T) {
	os.Setenv("TESTEXPAND_A", "hello")
	os.Setenv("TESTEXPAND_B", "world")
	defer os.Unsetenv("TESTEXPAND_A")
	defer os.Unsetenv("TESTEXPAND_B")

	tests := []struct {
		input, want string
	}{
		{"${TESTEXPAND_A}", "hello"},
		{"${TESTEXPAND_A} ${TESTEXPAND_B}", "hello world"},
		{"no vars here", "no vars here"},
		{"${TESTEXPAND_UNDEFINED}", ""},
		{"prefix-${TESTEXPAND_A}-suffix", "prefix-hello-suffix"},
	}
	for _, tt := range tests {
		if got := ExpandEnvVars(tt.input); got != tt.want {
			t.Errorf("ExpandEnvVars(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
