package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCaptureGitState(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state: %v", err)
	}

	if state.RepoPath != repo {
		t.Errorf("Expected repo path %s, got %s", repo, state.RepoPath)
	}

	if state.Head == "" {
		t.Error("Expected HEAD commit hash, got empty string")
	}

	if state.Branch != "main" && state.Branch != "master" {
		t.Errorf("Expected branch 'main' or 'master', got %s", state.Branch)
	}

	if state.IsDirty {
		t.Error("Expected clean repo, got dirty state")
	}

	if len(state.DirtyFiles) != 0 {
		t.Errorf("Expected 0 dirty files, got %d", len(state.DirtyFiles))
	}
}

func TestCaptureGitStateDirtyRepo(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	testFile := filepath.Join(repo, "uncommitted.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state: %v", err)
	}

	if !state.IsDirty {
		t.Error("Expected dirty state, got clean")
	}

	if len(state.DirtyFiles) == 0 {
		t.Error("Expected dirty files, got none")
	}

	found := false
	for _, file := range state.DirtyFiles {
		if file == "uncommitted.txt" || file == "?? uncommitted.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected uncommitted.txt in dirty files, got %v", state.DirtyFiles)
	}
}

func TestCaptureGitStateModifiedFile(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	readmePath := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readmePath, []byte("Modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state: %v", err)
	}

	if !state.IsDirty {
		t.Error("Expected dirty state for modified file")
	}

	found := false
	for _, file := range state.DirtyFiles {
		if file == "README.md" || file == " M README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected README.md in dirty files, got %v", state.DirtyFiles)
	}
}

func TestCaptureGitStateDiffSummary(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	readmePath := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readmePath, []byte("Line 1\nLine 2\nLine 3\n"), 0644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state: %v", err)
	}

	if state.DiffSummary == "" {
		t.Error("Expected diff summary, got empty string")
	}

	// Diff summary should contain insertions/deletions info
	// Format is typically like "+3 -1" or "3 insertions(+), 1 deletion(-)"
	// Just verify it's not empty for now
}

func TestCaptureGitStateRemoteTracking(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state: %v", err)
	}

	// For a local-only repo, remote tracking should be empty
	// We're not testing actual remote tracking in this basic test
	// (would require setting up a remote)
	if state.RemoteTracking != "" {
		t.Logf("Remote tracking: %s", state.RemoteTracking)
	}
}

func TestCaptureGitStateNonRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "non-repo-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = CaptureGitState(tmpDir)
	if err == nil {
		t.Error("Expected error for non-git directory, got nil")
	}
}

func TestCaptureGitStateAheadBehind(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state: %v", err)
	}

	// For a local repo with no remote, ahead/behind should be 0
	if state.Ahead != 0 {
		t.Errorf("Expected ahead=0, got %d", state.Ahead)
	}

	if state.Behind != 0 {
		t.Errorf("Expected behind=0, got %d", state.Behind)
	}
}

// Helper functions

// setupTestRepo creates a temporary git repository for testing
func setupTestRepo(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize git repo
	runGit(t, tmpDir, "init", "-b", "main")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "user.name", "Test User")

	// Create initial commit
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	runGit(t, tmpDir, "add", "README.md")
	runGit(t, tmpDir, "commit", "-m", "Initial commit")

	return tmpDir
}

// runGit runs a git command in the specified directory
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Git command failed: %s\nOutput: %s", err, output)
	}

	return string(output)
}
