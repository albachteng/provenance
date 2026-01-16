package main

import (
	"strings"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/storage"
)

// TestBlameCommit tests blaming a specific commit SHA
func TestBlameCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test data: session -> prompt -> change_set
	session := &storage.Session{
		ID:        "session-blame-1",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:             "prompt-blame-1",
		Timestamp:      time.Now(),
		SessionID:      "session-blame-1",
		Agent:          "claude-code",
		PromptText:     "Implement user authentication",
		RepoPath:       "/test/repo",
		Author:         "testuser",
		FilesMentioned: []string{"auth.go", "auth_test.go"},
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	changeSet := &storage.ChangeSet{
		ID:                "cs-blame-1",
		PromptID:          "prompt-blame-1",
		SessionID:         "session-blame-1",
		Timestamp:         time.Now(),
		FilesChanged:      []string{"auth.go", "auth_test.go"},
		DiffSummary:       "+150 -20",
		CommitIntroduced:  "abc123def456",
		CorrelationMethod: "git_hook",
		Confidence:        0.95,
	}
	if err := storage.CreateChangeSet(db, changeSet); err != nil {
		t.Fatalf("Failed to create change set: %v", err)
	}

	// Run blame command
	output, err := runCLI(t, "blame", "abc123def456")
	if err != nil {
		t.Fatalf("blame command failed: %v\nOutput: %s", err, output)
	}

	// Should show the linked prompt
	if !strings.Contains(output, "prompt-blame-1") {
		t.Errorf("Expected to find prompt ID in output, got: %s", output)
	}

	if !strings.Contains(output, "Implement user authentication") {
		t.Errorf("Expected to find prompt text in output, got: %s", output)
	}

	if !strings.Contains(output, "0.95") || !strings.Contains(output, "95%") {
		t.Errorf("Expected to find confidence score in output, got: %s", output)
	}

	if !strings.Contains(output, "auth.go") {
		t.Errorf("Expected to find changed files in output, got: %s", output)
	}
}

// TestBlameCommitNoMatches tests blaming a commit with no correlations
func TestBlameCommitNoMatches(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	output, err := runCLI(t, "blame", "nonexistent-commit-sha")
	if err != nil {
		t.Fatalf("blame command failed: %v", err)
	}

	// Should indicate no prompts found
	if !strings.Contains(output, "No prompts found") && !strings.Contains(output, "no correlations") {
		t.Errorf("Expected 'no prompts found' message, got: %s", output)
	}
}

