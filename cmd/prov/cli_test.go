package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/storage"
)

var testBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "prov-test-binary-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	testBinary = filepath.Join(tmpDir, "prov-test")
	cmd := exec.Command("go", "build", "-o", testBinary, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build CLI: %v\nOutput: %s\n", err, output)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}

func TestCLIVersion(t *testing.T) {
	output, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	if !strings.Contains(output, "prov version") {
		t.Errorf("Expected version output, got: %s", output)
	}
}

func TestCLIDaemonStart(t *testing.T) {
	tmpDir := setupTestEnv(t)

	output, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("daemon start failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Daemon started") {
		t.Errorf("Expected 'Daemon started' message, got: %s", output)
	}

	socketPath := filepath.Join(tmpDir, "daemon.sock")
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("Daemon socket file not created")
	}

	runCLI(t, "daemon", "stop") //nolint:errcheck
}

func TestCLIDaemonStop(t *testing.T) {
	setupTestEnv(t)

	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	output, err := runCLI(t, "daemon", "stop")
	if err != nil {
		t.Fatalf("daemon stop failed: %v", err)
	}

	if !strings.Contains(output, "Daemon stopped") {
		t.Errorf("Expected 'Daemon stopped' message, got: %s", output)
	}
}

func TestCLIDaemonStatus(t *testing.T) {
	setupTestEnv(t)

	output, err := runCLI(t, "daemon", "status")
	if err == nil {
		t.Errorf("Expected error when daemon not running")
	}

	if !strings.Contains(output, "not running") {
		t.Errorf("Expected 'not running' status, got: %s", output)
	}

	_, err = runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer runCLI(t, "daemon", "stop") //nolint:errcheck

	time.Sleep(50 * time.Millisecond)

	output, err = runCLI(t, "daemon", "status")
	if err != nil {
		t.Fatalf("daemon status failed: %v", err)
	}

	if !strings.Contains(output, "running") {
		t.Errorf("Expected 'running' status, got: %s", output)
	}
}

func TestCLIList(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// V2: sessions removed, sessionID can be empty
	createTestEvent(t, db, "event-1", "", "First test prompt")
	createTestEvent(t, db, "event-2", "", "Second test prompt")

	output, err := runCLI(t, "list")
	if err != nil {
		t.Fatalf("list command failed: %v", err)
	}

	if !strings.Contains(output, "event-1") {
		t.Errorf("Expected event-1 in output, got: %s", output)
	}

	if !strings.Contains(output, "First test prompt") {
		t.Errorf("Expected prompt text in output, got: %s", output)
	}

	if !strings.Contains(output, "event-2") {
		t.Errorf("Expected event-2 in output, got: %s", output)
	}
}

func TestCLIListLimit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// V2: sessions removed, sessionID can be empty
	for i := 0; i < 10; i++ {
		createTestEvent(t, db, fmt.Sprintf("event-%d", i), "", fmt.Sprintf("Prompt %d", i))
	}

	output, err := runCLI(t, "list", "--limit", "3")
	if err != nil {
		t.Fatalf("list command failed: %v", err)
	}

	if !strings.Contains(output, "Showing 3 event(s)") {
		t.Errorf("Expected 'Showing 3 event(s)', got: %s", output)
	}

	// Verify we got at most 3 event lines (not counting header, separator, summary)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	eventCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "event-") {
			eventCount++
		}
	}
	if eventCount > 3 {
		t.Errorf("Expected at most 3 events, got %d", eventCount)
	}
}

func TestCLIShow(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// V2: sessions removed, sessionID can be empty
	createTestEvent(t, db, "show-event-1", "", "Show this prompt")

	output, err := runCLI(t, "show", "show-event-1")
	if err != nil {
		t.Fatalf("show command failed: %v", err)
	}

	if !strings.Contains(output, "show-event-1") {
		t.Errorf("Expected event ID in output, got: %s", output)
	}

	if !strings.Contains(output, "Show this prompt") {
		t.Errorf("Expected prompt text in output, got: %s", output)
	}

	// V2: session ID check removed - sessions no longer exist
}

