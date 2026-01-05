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

// TestMain builds the CLI binary once for all tests
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "prov-test-binary-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

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

	runCLI(t, "daemon", "stop")
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
	defer runCLI(t, "daemon", "stop")

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
	defer db.Close()

	sessionID := "test-session-cli"
	createTestSession(t, db, sessionID)
	createTestEvent(t, db, "event-1", sessionID, "First test prompt")
	createTestEvent(t, db, "event-2", sessionID, "Second test prompt")

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
	defer db.Close()

	sessionID := "test-session-limit"
	createTestSession(t, db, sessionID)
	for i := 0; i < 10; i++ {
		createTestEvent(t, db, fmt.Sprintf("event-%d", i), sessionID, fmt.Sprintf("Prompt %d", i))
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
	defer db.Close()

	sessionID := "test-session-show"
	createTestSession(t, db, sessionID)
	createTestEvent(t, db, "show-event-1", sessionID, "Show this prompt")

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

	if !strings.Contains(output, sessionID) {
		t.Errorf("Expected session ID in output, got: %s", output)
	}
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
	defer db.Close()

	sessionID := "test-session-search"
	createTestSession(t, db, sessionID)
	createTestEvent(t, db, "search-1", sessionID, "Implement authentication feature")
	createTestEvent(t, db, "search-2", sessionID, "Fix bug in database layer")
	createTestEvent(t, db, "search-3", sessionID, "Add authentication tests")

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
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	now := time.Now()

	sessions := []struct {
		id       string
		repoPath string
	}{
		{"780bed8d-9aeb-4303-87de-be31e39bfef9", "/home/user/jobqueue"}, // Long UUID (36 chars)
		{"0fe4aa04-441b-4ca8-abe5-698dac925ec5", "/home/user/dev-env"},  // Long UUID (36 chars)
		{"test-session", "/home/user/provenance"},                       // Short ID (13 chars)
		{"session-provenance", "/home/user/project"},                    // Medium ID (18 chars)
	}

	for _, s := range sessions {
		session := &storage.Session{
			ID:        s.id,
			StartTime: now,
			RepoPath:  s.repoPath,
		}
		if err := storage.CreateSession(db, session); err != nil {
			t.Fatalf("Failed to create test session %s: %v", s.id, err)
		}
	}

	output, err := runCLI(t, "session", "list")
	if err != nil {
		t.Fatalf("session list failed: %v", err)
	}

	lines := strings.Split(output, "\n")

	var headerLine string
	var dataLines []string

	for _, line := range lines {
		if strings.Contains(line, "ID") && strings.Contains(line, "Start Time") {
			headerLine = line
		} else if strings.Contains(line, "jobqueue") || strings.Contains(line, "dev-env") ||
			strings.Contains(line, "project") || (strings.Contains(line, "provenance") && !strings.Contains(line, "ID")) {
			dataLines = append(dataLines, line)
		}
	}

	if headerLine == "" {
		t.Fatal("Header line not found in output")
	}

	if len(dataLines) != 4 {
		t.Fatalf("Expected 4 data lines, got %d", len(dataLines))
	}

	foundTruncatedUUID := false
	for i, line := range dataLines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			t.Errorf("Line %d has no fields: %q", i, line)
			continue
		}

		id := fields[0]
		if len(id) > 12 {
			t.Errorf("Line %d: Session ID %q is longer than 12 characters (len=%d), will cause misalignment",
				i, id, len(id))
		}

		if id == "780bed8d-9ae" || id == "0fe4aa04-441" {
			foundTruncatedUUID = true
		}
	}

	if !foundTruncatedUUID {
		t.Error("Expected to find at least one truncated UUID (e.g., '780bed8d-9ae' or '0fe4aa04-441')")
	}

	startTimePos := strings.Index(headerLine, "Start Time")
	if startTimePos == -1 {
		t.Fatal("'Start Time' not found in header")
	}

	for i, line := range dataLines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		timestampIdx := strings.Index(line, fmt.Sprintf("%d-", now.Year()))
		if timestampIdx == -1 {
			t.Errorf("Line %d: No timestamp found in line: %q", i, line)
			continue
		}

		if abs(timestampIdx-startTimePos) > 5 { // Allow 5 character variance for spacing
			t.Errorf("Line %d: Timestamp at position %d, expected near %d (diff=%d). Table is misaligned.\nLine: %q",
				i, timestampIdx, startTimePos, abs(timestampIdx-startTimePos), line)
		}
	}
}

