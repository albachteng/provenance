package queries

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/git"
	"github.com/albachteng/provenance/internal/storage"
)

// TestGetPromptsForCommit_BasicWindow tests that prompts in a commit window are returned
func TestGetPromptsForCommit_BasicWindow(t *testing.T) {
	db, repoPath := setupTestQueriesWithGit(t)
	defer db.Close() //nolint:errcheck

	// Commit timeline on main branch:
	// commit1 (10:00) -> commit2 (10:30)
	// Prompts at 10:10, 10:20 should be associated with commit2

	commit1Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	commit2Time := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	// Create commit1
	_ = createGitCommit(t, repoPath, "Initial commit", commit1Time)

	// Create prompts after commit1
	prompt1 := &storage.PromptEvent{
		ID:              "prompt-1",
		Timestamp:       time.Date(2024, 1, 15, 10, 10, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Add feature X",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
	}
	if err := storage.StorePromptEvent(db, prompt1); err != nil {
		t.Fatalf("Failed to store prompt1: %v", err)
	}

	prompt2 := &storage.PromptEvent{
		ID:              "prompt-2",
		Timestamp:       time.Date(2024, 1, 15, 10, 20, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Fix bug Y",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
	}
	if err := storage.StorePromptEvent(db, prompt2); err != nil {
		t.Fatalf("Failed to store prompt2: %v", err)
	}

	// Create commit2
	commit2SHA := createGitCommit(t, repoPath, "Feature X implemented", commit2Time)

	// Query prompts for commit2
	prompts, err := GetPromptsForCommit(db, repoPath, commit2SHA, "main")
	if err != nil {
		t.Fatalf("GetPromptsForCommit failed: %v", err)
	}

	// Should return both prompts
	if len(prompts) != 2 {
		t.Errorf("Expected 2 prompts, got %d", len(prompts))
	}

	if len(prompts) >= 2 {
		if prompts[0].ID != "prompt-1" {
			t.Errorf("Expected first prompt ID 'prompt-1', got %s", prompts[0].ID)
		}
		if prompts[1].ID != "prompt-2" {
			t.Errorf("Expected second prompt ID 'prompt-2', got %s", prompts[1].ID)
		}
	}
}

// TestGetPromptsForCommit_BranchFiltering tests that only prompts on the same branch are returned
func TestGetPromptsForCommit_BranchFiltering(t *testing.T) {
	db, repoPath := setupTestQueriesWithGit(t)
	defer db.Close() //nolint:errcheck

	commit1Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	commit2Time := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	createGitCommit(t, repoPath, "Initial commit", commit1Time)

	// Prompt on main branch
	promptMain := &storage.PromptEvent{
		ID:              "prompt-main",
		Timestamp:       time.Date(2024, 1, 15, 10, 10, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Work on main",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
	}
	if err := storage.StorePromptEvent(db, promptMain); err != nil {
		t.Fatalf("Failed to store promptMain: %v", err)
	}

	// Prompt on feature branch (should be excluded)
	promptFeature := &storage.PromptEvent{
		ID:              "prompt-feature",
		Timestamp:       time.Date(2024, 1, 15, 10, 15, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Work on feature",
		RepoPath:        repoPath,
		GitBranch:       "feature/test",
		BranchAtCapture: "feature/test",
		PreBranchSwitch: false,
	}
	if err := storage.StorePromptEvent(db, promptFeature); err != nil {
		t.Fatalf("Failed to store promptFeature: %v", err)
	}

	commit2SHA := createGitCommit(t, repoPath, "Commit on main", commit2Time)

	// Query prompts for main branch commit
	prompts, err := GetPromptsForCommit(db, repoPath, commit2SHA, "main")
	if err != nil {
		t.Fatalf("GetPromptsForCommit failed: %v", err)
	}

	// Should only return main branch prompt
	if len(prompts) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(prompts))
	}

	if len(prompts) > 0 && prompts[0].ID != "prompt-main" {
		t.Errorf("Expected prompt 'prompt-main', got %s", prompts[0].ID)
	}
}

// TestGetPromptsForCommit_PreBranchSwitchExcluded tests that pre_branch_switch prompts are excluded
func TestGetPromptsForCommit_PreBranchSwitchExcluded(t *testing.T) {
	db, repoPath := setupTestQueriesWithGit(t)
	defer db.Close() //nolint:errcheck

	commit1Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	commit2Time := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	createGitCommit(t, repoPath, "Initial commit", commit1Time)

	// Normal prompt
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
		t.Fatalf("Failed to store promptNormal: %v", err)
	}

	// Prompt flagged as pre_branch_switch (should be excluded)
	promptPreSwitch := &storage.PromptEvent{
		ID:              "prompt-preswitch",
		Timestamp:       time.Date(2024, 1, 15, 10, 15, 0, 0, time.UTC),
		Agent:           "claude-code",
		PromptText:      "Work before switching branches",
		RepoPath:        repoPath,
		GitBranch:       "main",
		BranchAtCapture: "main",
		PreBranchSwitch: true, // Flagged
	}
	if err := storage.StorePromptEvent(db, promptPreSwitch); err != nil {
		t.Fatalf("Failed to store promptPreSwitch: %v", err)
	}

	commit2SHA := createGitCommit(t, repoPath, "Commit on main", commit2Time)

	// Query prompts for commit
	prompts, err := GetPromptsForCommit(db, repoPath, commit2SHA, "main")
	if err != nil {
		t.Fatalf("GetPromptsForCommit failed: %v", err)
	}

	// Should only return normal prompt (pre_branch_switch excluded)
	if len(prompts) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(prompts))
	}

	if len(prompts) > 0 && prompts[0].ID != "prompt-normal" {
		t.Errorf("Expected prompt 'prompt-normal', got %s", prompts[0].ID)
	}
}

