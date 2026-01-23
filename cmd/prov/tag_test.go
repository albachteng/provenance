package main

import (
	"strings"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/storage"
)

// TestTagPromptWithCommit tests manually tagging a prompt to a commit
func TestTagPromptWithCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	session := &storage.Session{
		ID:        "session-tag-1",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:         "prompt-tag-1",
		Timestamp:  time.Now(),
		SessionID:  "session-tag-1",
		Agent:      "claude-code",
		PromptText: "Implement feature X",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	output, err := runCLI(t, "tag", "prompt-tag-1", "--commit", "abc123def456")
	if err != nil {
		t.Fatalf("tag command failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Tagged") || !strings.Contains(output, "prompt-tag-1") {
		t.Errorf("Expected confirmation message, got: %s", output)
	}

	changeSets, err := storage.GetChangeSetsForCommit(db, "abc123def456")
	if err != nil {
		t.Fatalf("Failed to query change sets: %v", err)
	}

	if len(changeSets) != 1 {
		t.Fatalf("Expected 1 change set, got %d", len(changeSets))
	}

	cs := changeSets[0]
	if cs.PromptID != "prompt-tag-1" {
		t.Errorf("Expected prompt ID 'prompt-tag-1', got '%s'", cs.PromptID)
	}

	if cs.CommitIntroduced != "abc123def456" {
		t.Errorf("Expected commit 'abc123def456', got '%s'", cs.CommitIntroduced)
	}

	if cs.CorrelationMethod != "manual" {
		t.Errorf("Expected correlation method 'manual', got '%s'", cs.CorrelationMethod)
	}

	if cs.Confidence != 1.0 {
		t.Errorf("Expected confidence 1.0, got %f", cs.Confidence)
	}
}

// TestTagPromptWithFile tests manually tagging a prompt to a file
func TestTagPromptWithFile(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	session := &storage.Session{
		ID:        "session-tag-file",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:         "prompt-tag-file",
		Timestamp:  time.Now(),
		SessionID:  "session-tag-file",
		Agent:      "claude-code",
		PromptText: "Refactor auth.go",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	output, err := runCLI(t, "tag", "prompt-tag-file", "--file", "internal/auth.go")
	if err != nil {
		t.Fatalf("tag command failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Tagged") || !strings.Contains(output, "prompt-tag-file") {
		t.Errorf("Expected confirmation message, got: %s", output)
	}

	changeSets, err := storage.GetChangeSetsForPrompt(db, "prompt-tag-file")
	if err != nil {
		t.Fatalf("Failed to query change sets: %v", err)
	}

	if len(changeSets) != 1 {
		t.Fatalf("Expected 1 change set, got %d", len(changeSets))
	}

	cs := changeSets[0]
	if cs.PromptID != "prompt-tag-file" {
		t.Errorf("Expected prompt ID 'prompt-tag-file', got '%s'", cs.PromptID)
	}

	if len(cs.FilesChanged) != 1 || cs.FilesChanged[0] != "internal/auth.go" {
		t.Errorf("Expected files ['internal/auth.go'], got %v", cs.FilesChanged)
	}

	if cs.CorrelationMethod != "manual" {
		t.Errorf("Expected correlation method 'manual', got '%s'", cs.CorrelationMethod)
	}

	if cs.Confidence != 1.0 {
		t.Errorf("Expected confidence 1.0, got %f", cs.Confidence)
	}
}

// TestTagNoPromptID tests error when no prompt ID provided
func TestTagNoPromptID(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "tag", "--commit", "abc123")
	if err == nil {
		t.Error("Expected error when no prompt ID provided")
	}

	if !strings.Contains(output, "Usage") || !strings.Contains(output, "prompt-id") {
		t.Errorf("Expected usage error mentioning prompt-id, got: %s", output)
	}
}

// TestTagNoTarget tests error when neither commit nor file specified
func TestTagNoTarget(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "tag", "prompt-123")
	if err == nil {
		t.Error("Expected error when no --commit or --file provided")
	}

	if !strings.Contains(output, "commit") || !strings.Contains(output, "file") {
		t.Errorf("Expected error about commit/file, got: %s", output)
	}
}

// TestTagBothTargets tests error when both commit and file specified
func TestTagBothTargets(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "tag", "prompt-123", "--commit", "abc123", "--file", "test.go")
	if err == nil {
		t.Error("Expected error when both --commit and --file provided")
	}

	if !strings.Contains(output, "both") || !strings.Contains(output, "mutually exclusive") {
		t.Errorf("Expected mutually exclusive error, got: %s", output)
	}
}

// TestTagNonexistentPrompt tests error when prompt doesn't exist
func TestTagNonexistentPrompt(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "tag", "nonexistent-prompt", "--commit", "abc123")
	if err == nil {
		t.Error("Expected error when prompt doesn't exist")
	}

	if !strings.Contains(output, "not found") || !strings.Contains(output, "nonexistent-prompt") {
		t.Errorf("Expected 'not found' error, got: %s", output)
	}
}

