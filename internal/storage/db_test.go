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
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() { _ = db.Close() }() //nolint:errcheck

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Database file was not created")
	}

	tables := []string{
		"prompt_events",
		"tool_invocations",
		"commit_windows",
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
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() { _ = db.Close() }() //nolint:errcheck

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
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() { _ = db.Close() }() //nolint:errcheck

	expectedColumns := map[string]bool{
		"id":                true,
		"timestamp":         true,
		"session_id":        true, // Nullable, legacy from v1
		"agent":             true,
		"model_version":     true,
		"prompt_text":       true,
		"response_text":     true,
		"tokens_in":         true,
		"tokens_out":        true,
		"latency_ms":        true,
		"repo_path":         true,
		"git_commit":        true,
		"git_branch":        true,
		"git_dirty":         true,
		"dirty_files":       true,
		"author":            true,
		"ide":               true,
		"active_file":       true,
		"workspace_files":   true,
		"prompt_type":       true,
		"tools_invoked":     true,
		"files_mentioned":   true,
		"branch_at_capture": true, // V2: Branch at prompt submission
		"pre_branch_switch": true, // V2: Flag for branch switch detection
	}

	columns := getTableColumns(t, db, "prompt_events")
	for col := range expectedColumns {
		if !contains(columns, col) {
			t.Errorf("Column %s missing from prompt_events table", col)
		}
	}
}

// V2: Tool invocations schema test
func TestToolInvocationsSchema(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "provenance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() { _ = db.Close() }() //nolint:errcheck

	expectedColumns := map[string]bool{
		"id":        true,
		"prompt_id": true,
		"tool_name": true,
		"tool_args": true,
		"timestamp": true,
	}

	columns := getTableColumns(t, db, "tool_invocations")
	for col := range expectedColumns {
		if !contains(columns, col) {
			t.Errorf("Column %s missing from tool_invocations table", col)
		}
	}
}

// V2: Commit windows schema test
func TestCommitWindowsSchema(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "provenance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() { _ = db.Close() }() //nolint:errcheck

	expectedColumns := map[string]bool{
		"id":               true,
		"repo_path":        true,
		"branch":           true,
		"prev_commit":      true,
		"next_commit":      true,
		"prev_commit_time": true,
		"next_commit_time": true,
		"prompt_count":     true,
	}

	columns := getTableColumns(t, db, "commit_windows")
	for col := range expectedColumns {
		if !contains(columns, col) {
			t.Errorf("Column %s missing from commit_windows table", col)
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
	defer func() { _ = rows.Close() }() //nolint:errcheck

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

// TestDatabaseInitFromAnyDirectory tests that database initialization
// works regardless of current working directory (requires embedded migrations)
func TestDatabaseInitFromAnyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "provenance-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck

	workDir, err := os.MkdirTemp("", "provenance-work-*")
	if err != nil {
		t.Fatalf("Failed to create work dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }() //nolint:errcheck

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }() //nolint:errcheck

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database from different directory: %v", err)
	}
	defer func() { _ = db.Close() }() //nolint:errcheck

	if !tableExists(t, db, "prompt_events") {
		t.Error("Database tables not created - migrations likely didn't run")
	}
	if !tableExists(t, db, "tool_invocations") {
		t.Error("tool_invocations table not created - migrations likely didn't run")
	}
	if !tableExists(t, db, "commit_windows") {
		t.Error("commit_windows table not created - migrations likely didn't run")
	}
}
