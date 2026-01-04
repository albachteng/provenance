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

func TestListSessions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create multiple sessions with different states
	now := time.Now().Truncate(time.Second)

	// Active session 1
	session1 := &Session{
		ID:           "session-active-1",
		StartTime:    now.Add(-2 * time.Hour),
		EndTime:      nil,
		RepoPath:     "/home/user/project1",
		TotalPrompts: 5,
		TotalTokens:  1000,
		EndedBy:      "",
	}
	if err := CreateSession(db, session1); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}

	// Active session 2
	session2 := &Session{
		ID:           "session-active-2",
		StartTime:    now.Add(-1 * time.Hour),
		EndTime:      nil,
		RepoPath:     "/home/user/project2",
		TotalPrompts: 3,
		TotalTokens:  500,
		EndedBy:      "",
	}
	if err := CreateSession(db, session2); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}

	// Ended session
	endTime := now.Add(-30 * time.Minute)
	session3 := &Session{
		ID:           "session-ended-1",
		StartTime:    now.Add(-3 * time.Hour),
		EndTime:      &endTime,
		RepoPath:     "/home/user/project3",
		TotalPrompts: 10,
		TotalTokens:  2000,
		EndedBy:      "timeout",
	}
	if err := CreateSession(db, session3); err != nil {
		t.Fatalf("Failed to create session3: %v", err)
	}

	// Test listing all sessions
	t.Run("list all sessions", func(t *testing.T) {
		sessions, err := ListSessions(db, false)
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}

		if len(sessions) != 3 {
			t.Errorf("Expected 3 sessions, got %d", len(sessions))
		}

		// Sessions should be ordered by start_time DESC (newest first)
		if sessions[0].ID != "session-active-2" {
			t.Errorf("Expected first session to be session-active-2, got %s", sessions[0].ID)
		}
	})

	// Test listing only active sessions
	t.Run("list active sessions only", func(t *testing.T) {
		sessions, err := ListSessions(db, true)
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}

		if len(sessions) != 2 {
			t.Errorf("Expected 2 active sessions, got %d", len(sessions))
		}

		// Verify all returned sessions are active
		for _, s := range sessions {
			if s.EndTime != nil {
				t.Errorf("Expected active session, but session %s has end_time", s.ID)
			}
		}
	})

	// Test empty result
	t.Run("empty database", func(t *testing.T) {
		emptyDB := setupTestDB(t)
		defer emptyDB.Close()

		sessions, err := ListSessions(emptyDB, false)
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}

		if len(sessions) != 0 {
			t.Errorf("Expected 0 sessions, got %d", len(sessions))
		}
	})
}

func TestListSessionsOrderAndFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().Truncate(time.Second)

	// Create sessions in non-chronological order
	sessions := []*Session{
		{
			ID:           "session-3",
			StartTime:    now.Add(-3 * time.Hour),
			RepoPath:     "/repo3",
			TotalPrompts: 1,
			TotalTokens:  100,
		},
		{
			ID:           "session-1",
			StartTime:    now.Add(-1 * time.Hour),
			RepoPath:     "/repo1",
			TotalPrompts: 3,
			TotalTokens:  300,
		},
		{
			ID:           "session-2",
			StartTime:    now.Add(-2 * time.Hour),
			RepoPath:     "/repo2",
			TotalPrompts: 2,
			TotalTokens:  200,
		},
	}

	for _, s := range sessions {
		if err := CreateSession(db, s); err != nil {
			t.Fatalf("Failed to create session %s: %v", s.ID, err)
		}
	}

	// List all sessions
	result, err := ListSessions(db, false)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	// Verify ordering (newest first)
	expectedOrder := []string{"session-1", "session-2", "session-3"}
	for i, expected := range expectedOrder {
		if result[i].ID != expected {
			t.Errorf("Expected session %d to be %s, got %s", i, expected, result[i].ID)
		}
	}

	// Verify all fields are populated correctly
	// Map by ID for easier verification
	sessionMap := make(map[string]*Session)
	for _, s := range sessions {
		sessionMap[s.ID] = s
	}

	for _, s := range result {
		original := sessionMap[s.ID]
		if original == nil {
			t.Errorf("Session %s not found in original list", s.ID)
			continue
		}

		if s.RepoPath != original.RepoPath {
			t.Errorf("Session %s: expected repo %s, got %s", s.ID, original.RepoPath, s.RepoPath)
		}
		if s.TotalPrompts != original.TotalPrompts {
			t.Errorf("Session %s: expected %d prompts, got %d", s.ID, original.TotalPrompts, s.TotalPrompts)
		}
		if s.TotalTokens != original.TotalTokens {
			t.Errorf("Session %s: expected %d tokens, got %d", s.ID, original.TotalTokens, s.TotalTokens)
		}
	}
}

