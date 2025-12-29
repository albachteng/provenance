package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePromptEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create session first (required by foreign key constraint)
	session := &Session{
		ID:        "test-session-1",
		StartTime: time.Now(),
		RepoPath:  "/home/user/project",
	}
	if err := CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	event := &PromptEvent{
		ID:           "test-event-1",
		Timestamp:    time.Now(),
		SessionID:    "test-session-1",
		Agent:        "claude-code",
		ModelVersion: "sonnet-4.5",
		PromptText:   "Implement user authentication",
		ResponseText: "I'll help you implement authentication...",
		TokensIn:     50,
		TokensOut:    200,
		LatencyMs:    1500,
		RepoPath:     "/home/user/project",
		GitCommit:    "abc123",
		GitBranch:    "main",
		GitDirty:     false,
		DirtyFiles:   []string{},
		Author:       "testuser",
		IDE:          "vscode",
		ActiveFile:   "src/auth.go",
		WorkspaceFiles: []string{
			"src/auth.go",
			"src/main.go",
		},
		PromptType:     "chat",
		ToolsInvoked:   []string{"read_file", "write_file"},
		FilesMentioned: []string{"src/auth.go"},
	}

	err := StorePromptEvent(db, event)
	if err != nil {
		t.Fatalf("Failed to store prompt event: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM prompt_events WHERE id = ?", event.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query prompt event: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 event, got %d", count)
	}
}

func TestGetPromptEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create session first (required by foreign key constraint)
	session := &Session{
		ID:        "test-session-2",
		StartTime: time.Now(),
		RepoPath:  "/home/user/app",
	}
	if err := CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	originalEvent := &PromptEvent{
		ID:           "test-event-2",
		Timestamp:    time.Now().Truncate(time.Second), // Truncate for comparison
		SessionID:    "test-session-2",
		Agent:        "cursor",
		ModelVersion: "gpt-4",
		PromptText:   "Fix the bug in login",
		ResponseText: "The issue is in the validation...",
		TokensIn:     30,
		TokensOut:    150,
		LatencyMs:    800,
		RepoPath:     "/home/user/app",
		GitCommit:    "def456",
		GitBranch:    "feature/login",
		GitDirty:     true,
		DirtyFiles:   []string{" M src/login.go", "?? test.txt"},
		Author:       "devuser",
		IDE:          "neovim",
		ActiveFile:   "src/login.go",
		WorkspaceFiles: []string{
			"src/login.go",
			"src/auth.go",
		},
		PromptType:     "edit",
		ToolsInvoked:   []string{"edit_file"},
		FilesMentioned: []string{"src/login.go"},
	}

	err := StorePromptEvent(db, originalEvent)
	if err != nil {
		t.Fatalf("Failed to store prompt event: %v", err)
	}

	retrievedEvent, err := GetPromptEvent(db, originalEvent.ID)
	if err != nil {
		t.Fatalf("Failed to get prompt event: %v", err)
	}

	if retrievedEvent.ID != originalEvent.ID {
		t.Errorf("Expected ID %s, got %s", originalEvent.ID, retrievedEvent.ID)
	}

	if retrievedEvent.Agent != originalEvent.Agent {
		t.Errorf("Expected agent %s, got %s", originalEvent.Agent, retrievedEvent.Agent)
	}

	if retrievedEvent.PromptText != originalEvent.PromptText {
		t.Errorf("Expected prompt %s, got %s", originalEvent.PromptText, retrievedEvent.PromptText)
	}

	if retrievedEvent.GitDirty != originalEvent.GitDirty {
		t.Errorf("Expected git_dirty %v, got %v", originalEvent.GitDirty, retrievedEvent.GitDirty)
	}

	if len(retrievedEvent.DirtyFiles) != len(originalEvent.DirtyFiles) {
		t.Errorf("Expected %d dirty files, got %d", len(originalEvent.DirtyFiles), len(retrievedEvent.DirtyFiles))
	}

	if len(retrievedEvent.WorkspaceFiles) != len(originalEvent.WorkspaceFiles) {
		t.Errorf("Expected %d workspace files, got %d", len(originalEvent.WorkspaceFiles), len(retrievedEvent.WorkspaceFiles))
	}

	if len(retrievedEvent.ToolsInvoked) != len(originalEvent.ToolsInvoked) {
		t.Errorf("Expected %d tools invoked, got %d", len(originalEvent.ToolsInvoked), len(retrievedEvent.ToolsInvoked))
	}
}

func TestGetPromptEventNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := GetPromptEvent(db, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent event, got nil")
	}

	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestListPromptEvents(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sessionID := "test-session-3"

	// Create session first (required by foreign key constraint)
	session := &Session{
		ID:        sessionID,
		StartTime: time.Now(),
		RepoPath:  "/home/user/project",
	}
	if err := CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	events := []*PromptEvent{
		{
			ID:         "event-1",
			Timestamp:  time.Now().Add(-10 * time.Minute),
			SessionID:  sessionID,
			Agent:      "claude-code",
			PromptText: "First prompt",
			RepoPath:   "/home/user/project",
			Author:     "testuser",
		},
		{
			ID:         "event-2",
			Timestamp:  time.Now().Add(-5 * time.Minute),
			SessionID:  sessionID,
			Agent:      "claude-code",
			PromptText: "Second prompt",
			RepoPath:   "/home/user/project",
			Author:     "testuser",
		},
		{
			ID:         "event-3",
			Timestamp:  time.Now(),
			SessionID:  sessionID,
			Agent:      "claude-code",
			PromptText: "Third prompt",
			RepoPath:   "/home/user/project",
			Author:     "testuser",
		},
	}

	for _, event := range events {
		if err := StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event: %v", err)
		}
	}

	retrieved, err := ListPromptEvents(db, sessionID)
	if err != nil {
		t.Fatalf("Failed to list prompt events: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("Expected 3 events, got %d", len(retrieved))
	}

	// Verify events are in chronological order (oldest first)
	for i := 0; i < len(retrieved)-1; i++ {
		if retrieved[i].Timestamp.After(retrieved[i+1].Timestamp) {
			t.Error("Events are not in chronological order")
		}
	}
}

func TestStorePromptEventWithJSONFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create session first (required by foreign key constraint)
	session := &Session{
		ID:        "test-session-json",
		StartTime: time.Now(),
		RepoPath:  "/home/user/complex-project",
	}
	if err := CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	event := &PromptEvent{
		ID:         "test-event-json",
		Timestamp:  time.Now(),
		SessionID:  "test-session-json",
		Agent:      "aider",
		PromptText: "Complex refactoring",
		RepoPath:   "/home/user/complex-project",
		Author:     "testuser",
		DirtyFiles: []string{
			" M src/main.go",
			" M src/utils.go",
			"?? new_file.go",
		},
		WorkspaceFiles: []string{
			"src/main.go",
			"src/utils.go",
			"src/auth.go",
			"test/main_test.go",
		},
		ToolsInvoked: []string{
			"read_file",
			"write_file",
			"run_command",
		},
		FilesMentioned: []string{
			"src/main.go",
			"src/utils.go",
		},
	}

	err := StorePromptEvent(db, event)
	if err != nil {
		t.Fatalf("Failed to store prompt event with JSON fields: %v", err)
	}

	retrieved, err := GetPromptEvent(db, event.ID)
	if err != nil {
		t.Fatalf("Failed to get prompt event: %v", err)
	}

	if len(retrieved.DirtyFiles) != len(event.DirtyFiles) {
		t.Errorf("Expected %d dirty files, got %d", len(event.DirtyFiles), len(retrieved.DirtyFiles))
	}
	for i, file := range event.DirtyFiles {
		if retrieved.DirtyFiles[i] != file {
			t.Errorf("DirtyFiles[%d]: expected %s, got %s", i, file, retrieved.DirtyFiles[i])
		}
	}

	if len(retrieved.WorkspaceFiles) != len(event.WorkspaceFiles) {
		t.Errorf("Expected %d workspace files, got %d", len(event.WorkspaceFiles), len(retrieved.WorkspaceFiles))
	}

	if len(retrieved.ToolsInvoked) != len(event.ToolsInvoked) {
		t.Errorf("Expected %d tools invoked, got %d", len(event.ToolsInvoked), len(retrieved.ToolsInvoked))
	}

	if len(retrieved.FilesMentioned) != len(event.FilesMentioned) {
		t.Errorf("Expected %d files mentioned, got %d", len(event.FilesMentioned), len(retrieved.FilesMentioned))
	}
}

func TestCreateSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	session := &Session{
		ID:           "session-1",
		StartTime:    time.Now().Truncate(time.Second),
		EndTime:      nil, // Active session
		RepoPath:     "/home/user/project",
		TotalPrompts: 0,
		TotalTokens:  0,
		EndedBy:      "",
	}

	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", session.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query session: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 session, got %d", count)
	}
}

func TestGetActiveSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repoPath := "/home/user/active-project"

	session := &Session{
		ID:           "active-session",
		StartTime:    time.Now().Truncate(time.Second),
		EndTime:      nil,
		RepoPath:     repoPath,
		TotalPrompts: 5,
		TotalTokens:  1000,
		EndedBy:      "",
	}

	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	retrieved, err := GetActiveSession(db, repoPath)
	if err != nil {
		t.Fatalf("Failed to get active session: %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, retrieved.ID)
	}

	if retrieved.EndTime != nil {
		t.Error("Expected active session (EndTime = nil)")
	}

	if retrieved.TotalPrompts != session.TotalPrompts {
		t.Errorf("Expected %d prompts, got %d", session.TotalPrompts, retrieved.TotalPrompts)
	}
}

func TestGetActiveSessionNone(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := GetActiveSession(db, "/home/user/no-session")
	if err == nil {
		t.Error("Expected error when no active session exists")
	}

	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestEndSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	session := &Session{
		ID:           "session-to-end",
		StartTime:    time.Now().Add(-30 * time.Minute).Truncate(time.Second),
		EndTime:      nil,
		RepoPath:     "/home/user/project",
		TotalPrompts: 10,
		TotalTokens:  5000,
		EndedBy:      "",
	}

	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	endTime := time.Now().Truncate(time.Second)
	err = EndSession(db, session.ID, endTime, "commit")
	if err != nil {
		t.Fatalf("Failed to end session: %v", err)
	}

	var endedBy string
	var endTimeDB *int64
	err = db.QueryRow("SELECT ended_by, end_time FROM sessions WHERE id = ?", session.ID).Scan(&endedBy, &endTimeDB)
	if err != nil {
		t.Fatalf("Failed to query session: %v", err)
	}

	if endedBy != "commit" {
		t.Errorf("Expected ended_by 'commit', got %s", endedBy)
	}

	if endTimeDB == nil {
		t.Error("Expected end_time to be set")
	}
}

func TestUpdateSessionMetrics(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	session := &Session{
		ID:           "session-metrics",
		StartTime:    time.Now().Truncate(time.Second),
		EndTime:      nil,
		RepoPath:     "/home/user/project",
		TotalPrompts: 0,
		TotalTokens:  0,
		EndedBy:      "",
	}

	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	err = UpdateSessionMetrics(db, session.ID, 1, 250)
	if err != nil {
		t.Fatalf("Failed to update session metrics: %v", err)
	}

	var totalPrompts, totalTokens int
	err = db.QueryRow("SELECT total_prompts, total_tokens FROM sessions WHERE id = ?", session.ID).Scan(&totalPrompts, &totalTokens)
	if err != nil {
		t.Fatalf("Failed to query session: %v", err)
	}

	if totalPrompts != 1 {
		t.Errorf("Expected 1 prompt, got %d", totalPrompts)
	}

	if totalTokens != 250 {
		t.Errorf("Expected 250 tokens, got %d", totalTokens)
	}

	// Update again to verify increment behavior
	err = UpdateSessionMetrics(db, session.ID, 1, 300)
	if err != nil {
		t.Fatalf("Failed to update session metrics again: %v", err)
	}

	err = db.QueryRow("SELECT total_prompts, total_tokens FROM sessions WHERE id = ?", session.ID).Scan(&totalPrompts, &totalTokens)
	if err != nil {
		t.Fatalf("Failed to query session: %v", err)
	}

	if totalPrompts != 2 {
		t.Errorf("Expected 2 prompts, got %d", totalPrompts)
	}

	if totalTokens != 550 {
		t.Errorf("Expected 550 tokens, got %d", totalTokens)
	}
}

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "events-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return db
}
