package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestManagerUpdateGitState tests that manager updates git state on check
func TestManagerUpdateGitState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repoPath := setupGitRepo(t)

	strategy := NewGitEventStrategy(true, true, 4*time.Hour)
	manager := NewManager(db, strategy)

	_, err := manager.StartSession(repoPath)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	manager.UpdateGitState()

	session := manager.GetCurrentSession()
	if session == nil {
		t.Fatal("Expected active session")
	}

	if session.GitCommit == "" {
		t.Error("Expected GitCommit to be captured")
	}

	if session.GitBranch == "" {
		t.Error("Expected GitBranch to be captured")
	}

	initialCommit := session.GitCommit
	initialBranch := session.GitBranch

	createCommit(t, repoPath, "test.txt", "test content")

	manager.UpdateGitState()

	session = manager.GetCurrentSession()

	if session.LastGitCommit != initialCommit {
		t.Errorf("LastGitCommit = %s, want %s", session.LastGitCommit, initialCommit)
	}

	if session.GitCommit == initialCommit {
		t.Error("GitCommit should be updated after new commit")
	}

	if session.GitBranch != initialBranch {
		t.Errorf("GitBranch = %s, want %s", session.GitBranch, initialBranch)
	}

	if session.LastGitBranch != initialBranch {
		t.Errorf("LastGitBranch = %s, want %s", session.LastGitBranch, initialBranch)
	}

	if manager.GetCurrentSession() == nil {
		t.Error("Session should still be active after first commit check")
	}

	ended := manager.CheckSessionBoundaries()
	if !ended {
		t.Error("Expected session to end after commit detected")
	}

	if manager.GetCurrentSession() != nil {
		t.Error("Expected no active session after commit-triggered end")
	}

	_, err = manager.StartSession(repoPath)
	if err != nil {
		t.Fatalf("Failed to start new session: %v", err)
	}
}

// TestManagerBranchSwitchDetection tests that manager detects branch switches
func TestManagerBranchSwitchDetection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repoPath := setupGitRepo(t)

	strategy := NewGitEventStrategy(false, true, 4*time.Hour)
	manager := NewManager(db, strategy)

	_, err := manager.StartSession(repoPath)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	manager.UpdateGitState()

	session := manager.GetCurrentSession()
	initialBranch := session.GitBranch

	switchBranch(t, repoPath, "feature-branch")

	manager.UpdateGitState()

	session = manager.GetCurrentSession()

	if session.LastGitBranch != initialBranch {
		t.Errorf("LastGitBranch = %s, want %s", session.LastGitBranch, initialBranch)
	}

	if session.GitBranch == initialBranch {
		t.Error("GitBranch should be updated after branch switch")
	}

	if session.GitBranch != "feature-branch" {
		t.Errorf("GitBranch = %s, want feature-branch", session.GitBranch)
	}

	ended := manager.CheckSessionBoundaries()
	if !ended {
		t.Error("Expected session to end after branch switch")
	}

	if manager.GetCurrentSession() != nil {
		t.Error("Expected no active session after branch-triggered end")
	}
}

// TestManagerNoGitStateUpdateWhenNoSession tests that UpdateGitState handles no active session gracefully
func TestManagerNoGitStateUpdateWhenNoSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewGitEventStrategy(true, true, 4*time.Hour)
	manager := NewManager(db, strategy)

	// Should not panic when no session is active
	manager.UpdateGitState()

	if manager.GetCurrentSession() != nil {
		t.Error("Expected no active session")
	}
}

// TestManagerGitStateInNonGitDirectory tests handling of non-git directories
func TestManagerGitStateInNonGitDirectory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tmpDir := t.TempDir()

	strategy := NewGitEventStrategy(true, true, 4*time.Hour)
	manager := NewManager(db, strategy)

	_, err := manager.StartSession(tmpDir)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	manager.UpdateGitState()

	session := manager.GetCurrentSession()
	if session == nil {
		t.Fatal("Expected active session")
	}

	if session.GitCommit != "" {
		t.Errorf("Expected empty GitCommit in non-git repo, got %s", session.GitCommit)
	}

	if session.GitBranch != "" {
		t.Errorf("Expected empty GitBranch in non-git repo, got %s", session.GitBranch)
	}

	if !session.IsActive() {
		t.Error("Session should remain active even in non-git directory")
	}
}

// setupGitRepo creates a temporary git repository for testing
func setupGitRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	runGitCmd(t, tmpDir, "init")
	runGitCmd(t, tmpDir, "config", "user.email", "test@example.com")
	runGitCmd(t, tmpDir, "config", "user.name", "Test User")

	createCommit(t, tmpDir, "README.md", "# Test Repo")

	return tmpDir
}

// createCommit creates a new commit in the repo
func createCommit(t *testing.T, repoPath, filename, content string) {
	t.Helper()

	filePath := filepath.Join(repoPath, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	runGitCmd(t, repoPath, "add", filename)
	runGitCmd(t, repoPath, "commit", "-m", "Test commit")
}

// switchBranch creates and switches to a new branch
func switchBranch(t *testing.T, repoPath, branchName string) {
	t.Helper()
	runGitCmd(t, repoPath, "checkout", "-b", branchName)
}

// runGitCmd runs a git command in the specified directory
func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Git command failed: %s\nOutput: %s", err, output)
	}
}
