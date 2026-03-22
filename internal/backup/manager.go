package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Entry represents a single backup record in the backup_log table.
type Entry struct {
	ID         string     `json:"id"`
	EntityType string     `json:"entity_type"`
	EntityName string     `json:"entity_name"`
	Action     string     `json:"action"`
	ParentID   string     `json:"parent_id,omitempty"`
	ConfigSnap string     `json:"config_snapshot,omitempty"`
	Files      string     `json:"files,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RestoredAt *time.Time `json:"restored_at,omitempty"`
}

// Filter constrains which backup entries to list.
type Filter struct {
	EntityType string
	ParentID   string
	Limit      int
}

// Manager handles backup creation, listing, and restore operations.
type Manager struct {
	db      *sql.DB
	dataDir string
	logger  *slog.Logger
}

// New creates a Manager and runs the backup_log migration.
func New(db *sql.DB, dataDir string, logger *slog.Logger) (*Manager, error) {
	m := &Manager{db: db, dataDir: dataDir, logger: logger}
	if err := m.migrate(); err != nil {
		return nil, fmt.Errorf("backup migration: %w", err)
	}
	return m, nil
}

func (m *Manager) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS backup_log (
			id           TEXT PRIMARY KEY,
			entity_type  TEXT NOT NULL,
			entity_name  TEXT NOT NULL,
			action       TEXT NOT NULL,
			parent_id    TEXT NOT NULL DEFAULT '',
			config_snap  TEXT NOT NULL DEFAULT '',
			files        TEXT NOT NULL DEFAULT '',
			created_at   DATETIME NOT NULL,
			restored_at  DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_backup_log_entity ON backup_log(entity_type, entity_name)`,
		`CREATE INDEX IF NOT EXISTS idx_backup_log_parent ON backup_log(parent_id)`,
	}
	for _, stmt := range stmts {
		if _, err := m.db.Exec(stmt); err != nil {
			return fmt.Errorf("backup migration: %w", err)
		}
	}
	return nil
}

// backupDir returns the directory for a specific backup ID.
func (m *Manager) backupDir(id string) string {
	return filepath.Join(m.dataDir, "backup", id)
}

// CreateBackup inserts a backup_log row and copies files to the backup directory.
// filesToCopy maps relative backup paths (e.g. "agents/reviewer.md") to absolute source paths.
// configSnap is the full content of tasks.yaml at time of backup.
func (m *Manager) CreateBackup(ctx context.Context, entityType, entityName, action, parentID, configSnap string, filesToCopy map[string]string) (*Entry, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	dir := m.backupDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	// Copy files.
	var fileList []string
	for relPath, srcPath := range filesToCopy {
		dstPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return nil, fmt.Errorf("create backup subdir: %w", err)
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("backup file %s: %w", relPath, err)
		}
		fileList = append(fileList, relPath)
	}

	filesJSON, _ := json.Marshal(fileList)

	entry := &Entry{
		ID:         id,
		EntityType: entityType,
		EntityName: entityName,
		Action:     action,
		ParentID:   parentID,
		ConfigSnap: configSnap,
		Files:      string(filesJSON),
		CreatedAt:  now,
	}

	_, err := m.db.ExecContext(ctx,
		`INSERT INTO backup_log (id, entity_type, entity_name, action, parent_id, config_snap, files, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.EntityType, entry.EntityName, entry.Action,
		entry.ParentID, entry.ConfigSnap, entry.Files, entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert backup_log: %w", err)
	}

	m.logger.Info("backup created", "id", id, "entity_type", entityType, "entity_name", entityName, "action", action)
	return entry, nil
}

// ListBackups returns backup entries ordered by most recent first.
func (m *Manager) ListBackups(ctx context.Context, f Filter) ([]Entry, error) {
	query := `SELECT id, entity_type, entity_name, action, parent_id, config_snap, files, created_at, restored_at
			  FROM backup_log WHERE 1=1`
	var args []any

	if f.EntityType != "" {
		query += ` AND entity_type = ?`
		args = append(args, f.EntityType)
	}
	if f.ParentID != "" {
		query += ` AND parent_id = ?`
		args = append(args, f.ParentID)
	}

	query += ` ORDER BY created_at DESC`

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	query += ` LIMIT ?`
	args = append(args, limit)

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.EntityType, &e.EntityName, &e.Action, &e.ParentID, &e.ConfigSnap, &e.Files, &e.CreatedAt, &e.RestoredAt); err != nil {
			return nil, fmt.Errorf("scan backup: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetBackup returns a single backup entry by ID.
func (m *Manager) GetBackup(ctx context.Context, id string) (*Entry, error) {
	var e Entry
	err := m.db.QueryRowContext(ctx,
		`SELECT id, entity_type, entity_name, action, parent_id, config_snap, files, created_at, restored_at
		 FROM backup_log WHERE id = ?`, id,
	).Scan(&e.ID, &e.EntityType, &e.EntityName, &e.Action, &e.ParentID, &e.ConfigSnap, &e.Files, &e.CreatedAt, &e.RestoredAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get backup: %w", err)
	}
	return &e, nil
}

// RestoreFiles reads the backed-up files from the backup directory.
// Returns a map of relative paths to file contents.
func (m *Manager) RestoreFiles(ctx context.Context, id string) (map[string][]byte, error) {
	entry, err := m.GetBackup(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("backup not found: %s", id)
	}

	var fileList []string
	if entry.Files != "" {
		if err := json.Unmarshal([]byte(entry.Files), &fileList); err != nil {
			return nil, fmt.Errorf("parse file list: %w", err)
		}
	}

	dir := m.backupDir(id)
	result := make(map[string][]byte)
	for _, relPath := range fileList {
		data, err := os.ReadFile(filepath.Join(dir, relPath))
		if err != nil {
			return nil, fmt.Errorf("read backup file %s: %w", relPath, err)
		}
		result[relPath] = data
	}
	return result, nil
}

// MarkRestored sets the restored_at timestamp on a backup entry.
func (m *Manager) MarkRestored(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := m.db.ExecContext(ctx, `UPDATE backup_log SET restored_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("mark restored: %w", err)
	}
	return nil
}

// GetChildren returns all backup entries whose parent_id matches the given ID.
func (m *Manager) GetChildren(ctx context.Context, parentID string) ([]Entry, error) {
	return m.ListBackups(ctx, Filter{ParentID: parentID, Limit: 100})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
