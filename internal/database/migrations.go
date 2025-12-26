package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migration represents a database migration
type Migration struct {
	Version   int
	Name      string
	SQL       string
	AppliedAt time.Time
}

// Migrator handles database migrations
type Migrator struct {
	db     *DB
	logger *slog.Logger
}

// NewMigrator creates a new migrator
func NewMigrator(db *DB) *Migrator {
	return &Migrator{
		db:     db,
		logger: slog.Default().With("component", "migrator"),
	}
}

// Run runs all pending migrations
func (m *Migrator) Run(ctx context.Context) error {
	m.logger.Info("Running database migrations")

	// Ensure migrations table exists
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	// Get applied migrations
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Get available migrations
	available, err := m.getAvailableMigrations()
	if err != nil {
		return err
	}

	// Run pending migrations
	for _, migration := range available {
		if _, ok := applied[migration.Version]; ok {
			continue
		}

		if err := m.runMigration(ctx, migration); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", migration.Version, migration.Name, err)
		}

		m.logger.Info("Applied migration", "version", migration.Version, "name", migration.Name)
	}

	m.logger.Info("Database migrations completed")
	return nil
}

// GetStatus returns the migration status
func (m *Migrator) GetStatus(ctx context.Context) ([]Migration, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return nil, err
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	available, err := m.getAvailableMigrations()
	if err != nil {
		return nil, err
	}

	var result []Migration
	for _, migration := range available {
		if appliedAt, ok := applied[migration.Version]; ok {
			migration.AppliedAt = appliedAt
		}
		result = append(result, migration)
	}

	return result, nil
}

// ensureMigrationsTable creates the migrations tracking table if it doesn't exist
func (m *Migrator) ensureMigrationsTable(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at INTEGER NOT NULL DEFAULT (unixepoch())
		) STRICT
	`)
	return err
}

// getAppliedMigrations returns a map of applied migration versions to their applied time
func (m *Migrator) getAppliedMigrations(ctx context.Context) (map[int]time.Time, error) {
	rows, err := m.db.QueryContext(ctx, "SELECT version, applied_at FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]time.Time)
	for rows.Next() {
		var version int
		var appliedAt int64
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, err
		}
		result[version] = time.Unix(appliedAt, 0)
	}

	return result, rows.Err()
}

// getAvailableMigrations reads all available migration files
func (m *Migrator) getAvailableMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []Migration

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Parse version from filename (e.g., "001_initial_schema.sql")
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			m.logger.Warn("Invalid migration filename", "file", entry.Name())
			continue
		}

		name := strings.TrimSuffix(parts[1], ".sql")

		content, err := fs.ReadFile(migrationsFS, filepath.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// runMigration runs a single migration within a transaction
func (m *Migrator) runMigration(ctx context.Context, migration Migration) error {
	return m.db.Transaction(ctx, func(tx *sql.Tx) error {
		// Execute migration SQL
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return err
		}

		// Record migration
		_, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO schema_migrations (version, name) VALUES (?, ?)",
			migration.Version, migration.Name,
		)
		return err
	})
}
