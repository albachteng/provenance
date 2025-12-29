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

func TestCaptureGitStateDetachedHead(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	testFile := filepath.Join(repo, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	runGit(t, repo, "add", "test.txt")
	runGit(t, repo, "commit", "-m", "Second commit")

	firstCommit := runGit(t, repo, "rev-list", "--max-parents=0", "HEAD")
	firstCommit = firstCommit[:len(firstCommit)-1] // trim newline

	runGit(t, repo, "checkout", firstCommit)

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state: %v", err)
	}

	// In detached HEAD state, branch might be empty or "HEAD"
	if state.Branch != "" && state.Branch != "HEAD" {
		t.Logf("Detached HEAD branch name: %s", state.Branch)
	}

	// HEAD should still be populated
	if state.Head == "" {
		t.Error("Expected HEAD commit in detached state")
	}
}

func TestCaptureGitStateMergeConflict(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	runGit(t, repo, "checkout", "-b", "branch-a")
	readmePath := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Branch A\n"), 0644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "Change from branch A")

	runGit(t, repo, "checkout", "main")
	if err := os.WriteFile(readmePath, []byte("# Main Branch\n"), 0644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "Change from main")

	cmd := exec.Command("git", "merge", "branch-a")
	cmd.Dir = repo
	_ = cmd.Run() // Ignore error - merge is expected to fail

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state during conflict: %v", err)
	}

	if !state.IsDirty {
		t.Error("Expected dirty state during merge conflict")
	}

	if len(state.DirtyFiles) == 0 {
		t.Error("Expected dirty files during merge conflict")
	}
}

func TestCaptureGitStateWithSubmodule(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	submoduleRepo := setupTestRepo(t)
	defer os.RemoveAll(submoduleRepo)

	runGit(t, repo, "submodule", "add", submoduleRepo, "submodule")
	runGit(t, repo, "commit", "-m", "Add submodule")

	state, err := CaptureGitState(repo)
	if err != nil {
		t.Fatalf("Failed to capture git state with submodule: %v", err)
	}

	if state.Head == "" {
		t.Error("Expected HEAD to be populated with submodules")
	}

	// Submodule changes should be detectable
	// (we don't modify the submodule here, so should be clean)
	if state.IsDirty {
		t.Logf("Note: Repo with submodule marked as dirty: %v", state.DirtyFiles)
	}
}

func TestCaptureGitStatePermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	gitDir := filepath.Join(repo, ".git")
	if err := os.Chmod(gitDir, 0000); err != nil {
		t.Fatalf("Failed to change permissions: %v", err)
	}
	defer os.Chmod(gitDir, 0755) // Restore for cleanup

	_, err := CaptureGitState(repo)
	if err == nil {
		t.Error("Expected error for permission denied, got nil")
	}
}

func TestCaptureGitStateCorruptedRepo(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	headFile := filepath.Join(repo, ".git", "HEAD")
	if err := os.Remove(headFile); err != nil {
		t.Fatalf("Failed to remove HEAD file: %v", err)
	}

	_, err := CaptureGitState(repo)
	if err == nil {
		t.Error("Expected error for corrupted repo, got nil")
	}
}

func TestCaptureGitStateGitNotInstalled(t *testing.T) {
	// We'll test this by temporarily modifying PATH to exclude git

	repo := setupTestRepo(t)
	defer os.RemoveAll(repo)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	os.Setenv("PATH", "")

	_, err := CaptureGitState(repo)
	if err == nil {
		t.Error("Expected error when git is not in PATH, got nil")
	}

	if err != nil {
		errMsg := err.Error()
		t.Logf("Error message: %s", errMsg)
	}
}

func TestCaptureGitStateEmptyRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "empty-repo-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	runGit(t, tmpDir, "init", "-b", "main")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "user.name", "Test User")

	state, err := CaptureGitState(tmpDir)
	if err != nil {
		t.Fatalf("Failed to capture git state for empty repo: %v", err)
	}

	if state.Branch == "" {
		t.Error("Expected branch name even in empty repo")
	}

	// HEAD might be empty or point to a non-existent commit
	// This is acceptable behavior
	t.Logf("Empty repo HEAD: %s", state.Head)
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
