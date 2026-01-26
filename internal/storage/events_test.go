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
	defer db.Close() //nolint:errcheck

	event := &PromptEvent{
		ID:           "test-event-1",
		Timestamp:    time.Now(),
		SessionID:    "", // V2: nullable, no FK constraint
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
		PromptType:      "chat",
		ToolsInvoked:    []string{"read_file", "write_file"},
		FilesMentioned:  []string{"src/auth.go"},
		BranchAtCapture: "main", // V2: branch at prompt submission
		PreBranchSwitch: false,  // V2: not before branch switch
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
	defer db.Close() //nolint:errcheck

	originalEvent := &PromptEvent{
		ID:              "test-event-2",
		Timestamp:       time.Now().Truncate(time.Second),
		SessionID:       "",
		Agent:           "cursor",
		ModelVersion:    "gpt-4",
		PromptText:      "Fix the bug in login",
		ResponseText:    "The issue is in the validation...",
		TokensIn:        30,
		TokensOut:       150,
		LatencyMs:       800,
		RepoPath:        "/home/user/app",
		GitCommit:       "def456",
		GitBranch:       "feature/login",
		GitDirty:        true,
		DirtyFiles:      []string{" M src/login.go", "?? test.txt"},
		Author:          "devuser",
		IDE:             "neovim",
		ActiveFile:      "src/login.go",
		WorkspaceFiles:  []string{"src/login.go", "src/auth.go"},
		PromptType:      "edit",
		ToolsInvoked:    []string{"edit_file"},
		FilesMentioned:  []string{"src/login.go"},
		BranchAtCapture: "feature/login",
		PreBranchSwitch: false,
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

	if retrievedEvent.BranchAtCapture != originalEvent.BranchAtCapture {
		t.Errorf("Expected branch_at_capture %s, got %s", originalEvent.BranchAtCapture, retrievedEvent.BranchAtCapture)
	}

	if retrievedEvent.PreBranchSwitch != originalEvent.PreBranchSwitch {
		t.Errorf("Expected pre_branch_switch %v, got %v", originalEvent.PreBranchSwitch, retrievedEvent.PreBranchSwitch)
	}
}

func TestGetPromptEventNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	_, err := GetPromptEvent(db, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent event, got nil")
	}

	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestStorePromptEventWithJSONFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	event := &PromptEvent{
		ID:         "test-event-json",
		Timestamp:  time.Now(),
		SessionID:  "",
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
		BranchAtCapture: "feature/refactor",
		PreBranchSwitch: false,
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
