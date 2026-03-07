package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration loaded from tasks.yaml (or agents.yaml).
type Config struct {
	ClaudeBin  string            `yaml:"claude_bin"`
	Tasks      []Task            `yaml:"tasks"`
	Pipelines  []Pipeline        `yaml:"pipelines,omitempty"`
	MCPServers []MCPServerConfig `yaml:"mcp_servers,omitempty"`
	Auth       AuthConfig        `yaml:"auth,omitempty"`
	Server     ServerConfig      `yaml:"server,omitempty"`
}

// AuthConfig holds authentication settings for the server.
type AuthConfig struct {
	PASETOKey    string       `yaml:"paseto_key"`
	BearerTokens []string    `yaml:"bearer_tokens,omitempty"`
	Users        []UserConfig `yaml:"users,omitempty"`
}

// UserConfig represents a single user credential.
type UserConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"` // bcrypt hash
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr    string `yaml:"addr"`     // default ":8080"
	DataDir string `yaml:"data_dir"` // default "data"
}

// MCPServerConfig defines an MCP server that tasks can reference.
type MCPServerConfig struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

// legacyConfig is used to detect the old agents.yaml format and migrate it.
type legacyConfig struct {
	ClaudeBin string     `yaml:"claude_bin"`
	Agents    []Task     `yaml:"agents"`
	Pipelines []Pipeline `yaml:"pipelines,omitempty"`
}

// Load reads configuration from the given path. If path is empty it tries
// "tasks.yaml" first, then falls back to "agents.yaml" for backward
// compatibility. It applies defaults and validates the result.
func Load(path string) (*Config, error) {
	if path == "" {
		// Try tasks.yaml first, fall back to agents.yaml.
		if _, err := os.Stat("tasks.yaml"); err == nil {
			path = "tasks.yaml"
		} else {
			path = "agents.yaml"
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// Attempt legacy format where the key is "agents" instead of "tasks".
		var legacy legacyConfig
		if legacyErr := yaml.Unmarshal(data, &legacy); legacyErr == nil && len(legacy.Agents) > 0 {
			cfg.ClaudeBin = legacy.ClaudeBin
			cfg.Tasks = legacy.Agents
			cfg.Pipelines = legacy.Pipelines
		} else {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	// If Tasks is still empty, the file might use the old "agents" key and
	// parsed without error (tasks field was simply ignored). Try legacy.
	if len(cfg.Tasks) == 0 {
		var legacy legacyConfig
		if err := yaml.Unmarshal(data, &legacy); err == nil && len(legacy.Agents) > 0 {
			cfg.Tasks = legacy.Agents
			if cfg.ClaudeBin == "" {
				cfg.ClaudeBin = legacy.ClaudeBin
			}
			if len(cfg.Pipelines) == 0 {
				cfg.Pipelines = legacy.Pipelines
			}
		}
	}

	applyDefaults(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyDefaults fills in zero-value fields with sensible defaults.
func applyDefaults(cfg *Config) {
	if cfg.ClaudeBin == "" {
		cfg.ClaudeBin = "claude"
	}
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Server.DataDir == "" {
		cfg.Server.DataDir = "data"
	}
}
