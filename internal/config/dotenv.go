package config

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// envVarPattern matches ${VAR_NAME} references in strings.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// LoadDotEnv reads a .env file and sets any variables that are not already
// present in the process environment. This gives real env vars higher priority.
// Missing file is silently ignored.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env is optional
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first '='.
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes (single or double).
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Only set if not already in environment (env vars take priority).
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

// ExpandEnvVars replaces all ${VAR} references in s with their values
// from the process environment. Unknown variables are replaced with "".
func ExpandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${NAME}.
		name := match[2 : len(match)-1]
		return os.Getenv(name)
	})
}

// expandConfigEnvVars walks the config and expands ${VAR} references
// in fields that commonly contain secrets or environment-specific values.
func expandConfigEnvVars(cfg *Config) {
	// Auth
	cfg.Auth.PASETOKey = ExpandEnvVars(cfg.Auth.PASETOKey)
	for i := range cfg.Auth.BearerTokens {
		cfg.Auth.BearerTokens[i] = ExpandEnvVars(cfg.Auth.BearerTokens[i])
	}
	for i := range cfg.Auth.Users {
		cfg.Auth.Users[i].Password = ExpandEnvVars(cfg.Auth.Users[i].Password)
	}

	// Server
	cfg.Server.Addr = ExpandEnvVars(cfg.Server.Addr)
	cfg.Server.DataDir = ExpandEnvVars(cfg.Server.DataDir)

	// MCP servers env
	for i := range cfg.MCPServers {
		cfg.MCPServers[i].Command = ExpandEnvVars(cfg.MCPServers[i].Command)
		for j := range cfg.MCPServers[i].Args {
			cfg.MCPServers[i].Args[j] = ExpandEnvVars(cfg.MCPServers[i].Args[j])
		}
		for k, v := range cfg.MCPServers[i].Env {
			cfg.MCPServers[i].Env[k] = ExpandEnvVars(v)
		}
	}

	// Domains
	for k, d := range cfg.Domains {
		d.DataDir = ExpandEnvVars(d.DataDir)
		d.DB = ExpandEnvVars(d.DB)
		cfg.Domains[k] = d
	}

	// Task prompts and related fields
	for i := range cfg.Tasks {
		cfg.Tasks[i].Prompt = ExpandEnvVars(cfg.Tasks[i].Prompt)
		cfg.Tasks[i].WorkDir = ExpandEnvVars(cfg.Tasks[i].WorkDir)
		cfg.Tasks[i].AppendSystemPrompt = ExpandEnvVars(cfg.Tasks[i].AppendSystemPrompt)
		if cfg.Tasks[i].Notify != nil {
			for j := range cfg.Tasks[i].Notify.Email {
				cfg.Tasks[i].Notify.Email[j] = ExpandEnvVars(cfg.Tasks[i].Notify.Email[j])
			}
			cfg.Tasks[i].Notify.Webhook = ExpandEnvVars(cfg.Tasks[i].Notify.Webhook)
		}
	}
}
