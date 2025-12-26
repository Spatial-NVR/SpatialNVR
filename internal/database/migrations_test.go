package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNewMigrator(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)
	if migrator == nil {
		t.Fatal("NewMigrator returned nil")
	}
	if migrator.db != db {
		t.Error("Migrator db not set correctly")
	}
	if migrator.logger == nil {
		t.Error("Migrator logger should be set")
	}
}

func TestMigrator_Run(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)

	// Run migrations
	err = migrator.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify migrations table exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query schema_migrations: %v", err)
	}

	// Running again should be idempotent
	err = migrator.Run(context.Background())
	if err != nil {
		t.Fatalf("Second Run failed: %v", err)
	}
}

func TestMigrator_GetStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)

	// Run migrations first
	err = migrator.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Get status
	status, err := migrator.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Should have at least one migration
	if len(status) == 0 {
		t.Error("Expected at least one migration in status")
	}

	// Check that migrations have AppliedAt set
	for _, m := range status {
		if m.AppliedAt.IsZero() {
			t.Errorf("Migration %d should have AppliedAt set", m.Version)
		}
		if m.Name == "" {
			t.Errorf("Migration %d should have Name set", m.Version)
		}
	}
}

func TestMigrator_ensureMigrationsTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)

	// Should create table
	err = migrator.ensureMigrationsTable(context.Background())
	if err != nil {
		t.Fatalf("ensureMigrationsTable failed: %v", err)
	}

	// Verify table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&name)
	if err != nil {
		t.Fatalf("schema_migrations table should exist: %v", err)
	}

	// Should be idempotent
	err = migrator.ensureMigrationsTable(context.Background())
	if err != nil {
		t.Fatalf("Second ensureMigrationsTable failed: %v", err)
	}
}

func TestMigrator_getAppliedMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)

	// Ensure table exists
	err = migrator.ensureMigrationsTable(context.Background())
	if err != nil {
		t.Fatalf("ensureMigrationsTable failed: %v", err)
	}

	// Initially empty
	applied, err := migrator.getAppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("getAppliedMigrations failed: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("Expected 0 applied migrations, got %d", len(applied))
	}

	// Insert a migration
	_, err = db.Exec("INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, 'test', ?)", time.Now().Unix())
	if err != nil {
		t.Fatalf("Failed to insert test migration: %v", err)
	}

	// Should now have one
	applied, err = migrator.getAppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("getAppliedMigrations failed: %v", err)
	}
	if len(applied) != 1 {
		t.Errorf("Expected 1 applied migration, got %d", len(applied))
	}
	if _, ok := applied[1]; !ok {
		t.Error("Expected migration version 1 to be in applied map")
	}
}

func TestMigrator_getAvailableMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)

	// Get available migrations
	migrations, err := migrator.getAvailableMigrations()
	if err != nil {
		t.Fatalf("getAvailableMigrations failed: %v", err)
	}

	// Should have at least one migration from embedded files
	if len(migrations) == 0 {
		t.Error("Expected at least one available migration")
	}

	// Verify migrations are sorted by version
	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version <= migrations[i-1].Version {
			t.Error("Migrations should be sorted by version ascending")
		}
	}

	// Verify migrations have required fields
	for _, m := range migrations {
		if m.Version == 0 {
			t.Error("Migration version should not be 0")
		}
		if m.Name == "" {
			t.Error("Migration name should not be empty")
		}
		if m.SQL == "" {
			t.Error("Migration SQL should not be empty")
		}
	}
}

func TestMigration_Struct(t *testing.T) {
	now := time.Now()
	m := Migration{
		Version:   1,
		Name:      "initial_schema",
		SQL:       "CREATE TABLE test (id INTEGER PRIMARY KEY);",
		AppliedAt: now,
	}

	if m.Version != 1 {
		t.Errorf("Expected Version 1, got %d", m.Version)
	}
	if m.Name != "initial_schema" {
		t.Errorf("Expected Name 'initial_schema', got %s", m.Name)
	}
	if m.SQL == "" {
		t.Error("SQL should not be empty")
	}
	if m.AppliedAt.IsZero() {
		t.Error("AppliedAt should be set")
	}
}

func TestMigrator_RunMigrationOrder(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)

	// Run all migrations
	err = migrator.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Get applied migrations
	applied, err := migrator.getAppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("getAppliedMigrations failed: %v", err)
	}

	// Get available migrations
	available, err := migrator.getAvailableMigrations()
	if err != nil {
		t.Fatalf("getAvailableMigrations failed: %v", err)
	}

	// All available should be applied
	for _, m := range available {
		if _, ok := applied[m.Version]; !ok {
			t.Errorf("Migration %d should be applied", m.Version)
		}
	}
}

func TestMigrator_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db)

	// Cancel context before running
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run with cancelled context
	err = migrator.Run(ctx)
	// May or may not error depending on timing, but should not panic
}
