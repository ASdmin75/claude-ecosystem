package domain

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/config"
	_ "modernc.org/sqlite"
)

// Manager handles domain lifecycle: directory creation, schema initialization,
// environment variable generation, and domain doc content reading.
type Manager struct {
	domains map[string]config.Domain
	logger  *slog.Logger
}

// New creates a new domain Manager.
func New(domains map[string]config.Domain, logger *slog.Logger) *Manager {
	if domains == nil {
		domains = make(map[string]config.Domain)
	}
	return &Manager{
		domains: domains,
		logger:  logger,
	}
}

// Init creates data directories, applies database schemas, and generates
// template DOMAIN.md files for all configured domains.
func (m *Manager) Init() error {
	for name, d := range m.domains {
		if err := m.initSingle(name, d); err != nil {
			return err
		}
	}
	return nil
}

// AddDomain registers a new domain, creates its data directory, applies the
// schema, and generates the DOMAIN.md template — the same init logic as Init
// but for a single domain added at runtime.
func (m *Manager) AddDomain(name string, d config.Domain) error {
	if _, exists := m.domains[name]; exists {
		return fmt.Errorf("domain %q already exists", name)
	}
	d.Name = name
	if err := m.initSingle(name, d); err != nil {
		return err
	}
	m.domains[name] = d
	return nil
}

// initSingle initializes a single domain: mkdir, apply schema, generate doc.
func (m *Manager) initSingle(name string, d config.Domain) error {
	if err := os.MkdirAll(d.DataDir, 0o755); err != nil {
		return fmt.Errorf("domain %s: create data dir %s: %w", name, d.DataDir, err)
	}
	m.logger.Info("domain data dir ready", "domain", name, "path", d.DataDir)

	if d.DB != "" && d.Schema != "" {
		dbPath := d.DBPath()
		if err := m.applySchema(dbPath, d.Schema); err != nil {
			return fmt.Errorf("domain %s: apply schema: %w", name, err)
		}
		m.logger.Info("domain database ready", "domain", name, "db", dbPath)
	}

	if d.DomainDoc != "" {
		docPath := d.DomainDocPath()
		if _, err := os.Stat(docPath); os.IsNotExist(err) {
			content := m.generateDomainDocTemplate(d)
			if err := os.WriteFile(docPath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("domain %s: create domain doc: %w", name, err)
			}
			m.logger.Info("domain doc template created", "domain", name, "path", docPath)
		}
	}
	return nil
}

// Reload updates the in-memory domain map from a new config.
// New domains are initialized (mkdir, schema, doc); existing ones are kept.
func (m *Manager) Reload(domains map[string]config.Domain) {
	if domains == nil {
		return
	}
	for name, d := range domains {
		if _, ok := m.domains[name]; ok {
			continue // already initialized
		}
		d.Name = name
		if err := m.initSingle(name, d); err != nil {
			m.logger.Error("domain reload: init failed", "domain", name, "error", err)
			continue
		}
		m.domains[name] = d
		m.logger.Info("domain registered on reload", "domain", name)
	}
}

// RemoveDomain removes a domain from the in-memory map.
// The domain's data directory is left on disk as a safety measure.
func (m *Manager) RemoveDomain(name string) {
	delete(m.domains, name)
	m.logger.Info("domain removed from manager", "domain", name)
}

// GetDomain returns a domain by name.
func (m *Manager) GetDomain(name string) (config.Domain, bool) {
	d, ok := m.domains[name]
	return d, ok
}

// DomainEnvVars returns environment variables for a domain that should be
// injected into MCP server configurations.
func (m *Manager) DomainEnvVars(name string) map[string]string {
	d, ok := m.domains[name]
	if !ok {
		return nil
	}
	vars := map[string]string{
		"DOMAIN_NAME":     name,
		"DOMAIN_DATA_DIR": d.DataDir,
	}
	if d.DB != "" {
		absPath, err := filepath.Abs(d.DBPath())
		if err != nil {
			absPath = d.DBPath()
		}
		vars["DOMAIN_DB_PATH"] = absPath
	}
	if d.DomainDoc != "" {
		vars["DOMAIN_DOC_PATH"] = d.DomainDocPath()
	}
	return vars
}

