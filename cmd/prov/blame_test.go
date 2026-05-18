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

// TestBlameCommit tests blaming a specific commit SHA
func TestBlameCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// Create a real git repo
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	if err := git.InitRepo(repoPath); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create first commit
	commit1Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	testFile := filepath.Join(repoPath, "auth.go")
	if err := os.WriteFile(testFile, []byte("package auth\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"auth.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	_, err := git.CreateCommitWithTime(repoPath, "Initial commit", commit1Time)
	if err != nil {
		t.Fatalf("Failed to create commit1: %v", err)
	}

	// Create prompts in commit window
	prompt1 := &storage.PromptEvent{
		ID:              "prompt-blame-1",
		Timestamp:       time.Date(2024, 1, 15, 10, 10, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Implement user authentication",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
		Author:          "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt1); err != nil {
		t.Fatalf("Failed to store prompt: %v", err)
	}

	prompt2 := &storage.PromptEvent{
		ID:              "prompt-blame-2",
		Timestamp:       time.Date(2024, 1, 15, 10, 20, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Add authentication tests",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
		Author:          "testuser",
	}
	if err := storage.StorePromptEvent(db, prompt2); err != nil {
		t.Fatalf("Failed to store prompt2: %v", err)
	}

	// Create second commit
	commit2Time := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if err := os.WriteFile(testFile, []byte("package auth\n\nfunc Login() {}\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"auth.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	commit2SHA, err := git.CreateCommitWithTime(repoPath, "Add authentication", commit2Time)
	if err != nil {
		t.Fatalf("Failed to create commit2: %v", err)
	}

	// Change to repo directory for blame command
	originalDir, _ := os.Getwd()
	os.Chdir(repoPath)          //nolint:errcheck
	defer os.Chdir(originalDir) //nolint:errcheck

	// Run blame command
	output, err := runCLI(t, "blame", commit2SHA)
	if err != nil {
		t.Fatalf("blame command failed: %v\nOutput: %s", err, output)
	}

	// Should show both prompts (no confidence scores in v2)
	if !strings.Contains(output, "prompt-blame-1") {
		t.Errorf("Expected to find prompt-blame-1 in output, got: %s", output)
	}

	if !strings.Contains(output, "prompt-blame-2") {
		t.Errorf("Expected to find prompt-blame-2 in output, got: %s", output)
	}

	if !strings.Contains(output, "Implement user authentication") {
		t.Errorf("Expected to find first prompt text in output, got: %s", output)
	}

	if !strings.Contains(output, "Add authentication tests") {
		t.Errorf("Expected to find second prompt text in output, got: %s", output)
	}

	// Should show commit info
	if !strings.Contains(output, commit2SHA) {
		t.Errorf("Expected to find commit SHA in output, got: %s", output)
	}

	// Should show files changed
	if !strings.Contains(output, "auth.go") {
		t.Errorf("Expected to find changed files in output, got: %s", output)
	}

	// V2: Should NOT have confidence scores
	if strings.Contains(output, "Confidence") || strings.Contains(output, "0.95") {
		t.Errorf("V2 should not show confidence scores, got: %s", output)
	}
}

// TestBlameCommitNoMatches tests blaming a commit with no prompts in window
func TestBlameCommitNoMatches(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// Create a real git repo
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	if err := git.InitRepo(repoPath); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create commit with no prompts in window
	commitTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	testFile := filepath.Join(repoPath, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"test.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	commitSHA, err := git.CreateCommitWithTime(repoPath, "Test commit", commitTime)
	if err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	// Change to repo directory for blame command
	originalDir, _ := os.Getwd()
	os.Chdir(repoPath)          //nolint:errcheck
	defer os.Chdir(originalDir) //nolint:errcheck

	// Run blame command
	output, err := runCLI(t, "blame", commitSHA)
	if err != nil {
		t.Fatalf("blame command failed: %v", err)
	}

	// Should indicate no prompts found
	if !strings.Contains(output, "No prompts found") {
		t.Errorf("Expected 'No prompts found' message, got: %s", output)
	}
}

// TestBlameCommitBranchFiltering tests that only prompts on same branch are shown
func TestBlameCommitBranchFiltering(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// Create a real git repo
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	if err := git.InitRepo(repoPath); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create first commit
	commit1Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	testFile := filepath.Join(repoPath, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"test.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	_, err := git.CreateCommitWithTime(repoPath, "Initial commit", commit1Time)
	if err != nil {
		t.Fatalf("Failed to create commit1: %v", err)
	}

	// Create prompts on different branches
	promptMain := &storage.PromptEvent{
		ID:              "prompt-main",
		Timestamp:       time.Date(2024, 1, 15, 10, 10, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Work on main branch",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
	}
	if err := storage.StorePromptEvent(db, promptMain); err != nil {
		t.Fatalf("Failed to store main prompt: %v", err)
	}

	promptFeature := &storage.PromptEvent{
		ID:              "prompt-feature",
		Timestamp:       time.Date(2024, 1, 15, 10, 15, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Work on feature branch",
		RepoPath:        repoPath,
		GitBranch:       "feature/test",
		BranchAtCapture: "feature/test",
		PreBranchSwitch: false,
	}
	if err := storage.StorePromptEvent(db, promptFeature); err != nil {
		t.Fatalf("Failed to store feature prompt: %v", err)
	}

	// Create second commit on main
	commit2Time := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"test.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	commit2SHA, err := git.CreateCommitWithTime(repoPath, "Add main function", commit2Time)
	if err != nil {
		t.Fatalf("Failed to create commit2: %v", err)
	}

	// Change to repo directory for blame command
	originalDir, _ := os.Getwd()
	os.Chdir(repoPath)          //nolint:errcheck
	defer os.Chdir(originalDir) //nolint:errcheck

	// Run blame command
	output, err := runCLI(t, "blame", commit2SHA)
	if err != nil {
		t.Fatalf("blame command failed: %v\nOutput: %s", err, output)
	}

	// Should show only main branch prompt
	if !strings.Contains(output, "Work on main branch") {
		t.Errorf("Expected to find main branch prompt in output, got: %s", output)
	}

	// Should NOT show feature branch prompt
	if strings.Contains(output, "Work on feature branch") {
		t.Errorf("Should not show feature branch prompt for main commit, got: %s", output)
	}
}

// TestBlameCommitPreBranchSwitchExcluded tests that pre_branch_switch prompts are excluded
func TestBlameCommitPreBranchSwitchExcluded(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// Create a real git repo
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	if err := git.InitRepo(repoPath); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create first commit
	commit1Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	testFile := filepath.Join(repoPath, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"test.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	_, err := git.CreateCommitWithTime(repoPath, "Initial commit", commit1Time)
	if err != nil {
		t.Fatalf("Failed to create commit1: %v", err)
	}

	// Create normal prompt
	promptNormal := &storage.PromptEvent{
		ID:              "prompt-normal",
		Timestamp:       time.Date(2024, 1, 15, 10, 10, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Normal work",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
	}
	if err := storage.StorePromptEvent(db, promptNormal); err != nil {
		t.Fatalf("Failed to store normal prompt: %v", err)
	}

	// Create pre-switch prompt (should be excluded)
	promptPreSwitch := &storage.PromptEvent{
		ID:              "prompt-preswitch",
		Timestamp:       time.Date(2024, 1, 15, 10, 15, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Work before switching branches",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: true, // Flagged as pre-switch
	}
	if err := storage.StorePromptEvent(db, promptPreSwitch); err != nil {
		t.Fatalf("Failed to store pre-switch prompt: %v", err)
	}

	// Create second commit
	commit2Time := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"test.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	commit2SHA, err := git.CreateCommitWithTime(repoPath, "Add main function", commit2Time)
	if err != nil {
		t.Fatalf("Failed to create commit2: %v", err)
	}

	// Change to repo directory for blame command
	originalDir, _ := os.Getwd()
	os.Chdir(repoPath)          //nolint:errcheck
	defer os.Chdir(originalDir) //nolint:errcheck

	// Run blame command
	output, err := runCLI(t, "blame", commit2SHA)
	if err != nil {
		t.Fatalf("blame command failed: %v\nOutput: %s", err, output)
	}

	// Should show normal prompt
	if !strings.Contains(output, "Normal work") {
		t.Errorf("Expected to find normal prompt in output, got: %s", output)
	}

	// Should NOT show pre-switch prompt
	if strings.Contains(output, "Work before switching branches") {
		t.Errorf("Should not show pre-switch prompt, got: %s", output)
	}
}

// TestBlameNoArgument tests blame with no commit specified
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

// TestBlameInitialCommit tests blaming the initial commit (no previous commit)
func TestBlameInitialCommit(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close() //nolint:errcheck

	// Create a real git repo
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	if err := git.InitRepo(repoPath); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create initial commit
	commitTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	testFile := filepath.Join(repoPath, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := git.StageFiles(repoPath, []string{"test.go"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}
	commitSHA, err := git.CreateCommitWithTime(repoPath, "Initial commit", commitTime)
	if err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	// Change to repo directory for blame command
	originalDir, _ := os.Getwd()
	os.Chdir(repoPath)          //nolint:errcheck
	defer os.Chdir(originalDir) //nolint:errcheck

	// Run blame command
	output, err := runCLI(t, "blame", commitSHA)
	if err != nil {
		t.Fatalf("blame command failed: %v", err)
	}

	// Should indicate no prompts (initial commit has no previous window)
	if !strings.Contains(output, "No prompts found") {
		t.Errorf("Expected 'No prompts found' for initial commit, got: %s", output)
	}
}
