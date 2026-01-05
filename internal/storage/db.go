package storage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/mattn/go-sqlite3"
)

// InitDatabase creates or opens a SQLite database at the specified path,
// runs migrations, and enables WAL mode for concurrent access.
func InitDatabase(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Ping to ensure database file is created
	if err := db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close database after ping error: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set file permissions (owner read/write only)
	if err := os.Chmod(dbPath, 0600); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close database after chmod error: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to set database permissions: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close database after WAL error: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close database after foreign keys error: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := runMigrations(db); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close database after migration error: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// runMigrations applies database migrations using golang-migrate with embedded migrations
func runMigrations(db *sql.DB) error {
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		"sqlite3",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}
