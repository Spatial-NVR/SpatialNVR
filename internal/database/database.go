// Package database provides SQLite database access for the NVR system
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQL database connection with NVR-specific functionality
type DB struct {
	*sql.DB
	path   string
	logger *slog.Logger
}

// Config holds database configuration
type Config struct {
	Path            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultConfig returns the default database configuration
func DefaultConfig(dataDir string) *Config {
	return &Config{
		Path:            filepath.Join(dataDir, "nvr.db"),
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// Open opens a new database connection
func Open(cfg *Config) (*DB, error) {
	logger := slog.Default().With("component", "database")

	// Ensure directory exists
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Build connection string with SQLite pragmas
	connStr := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=ON", cfg.Path)

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Test connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set additional pragmas
	pragmas := []string{
		"PRAGMA cache_size = -64000",    // 64MB cache
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 268435456",  // 256MB mmap
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			logger.Warn("Failed to set pragma", "pragma", pragma, "error", err)
		}
	}

	logger.Info("Database opened", "path", cfg.Path)

	return &DB{
		DB:     db,
		path:   cfg.Path,
		logger: logger,
	}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.logger.Info("Closing database")
	return db.DB.Close()
}

// Path returns the database file path
func (db *DB) Path() string {
	return db.path
}

// Health checks the database health
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return db.PingContext(ctx)
}

// Stats returns database statistics
func (db *DB) Stats() sql.DBStats {
	return db.DB.Stats()
}

// Vacuum performs database maintenance
func (db *DB) Vacuum(ctx context.Context) error {
	db.logger.Info("Starting database vacuum")
	start := time.Now()

	_, err := db.ExecContext(ctx, "VACUUM")
	if err != nil {
		return fmt.Errorf("vacuum failed: %w", err)
	}

	db.logger.Info("Database vacuum completed", "duration", time.Since(start))
	return nil
}

// Analyze updates database statistics for query optimization
func (db *DB) Analyze(ctx context.Context) error {
	db.logger.Info("Starting database analyze")

	_, err := db.ExecContext(ctx, "ANALYZE")
	if err != nil {
		return fmt.Errorf("analyze failed: %w", err)
	}

	db.logger.Info("Database analyze completed")
	return nil
}

// Transaction wraps a function in a database transaction
func (db *DB) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	return nil
}

// GetSize returns the database file size in bytes
func (db *DB) GetSize() (int64, error) {
	info, err := os.Stat(db.path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Checkpoint forces a WAL checkpoint
func (db *DB) Checkpoint(ctx context.Context) error {
	_, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
