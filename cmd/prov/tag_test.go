package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/git"
	"github.com/albachteng/provenance/internal/storage"
)

func TestTagPromptWithCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	prompt := &storage.PromptEvent{
		ID:              "prompt-tag-1",
		Timestamp:       time.Now(),
		Agent:           "claude-code",
		PromptText:      "Refactor the auth module",
		RepoPath:        tmpDir,
		GitBranch:       "main",
		BranchAtCapture: "main",
		Author:          "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	output, err := runCLI(t, "tag", "prompt-tag-1", "--commit", "abc1234deadbeef")
	if err != nil {
		t.Fatalf("tag command failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "prompt-tag-1") {
		t.Errorf("Expected prompt ID in output, got: %s", output)
	}
	if !strings.Contains(output, "abc1234deadbeef") {
		t.Errorf("Expected commit SHA in output, got: %s", output)
	}
}

func TestTagNoPromptID(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "tag")
	if err == nil {
		t.Error("Expected error when no prompt ID provided")
	}
	if !strings.Contains(output, "Usage") {
		t.Errorf("Expected usage message, got: %s", output)
	}
}

func TestTagNoCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	prompt := &storage.PromptEvent{
		ID:              "prompt-tag-nc",
		Timestamp:       time.Now(),
		Agent:           "claude-code",
		PromptText:      "test",
		RepoPath:        tmpDir,
		GitBranch:       "main",
		BranchAtCapture: "main",
		Author:          "user",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	output, err := runCLI(t, "tag", "prompt-tag-nc")
	if err == nil {
		t.Error("Expected error when --commit not provided")
	}
	if !strings.Contains(output, "--commit") {
		t.Errorf("Expected error mentioning --commit, got: %s", output)
	}
}

func TestTagNonexistentPrompt(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "tag", "no-such-prompt", "--commit", "abc123")
	if err == nil {
		t.Error("Expected error for nonexistent prompt")
	}
	if !strings.Contains(output, "not found") && !strings.Contains(output, "Prompt not found") {
		t.Errorf("Expected 'not found' in error output, got: %s", output)
	}
}

func TestTagWithNote(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	prompt := &storage.PromptEvent{
		ID:              "prompt-tag-note",
		Timestamp:       time.Now(),
		Agent:           "claude-code",
		PromptText:      "Add rate limiting",
		RepoPath:        tmpDir,
		GitBranch:       "main",
		BranchAtCapture: "main",
		Author:          "user",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	output, err := runCLI(t, "tag", "prompt-tag-note", "--commit", "aaa111", "--note", "added during review")
	if err != nil {
		t.Fatalf("tag with note failed: %v\nOutput: %s", err, output)
	}
	if !strings.Contains(output, "added during review") {
		t.Errorf("Expected note in output, got: %s", output)
	}
}

func TestUntagPrompt(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	prompt := &storage.PromptEvent{
		ID:              "prompt-untag-1",
		Timestamp:       time.Now(),
		Agent:           "claude-code",
		PromptText:      "Fix bug",
		RepoPath:        tmpDir,
		GitBranch:       "main",
		BranchAtCapture: "main",
		Author:          "user",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	if _, err := runCLI(t, "tag", "prompt-untag-1", "--commit", "bbb222"); err != nil {
		t.Fatalf("tag failed: %v", err)
	}

	output, err := runCLI(t, "untag", "prompt-untag-1", "bbb222")
	if err != nil {
		t.Fatalf("untag failed: %v\nOutput: %s", err, output)
	}
	if !strings.Contains(output, "Untagged") {
		t.Errorf("Expected 'Untagged' in output, got: %s", output)
	}
}

func TestUntagNoPromptID(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "untag")
	if err == nil {
		t.Error("Expected error when no args provided")
	}
	if !strings.Contains(output, "Usage") {
		t.Errorf("Expected usage message, got: %s", output)
	}
}

func TestUntagNoCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "untag", "prompt-id-only")
	if err == nil {
		t.Error("Expected error when commit SHA missing")
	}
	if !strings.Contains(output, "Usage") {
		t.Errorf("Expected usage message, got: %s", output)
	}
}

func TestUntagNonexistentTag(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	output, err := runCLI(t, "untag", "no-prompt", "no-commit")
	if err == nil {
		t.Error("Expected error for nonexistent tag")
	}
	if !strings.Contains(output, "No tag found") {
		t.Errorf("Expected 'No tag found' in error output, got: %s", output)
	}
}

func TestTagAppearsInBlame(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}
	if err := git.InitRepo(repoPath); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	commitTime := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	testFile := filepath.Join(repoPath, "fix.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"fix.go"}); err != nil {
		t.Fatalf("Failed to stage: %v", err)
	}
	commitSHA, err := git.CreateCommitWithTime(repoPath, "Fix critical bug", commitTime)
	if err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	// Prompt captured well outside the commit window but manually tagged
	prompt := &storage.PromptEvent{
		ID:              "prompt-manual-tag",
		Timestamp:       time.Date(2024, 2, 15, 12, 0, 0, 0, time.UTC), // Before the window
		Agent:           "claude-code",
		PromptText:      "Discuss approach to fixing the critical bug",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		Author:          "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	// Manually tag the out-of-window prompt to the commit
	if _, err := runCLI(t, "tag", "prompt-manual-tag", "--commit", commitSHA, "--note", "planning discussion"); err != nil {
		t.Fatalf("tag failed: %v", err)
	}

	originalDir, _ := os.Getwd()
	os.Chdir(repoPath)          //nolint:errcheck
	defer os.Chdir(originalDir) //nolint:errcheck

	output, err := runCLI(t, "blame", commitSHA)
	if err != nil {
		t.Fatalf("blame failed: %v\nOutput: %s", err, output)
	}

	// Should show the manually tagged prompt even though it's outside the time window
	if !strings.Contains(output, "Discuss approach to fixing the critical bug") {
		t.Errorf("Expected manually tagged prompt text in blame output, got: %s", output)
	}
	if !strings.Contains(output, "manually tagged") {
		t.Errorf("Expected 'manually tagged' label in blame output, got: %s", output)
	}
}
