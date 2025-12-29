package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestDatabaseSchemaCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "provenance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Database file was not created")
	}

	tables := []string{
		"prompt_events",
		"sessions",
		"change_sets",
		"redaction_rules",
		"schema_migrations", // golang-migrate creates this
	}

	for _, table := range tables {
		if !tableExists(t, db, table) {
			t.Errorf("Table %s does not exist", table)
		}
	}
}

func TestDatabaseWALMode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "provenance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("Failed to query journal mode: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("Expected WAL mode, got %s", journalMode)
	}
}

func TestPromptEventSchema(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "provenance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	expectedColumns := map[string]bool{
		"id":              true,
		"timestamp":       true,
		"session_id":      true,
		"agent":           true,
		"model_version":   true,
		"prompt_text":     true,
		"response_text":   true,
		"tokens_in":       true,
		"tokens_out":      true,
		"latency_ms":      true,
		"repo_path":       true,
		"git_commit":      true,
		"git_branch":      true,
		"git_dirty":       true,
		"dirty_files":     true,
		"author":          true,
		"ide":             true,
		"active_file":     true,
		"workspace_files": true,
		"prompt_type":     true,
		"tools_invoked":   true,
		"files_mentioned": true,
	}

	columns := getTableColumns(t, db, "prompt_events")
	for col := range expectedColumns {
		if !contains(columns, col) {
			t.Errorf("Column %s missing from prompt_events table", col)
		}
	}
}

func TestSessionsSchema(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "provenance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	expectedColumns := map[string]bool{
		"id":            true,
		"start_time":    true,
		"end_time":      true,
		"repo_path":     true,
		"total_prompts": true,
		"total_tokens":  true,
		"ended_by":      true,
	}

	columns := getTableColumns(t, db, "sessions")
	for col := range expectedColumns {
		if !contains(columns, col) {
			t.Errorf("Column %s missing from sessions table", col)
		}
	}
}

func tableExists(t *testing.T, db *sql.DB, tableName string) bool {
	query := `
		SELECT name
		FROM sqlite_master
		WHERE type='table' AND name=?
	`
	var name string
	err := db.QueryRow(query, tableName).Scan(&name)
	return err == nil
}

func getTableColumns(t *testing.T, db *sql.DB, tableName string) []string {
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		t.Fatalf("Failed to get table info for %s: %v", tableName, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue sql.NullString
		var pk int

		err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk)
		if err != nil {
			t.Fatalf("Failed to scan column info: %v", err)
		}
		columns = append(columns, name)
	}
	return columns
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