// TestGetRepoStats tests getting aggregate statistics for a repository
func TestGetRepoStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repoPath := "/home/user/test-project"
	now := time.Now().Truncate(time.Second)

	// Create two sessions
	session1 := &Session{
		ID:           "stats-session-1",
		StartTime:    now.Add(-2 * time.Hour),
		EndTime:      nil,
		RepoPath:     repoPath,
		TotalPrompts: 0,
		TotalTokens:  0,
	}
	session2 := &Session{
		ID:           "stats-session-2",
		StartTime:    now.Add(-1 * time.Hour),
		EndTime:      nil,
		RepoPath:     repoPath,
		TotalPrompts: 0,
		TotalTokens:  0,
	}

	if err := CreateSession(db, session1); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	if err := CreateSession(db, session2); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}

	// Create prompt events across both sessions
	events := []*PromptEvent{
		{
			ID:             "stats-event-1",
			Timestamp:      now.Add(-90 * time.Minute),
			SessionID:      "stats-session-1",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Fix bug in auth",
			ResponseText:   "Here's the fix...",
			TokensIn:       100,
			TokensOut:      300,
			LatencyMs:      1000,
			RepoPath:       repoPath,
			GitCommit:      "abc123",
			GitBranch:      "main",
			GitDirty:       true,
			DirtyFiles:     []string{"src/auth.go"},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "src/auth.go",
			WorkspaceFiles: []string{"src/auth.go", "src/main.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"read_file", "write_file"},
			FilesMentioned: []string{"src/auth.go"},
		},
		{
			ID:             "stats-event-2",
			Timestamp:      now.Add(-60 * time.Minute),
			SessionID:      "stats-session-1",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Add tests",
			ResponseText:   "I'll add tests...",
			TokensIn:       150,
			TokensOut:      400,
			LatencyMs:      1200,
			RepoPath:       repoPath,
			GitCommit:      "abc123",
			GitBranch:      "main",
			GitDirty:       true,
			DirtyFiles:     []string{"src/auth_test.go"},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "src/auth_test.go",
			WorkspaceFiles: []string{"src/auth.go", "src/auth_test.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"read_file", "write_file", "bash"},
			FilesMentioned: []string{"src/auth.go", "src/auth_test.go"},
		},
		{
			ID:             "stats-event-3",
			Timestamp:      now.Add(-30 * time.Minute),
			SessionID:      "stats-session-2",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Refactor database code",
			ResponseText:   "Let's refactor...",
			TokensIn:       200,
			TokensOut:      500,
			LatencyMs:      1500,
			RepoPath:       repoPath,
			GitCommit:      "def456",
			GitBranch:      "feature/db",
			GitDirty:       false,
			DirtyFiles:     []string{},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "src/db.go",
			WorkspaceFiles: []string{"src/db.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"read_file", "edit"},
			FilesMentioned: []string{"src/db.go"},
		},
	}

	for _, event := range events {
		if err := StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event %s: %v", event.ID, err)
		}
	}

	// Test getting repo stats
	stats, err := GetRepoStats(db, repoPath)
	if err != nil {
		t.Fatalf("GetRepoStats failed: %v", err)
	}

	// Verify aggregate counts
	if stats.TotalPrompts != 3 {
		t.Errorf("Expected 3 total prompts, got %d", stats.TotalPrompts)
	}

	expectedTokensIn := 100 + 150 + 200
	if stats.TotalTokensIn != expectedTokensIn {
		t.Errorf("Expected %d tokens in, got %d", expectedTokensIn, stats.TotalTokensIn)
	}

	expectedTokensOut := 300 + 400 + 500
	if stats.TotalTokensOut != expectedTokensOut {
		t.Errorf("Expected %d tokens out, got %d", expectedTokensOut, stats.TotalTokensOut)
	}

	if stats.SessionCount != 2 {
		t.Errorf("Expected 2 sessions, got %d", stats.SessionCount)
	}

	// Verify file mention counts
	expectedFileMentions := map[string]int{
		"src/auth.go":      2,
		"src/auth_test.go": 1,
		"src/db.go":        1,
	}

	for file, expectedCount := range expectedFileMentions {
		if count, ok := stats.FilesMentioned[file]; !ok {
			t.Errorf("Expected file %s to be mentioned, but not found", file)
		} else if count != expectedCount {
			t.Errorf("File %s: expected %d mentions, got %d", file, expectedCount, count)
		}
	}

	// Verify tool usage counts
	expectedToolUsage := map[string]int{
		"read_file":  3,
		"write_file": 2,
		"bash":       1,
		"edit":       1,
	}

	for tool, expectedCount := range expectedToolUsage {
		if count, ok := stats.ToolsInvoked[tool]; !ok {
			t.Errorf("Expected tool %s to be invoked, but not found", tool)
		} else if count != expectedCount {
			t.Errorf("Tool %s: expected %d invocations, got %d", tool, expectedCount, count)
		}
	}
}

// TestGetSessionStats tests getting statistics for a specific session
func TestGetSessionStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repoPath := "/home/user/test-project"
	now := time.Now().Truncate(time.Second)

	// Create session
	session := &Session{
		ID:           "stats-session-specific",
		StartTime:    now.Add(-1 * time.Hour),
		EndTime:      nil,
		RepoPath:     repoPath,
		TotalPrompts: 0,
		TotalTokens:  0,
	}

	if err := CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create events for this session
	events := []*PromptEvent{
		{
			ID:             "session-event-1",
			Timestamp:      now.Add(-50 * time.Minute),
			SessionID:      "stats-session-specific",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Implement feature X",
			ResponseText:   "Here's how...",
			TokensIn:       120,
			TokensOut:      350,
			LatencyMs:      1100,
			RepoPath:       repoPath,
			GitCommit:      "abc123",
			GitBranch:      "main",
			GitDirty:       false,
			DirtyFiles:     []string{},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "src/feature.go",
			WorkspaceFiles: []string{"src/feature.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"write_file"},
			FilesMentioned: []string{"src/feature.go"},
		},
		{
			ID:             "session-event-2",
			Timestamp:      now.Add(-30 * time.Minute),
			SessionID:      "stats-session-specific",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Add error handling",
			ResponseText:   "Let me add that...",
			TokensIn:       80,
			TokensOut:      250,
			LatencyMs:      900,
			RepoPath:       repoPath,
			GitCommit:      "abc123",
			GitBranch:      "main",
			GitDirty:       true,
			DirtyFiles:     []string{"src/feature.go"},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "src/feature.go",
			WorkspaceFiles: []string{"src/feature.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"edit", "read_file"},
			FilesMentioned: []string{"src/feature.go"},
		},
	}

	for _, event := range events {
		if err := StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event %s: %v", event.ID, err)
		}
	}

	// Test getting session-specific stats
	stats, err := GetSessionStats(db, "stats-session-specific")
	if err != nil {
		t.Fatalf("GetSessionStats failed: %v", err)
	}

	if stats.SessionID != "stats-session-specific" {
		t.Errorf("Expected session ID 'stats-session-specific', got %s", stats.SessionID)
	}

	if stats.TotalPrompts != 2 {
		t.Errorf("Expected 2 prompts, got %d", stats.TotalPrompts)
	}

	expectedTokensIn := 120 + 80
	if stats.TotalTokensIn != expectedTokensIn {
		t.Errorf("Expected %d tokens in, got %d", expectedTokensIn, stats.TotalTokensIn)
	}

	expectedTokensOut := 350 + 250
	if stats.TotalTokensOut != expectedTokensOut {
		t.Errorf("Expected %d tokens out, got %d", expectedTokensOut, stats.TotalTokensOut)
	}

	// Verify session timing
	if !stats.StartTime.Equal(session.StartTime) {
		t.Errorf("Expected start time %v, got %v", session.StartTime, stats.StartTime)
	}

	if stats.EndTime != nil {
		t.Errorf("Expected nil end time for active session, got %v", stats.EndTime)
	}

	// Verify file mentions
	if count, ok := stats.FilesMentioned["src/feature.go"]; !ok {
		t.Error("Expected src/feature.go to be mentioned")
	} else if count != 2 {
		t.Errorf("Expected src/feature.go to be mentioned 2 times, got %d", count)
	}

	// Verify tool usage
	expectedTools := map[string]int{
		"write_file": 1,
		"edit":       1,
		"read_file":  1,
	}

	for tool, expectedCount := range expectedTools {
		if count, ok := stats.ToolsInvoked[tool]; !ok {
			t.Errorf("Expected tool %s to be invoked", tool)
		} else if count != expectedCount {
			t.Errorf("Tool %s: expected %d invocations, got %d", tool, expectedCount, count)
		}
	}
}

// TestGetTimeframeStats tests getting statistics for a specific time window
func TestGetTimeframeStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repoPath := "/home/user/test-project"
	now := time.Now().Truncate(time.Second)

	// Create session
	session := &Session{
		ID:           "stats-timeframe-session",
		StartTime:    now.Add(-10 * 24 * time.Hour), // 10 days ago
		EndTime:      nil,
		RepoPath:     repoPath,
		TotalPrompts: 0,
		TotalTokens:  0,
	}

	if err := CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create events at different times
	events := []*PromptEvent{
		{
			// 9 days ago - should be included in "last 7 days"? No
			ID:             "timeframe-event-1",
			Timestamp:      now.Add(-9 * 24 * time.Hour),
			SessionID:      "stats-timeframe-session",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Old event",
			ResponseText:   "Response",
			TokensIn:       50,
			TokensOut:      100,
			LatencyMs:      500,
			RepoPath:       repoPath,
			GitCommit:      "old123",
			GitBranch:      "main",
			GitDirty:       false,
			DirtyFiles:     []string{},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "old.go",
			WorkspaceFiles: []string{"old.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"read_file"},
			FilesMentioned: []string{"old.go"},
		},
		{
			// 3 days ago - should be included
			ID:             "timeframe-event-2",
			Timestamp:      now.Add(-3 * 24 * time.Hour),
			SessionID:      "stats-timeframe-session",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Recent event 1",
			ResponseText:   "Response",
			TokensIn:       100,
			TokensOut:      200,
			LatencyMs:      800,
			RepoPath:       repoPath,
			GitCommit:      "recent123",
			GitBranch:      "main",
			GitDirty:       false,
			DirtyFiles:     []string{},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "recent.go",
			WorkspaceFiles: []string{"recent.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"write_file"},
			FilesMentioned: []string{"recent.go"},
		},
		{
			// 1 day ago - should be included
			ID:             "timeframe-event-3",
			Timestamp:      now.Add(-1 * 24 * time.Hour),
			SessionID:      "stats-timeframe-session",
			Agent:          "claude-code",
			ModelVersion:   "sonnet-4.5",
			PromptText:     "Recent event 2",
			ResponseText:   "Response",
			TokensIn:       150,
			TokensOut:      300,
			LatencyMs:      1000,
			RepoPath:       repoPath,
			GitCommit:      "recent456",
			GitBranch:      "main",
			GitDirty:       false,
			DirtyFiles:     []string{},
			Author:         "testuser",
			IDE:            "vscode",
			ActiveFile:     "recent.go",
			WorkspaceFiles: []string{"recent.go"},
			PromptType:     "chat",
			ToolsInvoked:   []string{"edit"},
			FilesMentioned: []string{"recent.go"},
		},
	}

	for _, event := range events {
		if err := StorePromptEvent(db, event); err != nil {
			t.Fatalf("Failed to store event %s: %v", event.ID, err)
		}
	}

	// Test getting stats for last 7 days
	since := now.Add(-7 * 24 * time.Hour)
	stats, err := GetTimeframeStats(db, repoPath, since)
	if err != nil {
		t.Fatalf("GetTimeframeStats failed: %v", err)
	}

	// Should only include events 2 and 3 (3 days ago and 1 day ago)
	if stats.TotalPrompts != 2 {
		t.Errorf("Expected 2 prompts in last 7 days, got %d", stats.TotalPrompts)
	}

	expectedTokensIn := 100 + 150
	if stats.TotalTokensIn != expectedTokensIn {
		t.Errorf("Expected %d tokens in, got %d", expectedTokensIn, stats.TotalTokensIn)
	}

	expectedTokensOut := 200 + 300
	if stats.TotalTokensOut != expectedTokensOut {
		t.Errorf("Expected %d tokens out, got %d", expectedTokensOut, stats.TotalTokensOut)
	}

	// Verify only recent file is counted
	if count, ok := stats.FilesMentioned["recent.go"]; !ok {
		t.Error("Expected recent.go to be mentioned")
	} else if count != 2 {
		t.Errorf("Expected recent.go to be mentioned 2 times, got %d", count)
	}

	if _, ok := stats.FilesMentioned["old.go"]; ok {
		t.Error("Did not expect old.go to be mentioned in recent stats")
	}

	// Verify only recent tools are counted
	expectedTools := map[string]int{
		"write_file": 1,
		"edit":       1,
	}

	for tool, expectedCount := range expectedTools {
		if count, ok := stats.ToolsInvoked[tool]; !ok {
			t.Errorf("Expected tool %s to be invoked", tool)
		} else if count != expectedCount {
			t.Errorf("Tool %s: expected %d invocations, got %d", tool, expectedCount, count)
		}
	}

	if _, ok := stats.ToolsInvoked["read_file"]; ok {
		t.Error("Did not expect read_file to be counted in recent stats")
	}
}

// TestGetRepoStatsEmptyRepo tests stats for a repo with no events
func TestGetRepoStatsEmptyRepo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	stats, err := GetRepoStats(db, "/nonexistent/repo")
	if err != nil {
		t.Fatalf("GetRepoStats failed: %v", err)
	}

	if stats.TotalPrompts != 0 {
		t.Errorf("Expected 0 prompts, got %d", stats.TotalPrompts)
	}

	if stats.TotalTokensIn != 0 {
		t.Errorf("Expected 0 tokens in, got %d", stats.TotalTokensIn)
	}

	if stats.TotalTokensOut != 0 {
		t.Errorf("Expected 0 tokens out, got %d", stats.TotalTokensOut)
	}

	if stats.SessionCount != 0 {
		t.Errorf("Expected 0 sessions, got %d", stats.SessionCount)
	}

	if len(stats.FilesMentioned) != 0 {
		t.Errorf("Expected 0 file mentions, got %d", len(stats.FilesMentioned))
	}

	if len(stats.ToolsInvoked) != 0 {
		t.Errorf("Expected 0 tool invocations, got %d", len(stats.ToolsInvoked))
	}
}

// TestGetSessionStatsNotFound tests getting stats for non-existent session
func TestGetSessionStatsNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := GetSessionStats(db, "nonexistent-session")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
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
