package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
)

// InitDatabase creates or opens a SQLite database at the specified path,
// runs migrations, and enables WAL mode for concurrent access.
func InitDatabase(dbPath string) (*sql.DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Ping to ensure database file is created
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set file permissions (owner read/write only)
	if err := os.Chmod(dbPath, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set database permissions: %w", err)
	}

	// Enable WAL mode for concurrent read/write
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign key constraints
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// runMigrations applies database migrations using golang-migrate
func runMigrations(db *sql.DB) error {
	// Create migrate driver
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	// Find migrations directory
	migrationsPath, err := findMigrationsPath()
	if err != nil {
		return fmt.Errorf("failed to find migrations directory: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"sqlite3",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// findMigrationsPath locates the migrations directory
// Searches relative to the current working directory and common locations
func findMigrationsPath() (string, error) {
	// Try common locations
	paths := []string{
		"migrations",
		"./migrations",
		"../../../migrations", // For tests running from internal/storage
		"../../migrations",    // Alternative test path
	}

	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			return absPath, nil
		}
	}

	return "", fmt.Errorf("migrations directory not found")
}
