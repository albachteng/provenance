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

// TestCLIVersion tests the version command
func TestCLIVersion(t *testing.T) {
	output, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	if !strings.Contains(output, "prov version") {
		t.Errorf("Expected version output, got: %s", output)
	}
}

// TestCLIDaemonStart tests starting the daemon
func TestCLIDaemonStart(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Start daemon in background
	output, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("daemon start failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Daemon started") {
		t.Errorf("Expected 'Daemon started' message, got: %s", output)
	}

	// Verify daemon is running (socket file exists)
	socketPath := filepath.Join(tmpDir, "daemon.sock")
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("Daemon socket file not created")
	}

	// Cleanup
	runCLI(t, "daemon", "stop")
}

// TestCLIDaemonStop tests stopping the daemon
func TestCLIDaemonStop(t *testing.T) {
	setupTestEnv(t)

	// Start daemon first
	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	// Stop daemon
	output, err := runCLI(t, "daemon", "stop")
	if err != nil {
		t.Fatalf("daemon stop failed: %v", err)
	}

	if !strings.Contains(output, "Daemon stopped") {
		t.Errorf("Expected 'Daemon stopped' message, got: %s", output)
	}
}

// TestCLIDaemonStatus tests checking daemon status
func TestCLIDaemonStatus(t *testing.T) {
	setupTestEnv(t)

	// Check status when daemon is not running (should exit with error)
	output, err := runCLI(t, "daemon", "status")
	if err == nil {
		t.Errorf("Expected error when daemon not running")
	}

	if !strings.Contains(output, "not running") {
		t.Errorf("Expected 'not running' status, got: %s", output)
	}

	// Start daemon
	_, err = runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer runCLI(t, "daemon", "stop")

	time.Sleep(100 * time.Millisecond)

	// Check status when daemon is running
	output, err = runCLI(t, "daemon", "status")
	if err != nil {
		t.Fatalf("daemon status failed: %v", err)
	}

	if !strings.Contains(output, "running") {
		t.Errorf("Expected 'running' status, got: %s", output)
	}
}

// TestCLIList tests listing prompt events
func TestCLIList(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test session and events
	sessionID := "test-session-cli"
	createTestSession(t, db, sessionID)
	createTestEvent(t, db, "event-1", sessionID, "First test prompt")
	createTestEvent(t, db, "event-2", sessionID, "Second test prompt")

	// List events
	output, err := runCLI(t, "list")
	if err != nil {
		t.Fatalf("list command failed: %v", err)
	}

	// Verify both events appear in output
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

// TestCLIListLimit tests listing with a limit
func TestCLIListLimit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test session and events
	sessionID := "test-session-limit"
	createTestSession(t, db, sessionID)
	for i := 0; i < 10; i++ {
		createTestEvent(t, db, fmt.Sprintf("event-%d", i), sessionID, fmt.Sprintf("Prompt %d", i))
	}

	// List with limit
	output, err := runCLI(t, "list", "--limit", "3")
	if err != nil {
		t.Fatalf("list command failed: %v", err)
	}

	// Verify the summary line shows correct count
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

// TestCLIShow tests showing a specific event
func TestCLIShow(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test session and event
	sessionID := "test-session-show"
	createTestSession(t, db, sessionID)
	createTestEvent(t, db, "show-event-1", sessionID, "Show this prompt")

	// Show event
	output, err := runCLI(t, "show", "show-event-1")
	if err != nil {
		t.Fatalf("show command failed: %v", err)
	}

	// Verify detailed output
	if !strings.Contains(output, "show-event-1") {
		t.Errorf("Expected event ID in output, got: %s", output)
	}

	if !strings.Contains(output, "Show this prompt") {
		t.Errorf("Expected prompt text in output, got: %s", output)
	}

	if !strings.Contains(output, sessionID) {
		t.Errorf("Expected session ID in output, got: %s", output)
	}
}

// TestCLIShowNotFound tests showing a non-existent event
func TestCLIShowNotFound(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	// Try to show non-existent event
	output, err := runCLI(t, "show", "nonexistent-id")
	if err == nil {
		t.Error("Expected error for non-existent event")
	}

	if !strings.Contains(output, "not found") {
		t.Errorf("Expected 'not found' message, got: %s", output)
	}
}

// TestCLISearch tests searching prompt events
func TestCLISearch(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test session and events
	sessionID := "test-session-search"
	createTestSession(t, db, sessionID)
	createTestEvent(t, db, "search-1", sessionID, "Implement authentication feature")
	createTestEvent(t, db, "search-2", sessionID, "Fix bug in database layer")
	createTestEvent(t, db, "search-3", sessionID, "Add authentication tests")

	// Search for "authentication"
	output, err := runCLI(t, "search", "authentication")
	if err != nil {
		t.Fatalf("search command failed: %v", err)
	}

	// Verify matching events appear
	if !strings.Contains(output, "search-1") {
		t.Errorf("Expected search-1 in results, got: %s", output)
	}

	if !strings.Contains(output, "search-3") {
		t.Errorf("Expected search-3 in results, got: %s", output)
	}

	// Verify non-matching event doesn't appear
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
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Set environment variables for CLI to use test directory
	os.Setenv("AI_PROVENANCE_HOME", tmpDir)
	t.Cleanup(func() { os.Unsetenv("AI_PROVENANCE_HOME") })

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

func createTestSession(t *testing.T, db *sql.DB, sessionID string) {
	t.Helper()

	session := &storage.Session{
		ID:        sessionID,
		StartTime: time.Now(),
		RepoPath:  "/home/user/test",
	}

	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
}

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

	// Build the CLI binary if not already built
	binary := buildCLI(t)

	cmd := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr for output
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return output, err
}

func buildCLI(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "prov-test")

	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = filepath.Join(".") // Current directory (cmd/prov)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	return binary
}
