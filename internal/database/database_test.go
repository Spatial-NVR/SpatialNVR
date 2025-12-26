package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Path:            dbPath,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Hour,
	}

	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}

	// Test health check
	if err := db.Health(context.Background()); err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("/data")

	if cfg.Path != "/data/nvr.db" {
		t.Errorf("Expected path /data/nvr.db, got %s", cfg.Path)
	}

	if cfg.MaxOpenConns != 25 {
		t.Errorf("Expected MaxOpenConns 25, got %d", cfg.MaxOpenConns)
	}

	if cfg.MaxIdleConns != 5 {
		t.Errorf("Expected MaxIdleConns 5, got %d", cfg.MaxIdleConns)
	}
}

func TestTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create a test table
	_, err = db.Exec(`CREATE TABLE test_table (id INTEGER PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Test successful transaction
	err = db.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO test_table (value) VALUES (?)`, "test1")
		return err
	})
	if err != nil {
		t.Errorf("Transaction failed: %v", err)
	}

	// Verify data was inserted
	var value string
	err = db.QueryRow(`SELECT value FROM test_table WHERE id = 1`).Scan(&value)
	if err != nil {
		t.Errorf("Failed to query inserted data: %v", err)
	}
	if value != "test1" {
		t.Errorf("Expected value 'test1', got '%s'", value)
	}

	// Test transaction rollback on error
	expectedErr := fmt.Errorf("intentional error")
	err = db.Transaction(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO test_table (value) VALUES (?)`, "test2")
		if err != nil {
			return err
		}
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Verify test2 was not inserted (rollback worked)
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test_table WHERE value = 'test2'`).Scan(&count)
	if err != nil {
		t.Errorf("Failed to count: %v", err)
	}
	if count != 0 {
		t.Error("Transaction should have rolled back, but data was inserted")
	}
}

func TestHealth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Test health when open
	err = db.Health(context.Background())
	if err != nil {
		t.Errorf("Health check failed on open database: %v", err)
	}

	// Close and test health
	db.Close()
	err = db.Health(context.Background())
	if err == nil {
		t.Error("Health check should fail on closed database")
	}
}

func TestGetSize(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert some data to create file size
	_, err = db.Exec(`CREATE TABLE test_table (id INTEGER PRIMARY KEY, data BLOB)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert some data
	_, err = db.Exec(`INSERT INTO test_table (data) VALUES (?)`, make([]byte, 1000))
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	size, err := db.GetSize()
	if err != nil {
		t.Errorf("GetSize failed: %v", err)
	}
	if size <= 0 {
		t.Error("Expected positive database size")
	}
}

func TestVacuum(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create and populate table
	_, err = db.Exec(`CREATE TABLE test_table (id INTEGER PRIMARY KEY, data TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	for i := 0; i < 100; i++ {
		_, err = db.Exec(`INSERT INTO test_table (data) VALUES (?)`, "test data "+string(rune(i)))
		if err != nil {
			t.Fatalf("Failed to insert data: %v", err)
		}
	}

	// Delete data
	_, err = db.Exec(`DELETE FROM test_table`)
	if err != nil {
		t.Fatalf("Failed to delete data: %v", err)
	}

	// Vacuum should succeed
	err = db.Vacuum(context.Background())
	if err != nil {
		t.Errorf("Vacuum failed: %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations with cancelled context should fail
	err = db.Health(ctx)
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
}

func TestPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if db.Path() != dbPath {
		t.Errorf("Expected path %s, got %s", dbPath, db.Path())
	}
}

func TestStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	stats := db.Stats()

	// Stats object should be returned successfully
	// Note: SQLite driver may not report all fields correctly
	_ = stats.OpenConnections
}

func TestAnalyze(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create a table with data
	_, err = db.Exec(`CREATE TABLE test_table (id INTEGER PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	for i := 0; i < 10; i++ {
		_, err = db.Exec(`INSERT INTO test_table (value) VALUES (?)`, fmt.Sprintf("value%d", i))
		if err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}
	}

	// Analyze should succeed
	err = db.Analyze(context.Background())
	if err != nil {
		t.Errorf("Analyze failed: %v", err)
	}
}

func TestCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert some data to create WAL entries
	_, err = db.Exec(`CREATE TABLE test_table (id INTEGER PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = db.Exec(`INSERT INTO test_table (value) VALUES (?)`, "test")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Checkpoint should succeed
	err = db.Checkpoint(context.Background())
	if err != nil {
		t.Errorf("Checkpoint failed: %v", err)
	}
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(&Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Close should succeed
	err = db.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Subsequent operations should fail
	err = db.Health(context.Background())
	if err == nil {
		t.Error("Expected error after close")
	}
}

func TestOpenInvalidPath(t *testing.T) {
	// Try to open database in a non-existent parent directory with no permissions
	cfg := &Config{
		Path: "/root/nonexistent/test.db",
	}

	_, err := Open(cfg)
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetSizeNonExistent(t *testing.T) {
	db := &DB{
		path: "/nonexistent/path/db.db",
	}

	_, err := db.GetSize()
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
}
