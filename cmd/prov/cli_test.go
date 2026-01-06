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

	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

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

	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

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

	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

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

func TestCLIExportJSON(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

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
			SessionID:      "export-session-1",
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
			SessionID:      "export-session-1",
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

	if exported[0]["id"] != "export-event-1" && exported[0]["id"] != "export-event-2" {
		t.Errorf("Expected event IDs to be export-event-1 or export-event-2, got %v", exported[0]["id"])
	}

	if exported[0]["prompt_text"] == nil {
		t.Error("Expected prompt_text field in exported JSON")
	}

	if exported[0]["tokens_in"] == nil {
		t.Error("Expected tokens_in field in exported JSON")
	}
}

func TestCLIExportCSV(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

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
		SessionID:      "export-csv-session",
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
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	repoPath := tmpDir
	now := time.Now()

	session1 := &storage.Session{
		ID:        "export-specific-session",
		StartTime: now.Add(-2 * time.Hour),
		RepoPath:  repoPath,
	}
	session2 := &storage.Session{
		ID:        "export-other-session",
		StartTime: now.Add(-3 * time.Hour),
		RepoPath:  repoPath,
	}
	if err := storage.CreateSession(db, session1); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	if err := storage.CreateSession(db, session2); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}

	events := []*storage.PromptEvent{
		{
			ID:           "event-in-target-session",
			Timestamp:    now.Add(-100 * time.Minute),
			SessionID:    "export-specific-session",
			Agent:        "claude-code",
			PromptText:   "Target session event",
			TokensIn:     100,
			TokensOut:    200,
			RepoPath:     repoPath,
			Author:       "testuser",
			ToolsInvoked: []string{"read"},
		},
		{
			ID:           "event-in-other-session",
			Timestamp:    now.Add(-150 * time.Minute),
			SessionID:    "export-other-session",
			Agent:        "claude-code",
			PromptText:   "Other session event",
			TokensIn:     50,
			TokensOut:    100,
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

	output, err := runCLI(t, "export", "--session", "export-specific-session", "--format", "json")
	if err != nil {
		t.Fatalf("export --session command failed: %v", err)
	}

	var exported []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &exported); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if len(exported) != 1 {
		t.Errorf("Expected 1 exported event (only from target session), got %d", len(exported))
	}

	if exported[0]["id"] != "event-in-target-session" {
		t.Errorf("Expected event-in-target-session, got %v", exported[0]["id"])
	}

	if strings.Contains(output, "event-in-other-session") {
		t.Error("Should not export events from other sessions")
	}
}

func TestCLIExportSince(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

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
			SessionID:    "export-timeframe-session",
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
			SessionID:    "export-timeframe-session",
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

	if exported[0]["id"] != "recent-event-export" {
		t.Errorf("Expected recent-event-export, got %v", exported[0]["id"])
	}

	if strings.Contains(output, "old-event-export") {
		t.Error("Should not export events older than 7 days")
	}
}

func TestCLIExportToFile(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

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
		SessionID:    "export-file-session",
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

	if exported[0]["id"] != "export-file-event" {
		t.Errorf("Expected export-file-event in file, got %v", exported[0]["id"])
	}
}

func TestCLIExportNoData(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

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