// DomainDocContent reads and returns the content of the domain's DOMAIN.md file.
func (m *Manager) DomainDocContent(name string) (string, error) {
	d, ok := m.domains[name]
	if !ok {
		return "", fmt.Errorf("unknown domain: %s", name)
	}
	if d.DomainDoc == "" {
		return "", nil
	}
	docPath := d.DomainDocPath()
	data, err := os.ReadFile(docPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading domain doc %s: %w", docPath, err)
	}
	return string(data), nil
}

// applySchema opens a SQLite database and executes the schema SQL.
func (m *Manager) applySchema(dbPath string, schema string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("opening database %s: %w", dbPath, err)
	}
	defer db.Close()

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("executing schema on %s: %w", dbPath, err)
	}
	return nil
}

// generateDomainDocTemplate creates a basic DOMAIN.md template from the domain config.
func (m *Manager) generateDomainDocTemplate(d config.Domain) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Domain: %s\n\n", d.Name))
	if d.Description != "" {
		sb.WriteString(d.Description + "\n\n")
	}

	sb.WriteString("## Files\n\n")
	sb.WriteString("| File | Description |\n")
	sb.WriteString("|------|-------------|\n")
	if d.DB != "" {
		sb.WriteString(fmt.Sprintf("| %s | SQLite database |\n", d.DB))
	}
	if d.DomainDoc != "" {
		sb.WriteString(fmt.Sprintf("| %s | This file — domain documentation |\n", d.DomainDoc))
	}
	sb.WriteString("\n")

	// Parse CREATE TABLE from schema to generate column docs
	if d.Schema != "" {
		tables := parseCreateTables(d.Schema)
		if len(tables) > 0 {
			sb.WriteString("## Tables\n\n")
			for _, t := range tables {
				sb.WriteString(fmt.Sprintf("### %s\n\n", t.name))
				sb.WriteString("| Column | Type | Description |\n")
				sb.WriteString("|--------|------|-------------|\n")
				for _, col := range t.columns {
					sb.WriteString(fmt.Sprintf("| %s | %s | |\n", col.name, col.typ))
				}
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

type tableInfo struct {
	name    string
	columns []columnInfo
}

type columnInfo struct {
	name string
	typ  string
}

// parseCreateTables does a simple parse of CREATE TABLE statements from SQL schema.
func parseCreateTables(schema string) []tableInfo {
	var tables []tableInfo
	lines := strings.Split(schema, "\n")

	var current *tableInfo
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		if strings.HasPrefix(upper, "CREATE TABLE") {
			// Extract table name
			name := extractTableName(trimmed)
			if name != "" {
				tables = append(tables, tableInfo{name: name})
				current = &tables[len(tables)-1]
			}
			continue
		}

		if current != nil && strings.HasPrefix(trimmed, ")") {
			current = nil
			continue
		}

		if current != nil && trimmed != "" && trimmed != "(" {
			// Skip constraints and indexes
			if strings.HasPrefix(upper, "CREATE") ||
				strings.HasPrefix(upper, "PRIMARY") ||
				strings.HasPrefix(upper, "UNIQUE") ||
				strings.HasPrefix(upper, "FOREIGN") ||
				strings.HasPrefix(upper, "CHECK") ||
				strings.HasPrefix(upper, "CONSTRAINT") {
				continue
			}

			col := parseColumnDef(trimmed)
			if col.name != "" {
				current.columns = append(current.columns, col)
			}
		}
	}
	return tables
}

func extractTableName(line string) string {
	// Handle: CREATE TABLE IF NOT EXISTS tablename (
	upper := strings.ToUpper(line)
	idx := strings.Index(upper, "EXISTS")
	if idx >= 0 {
		rest := strings.TrimSpace(line[idx+6:])
		return cleanTableName(rest)
	}
	// Handle: CREATE TABLE tablename (
	idx = strings.Index(upper, "TABLE")
	if idx >= 0 {
		rest := strings.TrimSpace(line[idx+5:])
		return cleanTableName(rest)
	}
	return ""
}

func cleanTableName(s string) string {
	s = strings.TrimSpace(s)
	// Remove trailing ( and whitespace
	s = strings.TrimRight(s, " (")
	// Remove quotes
	s = strings.Trim(s, "`\"'")
	return s
}

func parseColumnDef(line string) columnInfo {
	// Remove trailing comma
	line = strings.TrimRight(line, ",")
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return columnInfo{}
	}
	name := strings.Trim(parts[0], "`\"'")
	typ := parts[1]
	return columnInfo{name: name, typ: typ}
}