// TestGetPromptsForCommit_InitialCommit tests querying the initial commit (no previous commit)
func TestGetPromptsForCommit_InitialCommit(t *testing.T) {
	db, repoPath := setupTestQueriesWithGit(t)
	defer db.Close() //nolint:errcheck

	commitTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	// Create initial commit
	commitSHA := createGitCommit(t, repoPath, "Initial commit", commitTime)

	// Query prompts for initial commit (no previous commit)
	prompts, err := GetPromptsForCommit(db, repoPath, commitSHA, "main")
	if err != nil {
		t.Fatalf("GetPromptsForCommit failed: %v", err)
	}

	// Should return empty list (no prompts before initial commit)
	if len(prompts) != 0 {
		t.Errorf("Expected 0 prompts for initial commit, got %d", len(prompts))
	}
}

// TestGetPromptsForCommit_EmptyWindow tests a commit with no prompts in its window
func TestGetPromptsForCommit_EmptyWindow(t *testing.T) {
	db, repoPath := setupTestQueriesWithGit(t)
	defer db.Close() //nolint:errcheck

	commit1Time := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	commit2Time := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	createGitCommit(t, repoPath, "First commit", commit1Time)
	commit2SHA := createGitCommit(t, repoPath, "Second commit (no prompts)", commit2Time)

	// No prompts stored between commits

	// Query prompts for commit2
	prompts, err := GetPromptsForCommit(db, repoPath, commit2SHA, "main")
	if err != nil {
		t.Fatalf("GetPromptsForCommit failed: %v", err)
	}

	// Should return empty list
	if len(prompts) != 0 {
		t.Errorf("Expected 0 prompts, got %d", len(prompts))
	}
}

// Helper functions

func setupTestQueriesWithGit(t *testing.T) (*sql.DB, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "queries-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) }) //nolint:errcheck

	// Initialize git repo
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	if err := git.InitRepo(repoPath); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Initialize database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	return db, repoPath
}

func createGitCommit(t *testing.T, repoPath, message string, commitTime time.Time) string {
	t.Helper()

	// Create a file change
	testFile := filepath.Join(repoPath, "test.txt")
	content := message + "\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Stage the change
	if err := git.StageFiles(repoPath, []string{"test.txt"}); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}

	// Create commit with specific timestamp
	commitSHA, err := git.CreateCommitWithTime(repoPath, message, commitTime)
	if err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	return commitSHA
}