// abs returns the absolute value of x
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestCLIStatsRepo(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Initialize git repo in test directory
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Save current directory and change to test directory
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(originalDir)

	repoPath := tmpDir
	now := time.Now()

	session1 := &storage.Session{
		ID:        "stats-session-1",
		StartTime: now.Add(-1 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session1); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	events := []*storage.PromptEvent{
		{
			ID:             "stats-event-1",
			Timestamp:      now.Add(-50 * time.Minute),
			SessionID:      "stats-session-1",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Fix bug in auth",
			ResponseText:   "Here's the fix...",
			TokensIn:       100,
			TokensOut:      300,
			RepoPath:       repoPath,
			Author:         "testuser",
			ToolsInvoked:   []string{"read_file", "write_file"},
			FilesMentioned: []string{"src/auth.go", "src/db.go"},
		},
		{
			ID:             "stats-event-2",
			Timestamp:      now.Add(-30 * time.Minute),
			SessionID:      "stats-session-1",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Add tests",
			ResponseText:   "I'll add tests...",
			TokensIn:       150,
			TokensOut:      400,
			RepoPath:       repoPath,
			Author:         "testuser",
			ToolsInvoked:   []string{"write_file", "bash"},
			FilesMentioned: []string{"src/auth.go"},
		},
	}

	for _, event := range events {
		if err := storage.StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event: %v", err)
		}
	}

	output, err := runCLI(t, "stats")
	if err != nil {
		t.Fatalf("stats command failed: %v", err)
	}

	if !strings.Contains(output, "Total Prompts: 2") {
		t.Errorf("Expected 'Total Prompts: 2', got: %s", output)
	}

	if !strings.Contains(output, "Tokens In: 250") {
		t.Errorf("Expected 'Tokens In: 250', got: %s", output)
	}

	if !strings.Contains(output, "Tokens Out: 700") {
		t.Errorf("Expected 'Tokens Out: 700', got: %s", output)
	}

	if !strings.Contains(output, "Sessions: 1") {
		t.Errorf("Expected 'Sessions: 1', got: %s", output)
	}

	if !strings.Contains(output, "src/auth.go") {
		t.Errorf("Expected file 'src/auth.go' in output, got: %s", output)
	}

	if !strings.Contains(output, "write_file") {
		t.Errorf("Expected tool 'write_file' in output, got: %s", output)
	}
}

func TestCLIStatsSession(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	repoPath := tmpDir
	now := time.Now()

	session := &storage.Session{
		ID:        "stats-session-specific",
		StartTime: now.Add(-1 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	events := []*storage.PromptEvent{
		{
			ID:             "session-event-1",
			Timestamp:      now.Add(-50 * time.Minute),
			SessionID:      "stats-session-specific",
			Agent:          "claude-code",
			PromptText:     "Implement feature",
			TokensIn:       120,
			TokensOut:      350,
			RepoPath:       repoPath,
			Author:         "testuser",
			ToolsInvoked:   []string{"read_file"},
			FilesMentioned: []string{"src/feature.go"},
		},
	}

	for _, event := range events {
		if err := storage.StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event: %v", err)
		}
	}

	output, err := runCLI(t, "stats", "--session", "stats-session-specific")
	if err != nil {
		t.Fatalf("stats --session command failed: %v", err)
	}

	if !strings.Contains(output, "Session: stats-session-specific") {
		t.Errorf("Expected session ID in output, got: %s", output)
	}

	if !strings.Contains(output, "Total Prompts: 1") {
		t.Errorf("Expected 'Total Prompts: 1', got: %s", output)
	}

	if !strings.Contains(output, "Tokens In: 120") {
		t.Errorf("Expected 'Tokens In: 120', got: %s", output)
	}

	if !strings.Contains(output, "src/feature.go") {
		t.Errorf("Expected file 'src/feature.go' in output, got: %s", output)
	}
}

func TestCLIStatsSince(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Initialize git repo in test directory
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Save current directory and change to test directory
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(originalDir)

	repoPath := tmpDir
	now := time.Now()

	session := &storage.Session{
		ID:        "stats-timeframe-session",
		StartTime: now.Add(-10 * 24 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	events := []*storage.PromptEvent{
		{
			ID:             "old-event",
			Timestamp:      now.Add(-9 * 24 * time.Hour),
			SessionID:      "stats-timeframe-session",
			Agent:          "claude-code",
			PromptText:     "Old event",
			TokensIn:       50,
			TokensOut:      100,
			RepoPath:       repoPath,
			Author:         "testuser",
			ToolsInvoked:   []string{"read_file"},
			FilesMentioned: []string{"old.go"},
		},
		{
			ID:             "recent-event",
			Timestamp:      now.Add(-3 * 24 * time.Hour),
			SessionID:      "stats-timeframe-session",
			Agent:          "claude-code",
			PromptText:     "Recent event",
			TokensIn:       100,
			TokensOut:      200,
			RepoPath:       repoPath,
			Author:         "testuser",
			ToolsInvoked:   []string{"write_file"},
			FilesMentioned: []string{"recent.go"},
		},
	}

	for _, event := range events {
		if err := storage.StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event: %v", err)
		}
	}

	output, err := runCLI(t, "stats", "--since", "7 days ago")
	if err != nil {
		t.Fatalf("stats --since command failed: %v", err)
	}

	if !strings.Contains(output, "Total Prompts: 1") {
		t.Errorf("Expected 'Total Prompts: 1' (only recent event), got: %s", output)
	}

	if !strings.Contains(output, "Tokens In: 100") {
		t.Errorf("Expected 'Tokens In: 100', got: %s", output)
	}

	if !strings.Contains(output, "recent.go") {
		t.Errorf("Expected file 'recent.go' in output, got: %s", output)
	}

	if strings.Contains(output, "old.go") {
		t.Errorf("Did not expect 'old.go' in recent stats, got: %s", output)
	}
}

func TestCLIStatsNoData(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Initialize git repo in test directory
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Save current directory and change to test directory
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(originalDir)

	output, err := runCLI(t, "stats")
	if err != nil {
		t.Fatalf("stats command failed: %v", err)
	}

	if !strings.Contains(output, "Total Prompts: 0") && !strings.Contains(output, "No prompts") {
		t.Errorf("Expected empty stats message, got: %s", output)
	}
}
