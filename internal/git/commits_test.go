package git

import (
	"strings"
	"testing"
	"time"
)

// TestGetCommitTime smoke test
func TestGetCommitTime(t *testing.T) {
	// This test requires a git repo - we use the provenance repo itself
	repoPath := "../.." // Root of provenance project

	// Get the time of HEAD commit
	commitTime, err := GetCommitTime(repoPath, "HEAD")
	if err != nil {
		t.Fatalf("Failed to get commit time: %v", err)
	}

	// Verify it's a reasonable time (not zero, not in the future)
	if commitTime.IsZero() {
		t.Error("Commit time should not be zero")
	}

	if commitTime.After(time.Now()) {
		t.Error("Commit time should not be in the future")
	}
}

// TestGetCommitTimeInvalidRepo tests error handling
func TestGetCommitTimeInvalidRepo(t *testing.T) {
	_, err := GetCommitTime("/tmp/not-a-repo", "HEAD")
	if err == nil {
		t.Error("Expected error for invalid repo")
	}
}

// TestGetPreviousCommit smoke test
func TestGetPreviousCommit(t *testing.T) {
	repoPath := "../.."

	// Get previous commit of HEAD
	prevCommit, err := GetPreviousCommit(repoPath, "HEAD", "main")
	if err != nil {
		t.Fatalf("Failed to get previous commit: %v", err)
	}

	// Previous commit should be a valid SHA (40 hex characters) or empty for initial commit
	if prevCommit != "" && len(prevCommit) != 40 {
		t.Errorf("Expected 40-character SHA, got %d characters: %s", len(prevCommit), prevCommit)
	}
}

// TestGetCurrentBranch smoke test
func TestGetCurrentBranch(t *testing.T) {
	repoPath := "../.."

	branch, err := GetCurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}

	// Branch name should not be empty (unless detached HEAD)
	// Just verify we got something reasonable
	t.Logf("Current branch: %s", branch)
}

// TestGetFilesChanged smoke test
func TestGetFilesChanged(t *testing.T) {
	repoPath := "../.."

	files, err := GetFilesChanged(repoPath, "HEAD")
	if err != nil {
		t.Fatalf("Failed to get files changed: %v", err)
	}

	// HEAD should have at least one file changed (could be 0 for merge commits)
	t.Logf("Files changed in HEAD: %d", len(files))
}

// TestGetCommitsForFile smoke test
func TestGetCommitsForFile(t *testing.T) {
	repoPath := "../.."

	// Test with a file we know exists
	filePath := "go.mod"

	commits, err := GetCommitsForFile(repoPath, filePath)
	if err != nil {
		t.Fatalf("Failed to get commits for file: %v", err)
	}

	if len(commits) == 0 {
		t.Error("Expected at least one commit for go.mod")
	}

	// Verify commit info is populated
	for i, commit := range commits {
		if commit.SHA == "" {
			t.Errorf("Commit %d has empty SHA", i)
		}
		if commit.Timestamp.IsZero() {
			t.Errorf("Commit %d has zero timestamp", i)
		}
	}
}

// TestGetCommitsInBranch smoke test
func TestGetCommitsInBranch(t *testing.T) {
	repoPath := "../.."

	commits, err := GetCommitsInBranch(repoPath, "HEAD")
	if err != nil {
		t.Fatalf("Failed to get commits in branch: %v", err)
	}

	if len(commits) == 0 {
		t.Error("Expected at least one commit in current branch")
	}

	// Verify commits are ordered (newest first)
	for i := 1; i < len(commits); i++ {
		if commits[i].Timestamp.After(commits[i-1].Timestamp) {
			t.Errorf("Commits not ordered by timestamp: commit %d is newer than commit %d", i, i-1)
		}
	}
}

// TestExtractBranchFromRefs tests the helper function
func TestExtractBranchFromRefs(t *testing.T) {
	tests := []struct {
		refs     string
		expected string
	}{
		{"HEAD -> main, origin/main", "main"},
		{"HEAD -> feature/test", "feature/test"},
		{"origin/main", ""},
		{"", ""},
		{"HEAD, tag: v1.0.0", ""},
		{"develop", "develop"},
	}

	for _, tt := range tests {
		result := extractBranchFromRefs(tt.refs)
		if result != tt.expected {
			t.Errorf("extractBranchFromRefs(%q) = %q, want %q", tt.refs, result, tt.expected)
		}
	}
}

// TestGetBranchForCommit smoke test
func TestGetBranchForCommit(t *testing.T) {
	repoPath := "../.."

	branch, err := GetBranchForCommit(repoPath, "HEAD")
	if err != nil {
		t.Fatalf("Failed to get branch for commit: %v", err)
	}

	// Should return a branch name (not empty)
	if branch == "" {
		t.Error("Expected non-empty branch name")
	}

	// Branch name shouldn't have special characters like * or ->
	if strings.Contains(branch, "*") || strings.Contains(branch, "->") {
		t.Errorf("Branch name should be cleaned: %q", branch)
	}
}
