package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ClaudeBin string  `yaml:"claude_bin"`
	Agents    []Agent `yaml:"agents"`
}

type Agent struct {
	Name     string   `yaml:"name"`
	Prompt   string   `yaml:"prompt"`
	WorkDir  string   `yaml:"work_dir"`
	Schedule string   `yaml:"schedule,omitempty"` // cron expression
	Watch    *Watch   `yaml:"watch,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
	Model    string   `yaml:"model,omitempty"`
}

type Watch struct {
	Paths      []string `yaml:"paths"`
	Extensions []string `yaml:"extensions"`
	Debounce   string   `yaml:"debounce"` // e.g. "2s"
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.ClaudeBin == "" {
		cfg.ClaudeBin = "claude"
	}
	return &cfg, nil
}
