package config

import "path/filepath"

// Domain defines a business data domain linked to tasks, pipelines, and agents.
type Domain struct {
	Name        string   `yaml:"-" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	DataDir     string   `yaml:"data_dir" json:"data_dir"`
	DB          string   `yaml:"db,omitempty" json:"db,omitempty"`
	Schema      string   `yaml:"schema,omitempty" json:"schema,omitempty"`
	DomainDoc   string   `yaml:"domain_doc,omitempty" json:"domain_doc,omitempty"`
	Tasks       []string `yaml:"tasks,omitempty" json:"tasks,omitempty"`
	Pipelines   []string `yaml:"pipelines,omitempty" json:"pipelines,omitempty"`
	Agents      []string `yaml:"agents,omitempty" json:"agents,omitempty"`
	MCPServers  []string `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty"`
}

// DBPath returns the full path to the domain's SQLite database file.
func (d Domain) DBPath() string {
	if d.DB == "" {
		return ""
	}
	return filepath.Join(d.DataDir, d.DB)
}

// DomainDocPath returns the full path to the domain's documentation file.
func (d Domain) DomainDocPath() string {
	if d.DomainDoc == "" {
		return ""
	}
	return filepath.Join(d.DataDir, d.DomainDoc)
}