// TestTagAppearsInBlame tests that manually tagged prompts show up in blame
func TestTagAppearsInBlame(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	session := &storage.Session{
		ID:        "session-blame-tag",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:         "prompt-blame-tag",
		Timestamp:  time.Now(),
		SessionID:  "session-blame-tag",
		Agent:      "claude-code",
		PromptText: "Manual tag test",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	_, err := runCLI(t, "tag", "prompt-blame-tag", "--commit", "manual-commit-123")
	if err != nil {
		t.Fatalf("tag command failed: %v", err)
	}

	output, err := runCLI(t, "blame", "manual-commit-123")
	if err != nil {
		t.Fatalf("blame command failed: %v", err)
	}

	if !strings.Contains(output, "Manual tag test") {
		t.Errorf("Expected manually tagged prompt in blame output, got: %s", output)
	}

	if !strings.Contains(output, "1.00") || !strings.Contains(output, "100%") {
		t.Errorf("Expected confidence 1.00/100%% in blame output, got: %s", output)
	}

	if !strings.Contains(output, "manual") {
		t.Errorf("Expected 'manual' indicator in blame output, got: %s", output)
	}
}

// TestUntagPrompt tests removing a manual tag
func TestUntagPrompt(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	session := &storage.Session{
		ID:        "session-untag",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:         "prompt-untag",
		Timestamp:  time.Now(),
		SessionID:  "session-untag",
		Agent:      "claude-code",
		PromptText: "Untag test",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	_, err := runCLI(t, "tag", "prompt-untag", "--commit", "untag-commit-123")
	if err != nil {
		t.Fatalf("tag command failed: %v", err)
	}

	changeSets, err := storage.GetChangeSetsForCommit(db, "untag-commit-123")
	if err != nil || len(changeSets) != 1 {
		t.Fatalf("Failed to verify tag was created")
	}

	output, err := runCLI(t, "untag", "prompt-untag", "untag-commit-123")
	if err != nil {
		t.Fatalf("untag command failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Untagged") || !strings.Contains(output, "prompt-untag") {
		t.Errorf("Expected confirmation message, got: %s", output)
	}

	changeSets, err = storage.GetChangeSetsForCommit(db, "untag-commit-123")
	if err != nil {
		t.Fatalf("Failed to query change sets: %v", err)
	}

	if len(changeSets) != 0 {
		t.Errorf("Expected 0 change sets after untag, got %d", len(changeSets))
	}
}

// TestUntagNoPromptID tests error when no prompt ID provided to untag
func TestUntagNoPromptID(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "untag")
	if err == nil {
		t.Error("Expected error when no prompt ID provided")
	}

	if !strings.Contains(output, "Usage") {
		t.Errorf("Expected usage error, got: %s", output)
	}
}

// TestUntagNoCommit tests error when no commit provided to untag
func TestUntagNoCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "untag", "prompt-123")
	if err == nil {
		t.Error("Expected error when no commit provided")
	}

	if !strings.Contains(output, "Usage") || !strings.Contains(output, "commit") {
		t.Errorf("Expected usage error mentioning commit, got: %s", output)
	}
}

// TestUntagNonexistentTag tests error when tag doesn't exist
func TestUntagNonexistentTag(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	session := &storage.Session{
		ID:        "session-no-tag",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:         "prompt-no-tag",
		Timestamp:  time.Now(),
		SessionID:  "session-no-tag",
		Agent:      "claude-code",
		PromptText: "No tag test",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	output, err := runCLI(t, "untag", "prompt-no-tag", "nonexistent-commit")
	if err == nil {
		t.Error("Expected error when tag doesn't exist")
	}

	if !strings.Contains(output, "not found") || !strings.Contains(output, "No manual tag") {
		t.Errorf("Expected 'not found' error, got: %s", output)
	}
}

// TestUntagAutoCorrelation tests that untag only removes manual tags, not auto-correlations
func TestUntagAutoCorrelation(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	session := &storage.Session{
		ID:        "session-auto",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	prompt := &storage.PromptEvent{
		ID:         "prompt-auto",
		Timestamp:  time.Now(),
		SessionID:  "session-auto",
		Agent:      "claude-code",
		PromptText: "Auto correlation test",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	changeSet := &storage.ChangeSet{
		ID:                "cs-auto",
		PromptID:          "prompt-auto",
		SessionID:         "session-auto",
		Timestamp:         time.Now(),
		FilesChanged:      []string{"test.go"},
		CommitIntroduced:  "auto-commit-123",
		CorrelationMethod: "git_hook",
		Confidence:        0.85,
	}
	if err := storage.CreateChangeSet(db, changeSet); err != nil {
		t.Fatalf("Failed to create change set: %v", err)
	}

	output, err := runCLI(t, "untag", "prompt-auto", "auto-commit-123")
	if err == nil {
		t.Error("Expected error when trying to untag auto-correlation")
	}

	if !strings.Contains(output, "not a manual tag") || !strings.Contains(output, "git_hook") {
		t.Errorf("Expected 'not a manual tag' error, got: %s", output)
	}

	changeSets, err := storage.GetChangeSetsForCommit(db, "auto-commit-123")
	if err != nil {
		t.Fatalf("Failed to query change sets: %v", err)
	}

	if len(changeSets) != 1 {
		t.Errorf("Expected auto-correlation to still exist, got %d change sets", len(changeSets))
	}
}
