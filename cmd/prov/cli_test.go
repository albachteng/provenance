package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
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
	defer db.Close() //nolint:errcheck

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
	defer db.Close() //nolint:errcheck

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
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(originalDir) //nolint:errcheck

	output, err := runCLI(t, "stats")
	if err != nil {
		t.Fatalf("stats command failed: %v", err)
	}

	if !strings.Contains(output, "Total Prompts: 0") && !strings.Contains(output, "No prompts") {
		t.Errorf("Expected empty stats message, got: %s", output)
	}
}

func TestCLIExportJSON(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	repoPath := tmpDir
	now := time.Now()

	session := &storage.Session{
		ID:        "export-session-1",
		StartTime: now.Add(-1 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	events := []*storage.PromptEvent{
		{
			ID:             "export-event-1",
			Timestamp:      now.Add(-50 * time.Minute),
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Implement feature X",
			ResponseText:   "Here's the implementation...",
			TokensIn:       100,
			TokensOut:      300,
			RepoPath:       repoPath,
			Author:         "testuser",
			ToolsInvoked:   []string{"read_file", "write_file"},
			FilesMentioned: []string{"src/feature.go"},
		},
		{
			ID:             "export-event-2",
			Timestamp:      now.Add(-30 * time.Minute),
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Add tests",
			ResponseText:   "I'll add tests...",
			TokensIn:       80,
			TokensOut:      250,
			RepoPath:       repoPath,
			Author:         "testuser",
			ToolsInvoked:   []string{"write_file"},
			FilesMentioned: []string{"src/feature_test.go"},
		},
	}

	for _, event := range events {
		if err := storage.StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event: %v", err)
		}
	}

	output, err := runCLI(t, "export", "--format", "json")
	if err != nil {
		t.Fatalf("export command failed: %v", err)
	}

	var exported []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &exported); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if len(exported) != 2 {
		t.Errorf("Expected 2 exported events, got %d", len(exported))
	}

	if exported[0]["ID"] != "export-event-1" && exported[0]["ID"] != "export-event-2" {
		t.Errorf("Expected event IDs to be export-event-1 or export-event-2, got %v", exported[0]["ID"])
	}

	if exported[0]["PromptText"] == nil {
		t.Error("Expected PromptText field in exported JSON")
	}

	if exported[0]["TokensIn"] == nil {
		t.Error("Expected TokensIn field in exported JSON")
	}
}

func TestCLIExportCSV(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	repoPath := tmpDir
	now := time.Now()

	session := &storage.Session{
		ID:        "export-csv-session",
		StartTime: now.Add(-1 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	event := &storage.PromptEvent{
		ID:             "export-csv-event",
		Timestamp:      now.Add(-30 * time.Minute),
		Agent:          "claude-code",
		ModelVersion:   "sonnet-4.5",
		PromptText:     "Fix bug",
		ResponseText:   "Fixed!",
		TokensIn:       50,
		TokensOut:      100,
		RepoPath:       repoPath,
		Author:         "testuser",
		ToolsInvoked:   []string{"edit_file"},
		FilesMentioned: []string{"src/bug.go"},
	}

	if err := storage.StorePromptEvent(db, event); err != nil {
		t.Fatalf("Failed to store event: %v", err)
	}

	output, err := runCLI(t, "export", "--format", "csv")
	if err != nil {
		t.Fatalf("export --format csv command failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("Expected at least 2 lines (header + data), got %d", len(lines))
	}

	header := lines[0]
	if !strings.Contains(header, "id") || !strings.Contains(header, "timestamp") || !strings.Contains(header, "prompt_text") {
		t.Errorf("CSV header missing expected columns, got: %s", header)
	}

	dataLine := lines[1]
	if !strings.Contains(dataLine, "export-csv-event") {
		t.Errorf("Expected event ID in CSV data, got: %s", dataLine)
	}

	if !strings.Contains(dataLine, "claude-code") {
		t.Errorf("Expected agent name in CSV data, got: %s", dataLine)
	}
}

func TestCLIExportSession(t *testing.T) {
	t.Skip("V2: --session flag no longer filters by session (sessions removed); export all events instead")
}

func TestCLIExportSince(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	repoPath := tmpDir
	now := time.Now()

	session := &storage.Session{
		ID:        "export-timeframe-session",
		StartTime: now.Add(-10 * 24 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	events := []*storage.PromptEvent{
		{
			ID:           "old-event-export",
			Timestamp:    now.Add(-9 * 24 * time.Hour),
			Agent:        "claude-code",
			PromptText:   "Old event",
			TokensIn:     50,
			TokensOut:    100,
			RepoPath:     repoPath,
			Author:       "testuser",
			ToolsInvoked: []string{"read"},
		},
		{
			ID:           "recent-event-export",
			Timestamp:    now.Add(-3 * 24 * time.Hour),
			Agent:        "claude-code",
			PromptText:   "Recent event",
			TokensIn:     100,
			TokensOut:    200,
			RepoPath:     repoPath,
			Author:       "testuser",
			ToolsInvoked: []string{"write"},
		},
	}

	for _, event := range events {
		if err := storage.StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event: %v", err)
		}
	}

	output, err := runCLI(t, "export", "--since", "7 days ago", "--format", "json")
	if err != nil {
		t.Fatalf("export --since command failed: %v", err)
	}

	var exported []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &exported); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if len(exported) != 1 {
		t.Errorf("Expected 1 recent event, got %d", len(exported))
	}

	if exported[0]["ID"] != "recent-event-export" {
		t.Errorf("Expected recent-event-export, got %v", exported[0]["ID"])
	}

	if strings.Contains(output, "old-event-export") {
		t.Error("Should not export events older than 7 days")
	}
}

func TestCLIExportToFile(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	repoPath := tmpDir
	now := time.Now()

	session := &storage.Session{
		ID:        "export-file-session",
		StartTime: now.Add(-1 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	event := &storage.PromptEvent{
		ID:           "export-file-event",
		Timestamp:    now.Add(-30 * time.Minute),
		Agent:        "claude-code",
		PromptText:   "Test export to file",
		TokensIn:     75,
		TokensOut:    150,
		RepoPath:     repoPath,
		Author:       "testuser",
		ToolsInvoked: []string{"bash"},
	}

	if err := storage.StorePromptEvent(db, event); err != nil {
		t.Fatalf("Failed to store event: %v", err)
	}

	outputFile := filepath.Join(tmpDir, "export.json")

	output, err := runCLI(t, "export", "--format", "json", "--output", outputFile)
	if err != nil {
		t.Fatalf("export --output command failed: %v", err)
	}

	if !strings.Contains(output, "Exported") || !strings.Contains(output, outputFile) {
		t.Errorf("Expected success message with file path, got: %s", output)
	}

	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created: %s", outputFile)
	}

	fileContent, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	var exported []map[string]interface{}
	if err := json.Unmarshal(fileContent, &exported); err != nil {
		t.Fatalf("Failed to parse JSON from file: %v", err)
	}

	if len(exported) != 1 {
		t.Errorf("Expected 1 event in file, got %d", len(exported))
	}

	if exported[0]["ID"] != "export-file-event" {
		t.Errorf("Expected export-file-event in file, got %v", exported[0]["ID"])
	}
}

func TestCLIExportNoData(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	output, err := runCLI(t, "export", "--format", "json")
	if err != nil {
		t.Fatalf("export command with no data failed: %v", err)
	}

	var exported []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &exported); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if len(exported) != 0 {
		t.Errorf("Expected empty array for no data, got %d events", len(exported))
	}
}