func TestCLIShowNotFound(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "show", "nonexistent-id")
	if err == nil {
		t.Error("Expected error for non-existent event")
	}

	if !strings.Contains(output, "not found") {
		t.Errorf("Expected 'not found' message, got: %s", output)
	}
}

func TestCLISearch(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// V2: sessions removed, sessionID can be empty
	createTestEvent(t, db, "search-1", "", "Implement authentication feature")
	createTestEvent(t, db, "search-2", "", "Fix bug in database layer")
	createTestEvent(t, db, "search-3", "", "Add authentication tests")

	output, err := runCLI(t, "search", "authentication")
	if err != nil {
		t.Fatalf("search command failed: %v", err)
	}

	if !strings.Contains(output, "search-1") {
		t.Errorf("Expected search-1 in results, got: %s", output)
	}

	if !strings.Contains(output, "search-3") {
		t.Errorf("Expected search-3 in results, got: %s", output)
	}

	if strings.Contains(output, "search-2") {
		t.Errorf("search-2 should not appear in results, got: %s", output)
	}
}

// Helper functions

func setupTestEnv(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "prov-cli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) }) //nolint:errcheck

	// Set environment variables for CLI to use test directory
	os.Setenv("AI_PROVENANCE_HOME", tmpDir)                 //nolint:errcheck
	t.Cleanup(func() { os.Unsetenv("AI_PROVENANCE_HOME") }) //nolint:errcheck

	return tmpDir
}

func setupTestDB(t *testing.T, tmpDir string) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(tmpDir, "db.sqlite")
	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	return db
}

// V2 NOTE: createTestSession removed - sessions no longer exist in v2

func createTestEvent(t *testing.T, db *sql.DB, eventID, sessionID, promptText string) {
	t.Helper()

	event := &storage.PromptEvent{
		ID:         eventID,
		Timestamp:  time.Now(),
		SessionID:  sessionID,
		Agent:      "test-cli",
		PromptText: promptText,
		RepoPath:   "/home/user/test",
		Author:     "testuser",
	}

	if err := storage.StorePromptEvent(db, event); err != nil {
		t.Fatalf("Failed to create test event: %v", err)
	}
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command(testBinary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return output, err
}

// TestCLISessionListTableAlignment tests that session IDs are truncated
// to prevent table misalignment when mixing long UUIDs and short IDs
func TestCLISessionListTableAlignment(t *testing.T) {
	t.Skip("V2: Session commands removed - v2 uses commit windows instead of sessions")
}

func TestCLIStatsRepo(t *testing.T) {
	t.Skip("V2: Stats command updated - session-based stats replaced with commit-based stats")
}

func TestCLIStatsSession(t *testing.T) {
	t.Skip("V2: Session-specific stats removed - v2 uses commit windows")
}

func TestCLIStatsSince(t *testing.T) {
	t.Skip("V2: Stats command updated - will be reimplemented with commit-based approach")
}

func TestCLIStatsNoData(t *testing.T) {
	t.Skip("V2: Stats command updated - will be reimplemented with commit-based approach")
}

func TestCLIExportJSON(t *testing.T) {
	t.Skip("V2: Export command not yet fully implemented in v2 architecture")
}

func TestCLIExportCSV(t *testing.T) {
	t.Skip("V2: Export command not yet fully implemented in v2 architecture")
}

func TestCLIExportSession(t *testing.T) {
	t.Skip("V2: Export command not yet fully implemented in v2 architecture")
}

func TestCLIExportSince(t *testing.T) {
	t.Skip("V2: Export command not yet fully implemented in v2 architecture")
}

func TestCLIExportToFile(t *testing.T) {
	t.Skip("V2: Export command not yet fully implemented in v2 architecture")
}

func TestCLIExportNoData(t *testing.T) {
	t.Skip("V2: Export command not yet fully implemented in v2 architecture")
}