// TestBlameCommitMultiplePrompts tests a commit linked to multiple prompts
func TestBlameCommitMultiplePrompts(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test data with 2 prompts for same commit
	session := &storage.Session{
		ID:        "session-multi-1",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt1 := &storage.PromptEvent{
		ID:             "prompt-multi-1",
		Timestamp:      time.Now().Add(-5 * time.Minute),
		SessionID:      "session-multi-1",
		Agent:          "claude-code",
		PromptText:     "Add login functionality",
		RepoPath:       "/test/repo",
		Author:         "testuser",
		FilesMentioned: []string{"login.go"},
	}
	if err := storage.StorePromptEvent(db, prompt1); err != nil {
		t.Fatalf("Failed to store prompt1: %v", err)
	}

	prompt2 := &storage.PromptEvent{
		ID:             "prompt-multi-2",
		Timestamp:      time.Now().Add(-3 * time.Minute),
		SessionID:      "session-multi-1",
		Agent:          "claude-code",
		PromptText:     "Add logout functionality",
		RepoPath:       "/test/repo",
		Author:         "testuser",
		FilesMentioned: []string{"logout.go"},
	}
	if err := storage.StorePromptEvent(db, prompt2); err != nil {
		t.Fatalf("Failed to store prompt2: %v", err)
	}

	changeSet1 := &storage.ChangeSet{
		ID:                "cs-multi-1",
		PromptID:          "prompt-multi-1",
		SessionID:         "session-multi-1",
		Timestamp:         time.Now(),
		FilesChanged:      []string{"login.go"},
		CommitIntroduced:  "shared-commit-abc",
		CorrelationMethod: "git_hook",
		Confidence:        0.90,
	}
	if err := storage.CreateChangeSet(db, changeSet1); err != nil {
		t.Fatalf("Failed to create change set 1: %v", err)
	}

	changeSet2 := &storage.ChangeSet{
		ID:                "cs-multi-2",
		PromptID:          "prompt-multi-2",
		SessionID:         "session-multi-1",
		Timestamp:         time.Now(),
		FilesChanged:      []string{"logout.go"},
		CommitIntroduced:  "shared-commit-abc",
		CorrelationMethod: "git_hook",
		Confidence:        0.85,
	}
	if err := storage.CreateChangeSet(db, changeSet2); err != nil {
		t.Fatalf("Failed to create change set 2: %v", err)
	}

	// Run blame command
	output, err := runCLI(t, "blame", "shared-commit-abc")
	if err != nil {
		t.Fatalf("blame command failed: %v", err)
	}

	// Should show both prompts
	if !strings.Contains(output, "Add login functionality") {
		t.Errorf("Expected to find first prompt in output, got: %s", output)
	}

	if !strings.Contains(output, "Add logout functionality") {
		t.Errorf("Expected to find second prompt in output, got: %s", output)
	}

	// Should be ordered by confidence (highest first)
	loginIdx := strings.Index(output, "Add login")
	logoutIdx := strings.Index(output, "Add logout")
	if loginIdx == -1 || logoutIdx == -1 {
		t.Fatalf("Could not find both prompts in output")
	}
	if loginIdx > logoutIdx {
		t.Errorf("Expected prompts ordered by confidence (highest first), but got login after logout")
	}
}

// TestBlameFile tests blaming a file path
func TestBlameFile(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test data
	session := &storage.Session{
		ID:        "session-file-1",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:             "prompt-file-1",
		Timestamp:      time.Now(),
		SessionID:      "session-file-1",
		Agent:          "claude-code",
		PromptText:     "Refactor authentication module",
		RepoPath:       "/test/repo",
		Author:         "testuser",
		FilesMentioned: []string{"internal/auth/auth.go"},
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	changeSet := &storage.ChangeSet{
		ID:                "cs-file-1",
		PromptID:          "prompt-file-1",
		SessionID:         "session-file-1",
		Timestamp:         time.Now(),
		FilesChanged:      []string{"internal/auth/auth.go", "internal/auth/auth_test.go"},
		CommitIntroduced:  "file-commit-123",
		CorrelationMethod: "git_hook",
		Confidence:        0.88,
	}
	if err := storage.CreateChangeSet(db, changeSet); err != nil {
		t.Fatalf("Failed to create change set: %v", err)
	}

	// Run blame command on file
	output, err := runCLI(t, "blame", "internal/auth/auth.go")
	if err != nil {
		t.Fatalf("blame command failed: %v\nOutput: %s", err, output)
	}

	// Should show the linked prompt
	if !strings.Contains(output, "Refactor authentication module") {
		t.Errorf("Expected to find prompt text in output, got: %s", output)
	}

	if !strings.Contains(output, "file-commit-123") {
		t.Errorf("Expected to find commit SHA in output, got: %s", output)
	}

	if !strings.Contains(output, "0.88") || !strings.Contains(output, "88%") {
		t.Errorf("Expected to find confidence score in output, got: %s", output)
	}
}

// TestBlameFileNoMatches tests blaming a file with no correlations
func TestBlameFileNoMatches(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	output, err := runCLI(t, "blame", "nonexistent/file.go")
	if err != nil {
		t.Fatalf("blame command failed: %v", err)
	}

	// Should indicate no prompts found
	if !strings.Contains(output, "No prompts found") && !strings.Contains(output, "no correlations") {
		t.Errorf("Expected 'no prompts found' message, got: %s", output)
	}
}

// TestBlameFileMultipleCommits tests a file touched by multiple commits
func TestBlameFileMultipleCommits(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test data with same file in multiple commits
	session := &storage.Session{
		ID:        "session-file-multi",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// First prompt/commit
	prompt1 := &storage.PromptEvent{
		ID:             "prompt-file-m1",
		Timestamp:      time.Now().Add(-10 * time.Minute),
		SessionID:      "session-file-multi",
		Agent:          "claude-code",
		PromptText:     "Initial implementation of handler",
		RepoPath:       "/test/repo",
		Author:         "testuser",
		FilesMentioned: []string{"handler.go"},
	}
	if err := storage.StorePromptEvent(db, prompt1); err != nil {
		t.Fatalf("Failed to store prompt1: %v", err)
	}

	changeSet1 := &storage.ChangeSet{
		ID:                "cs-file-m1",
		PromptID:          "prompt-file-m1",
		SessionID:         "session-file-multi",
		Timestamp:         time.Now().Add(-9 * time.Minute),
		FilesChanged:      []string{"handler.go"},
		CommitIntroduced:  "commit-1",
		CorrelationMethod: "git_hook",
		Confidence:        0.92,
	}
	if err := storage.CreateChangeSet(db, changeSet1); err != nil {
		t.Fatalf("Failed to create change set 1: %v", err)
	}

	// Second prompt/commit
	prompt2 := &storage.PromptEvent{
		ID:             "prompt-file-m2",
		Timestamp:      time.Now().Add(-5 * time.Minute),
		SessionID:      "session-file-multi",
		Agent:          "claude-code",
		PromptText:     "Add error handling to handler",
		RepoPath:       "/test/repo",
		Author:         "testuser",
		FilesMentioned: []string{"handler.go"},
	}
	if err := storage.StorePromptEvent(db, prompt2); err != nil {
		t.Fatalf("Failed to store prompt2: %v", err)
	}

	changeSet2 := &storage.ChangeSet{
		ID:                "cs-file-m2",
		PromptID:          "prompt-file-m2",
		SessionID:         "session-file-multi",
		Timestamp:         time.Now().Add(-4 * time.Minute),
		FilesChanged:      []string{"handler.go"},
		CommitIntroduced:  "commit-2",
		CorrelationMethod: "git_hook",
		Confidence:        0.87,
	}
	if err := storage.CreateChangeSet(db, changeSet2); err != nil {
		t.Fatalf("Failed to create change set 2: %v", err)
	}

	// Run blame command on file
	output, err := runCLI(t, "blame", "handler.go")
	if err != nil {
		t.Fatalf("blame command failed: %v", err)
	}

	// Should show both prompts
	if !strings.Contains(output, "Initial implementation") {
		t.Errorf("Expected to find first prompt in output, got: %s", output)
	}

	if !strings.Contains(output, "Add error handling") {
		t.Errorf("Expected to find second prompt in output, got: %s", output)
	}

	// Should show both commits
	if !strings.Contains(output, "commit-1") {
		t.Errorf("Expected to find commit-1 in output, got: %s", output)
	}

	if !strings.Contains(output, "commit-2") {
		t.Errorf("Expected to find commit-2 in output, got: %s", output)
	}
}

// TestBlameNoArgument tests blame with no file/commit specified
func TestBlameNoArgument(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "blame")
	if err == nil {
		t.Error("Expected error when no argument provided")
	}

	// Error message should indicate usage
	if !strings.Contains(output, "Usage") || !strings.Contains(output, "blame") {
		t.Errorf("Expected usage error in output, got: %s", output)
	}
}

// TestBlameShortCommitSHA tests blaming with a short commit SHA
func TestBlameShortCommitSHA(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	// Create test data with full SHA
	session := &storage.Session{
		ID:        "session-short-sha",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:         "prompt-short-sha",
		Timestamp:  time.Now(),
		SessionID:  "session-short-sha",
		Agent:      "claude-code",
		PromptText: "Short SHA test",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	changeSet := &storage.ChangeSet{
		ID:                "cs-short-sha",
		PromptID:          "prompt-short-sha",
		SessionID:         "session-short-sha",
		Timestamp:         time.Now(),
		FilesChanged:      []string{"test.go"},
		CommitIntroduced:  "abc123def456789",
		CorrelationMethod: "git_hook",
		Confidence:        0.90,
	}
	if err := storage.CreateChangeSet(db, changeSet); err != nil {
		t.Fatalf("Failed to create change set: %v", err)
	}

	// Should work with short SHA (prefix match)
	output, err := runCLI(t, "blame", "abc123")
	if err != nil {
		t.Fatalf("blame command failed with short SHA: %v", err)
	}

	if !strings.Contains(output, "Short SHA test") {
		t.Errorf("Expected to find prompt with short SHA, got: %s", output)
	}
}
