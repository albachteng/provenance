package storage

import (
	"testing"
	"time"
)

// TestCreateCommitWindow tests creating a new commit window
func TestCreateCommitWindow(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	cw := &CommitWindow{
		ID:             "cw-test-1",
		RepoPath:       "/test/repo",
		Branch:         "main",
		PrevCommit:     "abc123",
		NextCommit:     "def456",
		PrevCommitTime: time.Unix(1000, 0),
		NextCommitTime: time.Unix(2000, 0),
		PromptCount:    5,
	}

	err := CreateCommitWindow(db, cw)
	if err != nil {
		t.Fatalf("Failed to create commit window: %v", err)
	}

	// Verify it was created
	retrieved, err := GetCommitWindow(db, "cw-test-1")
	if err != nil {
		t.Fatalf("Failed to get commit window: %v", err)
	}

	if retrieved.ID != cw.ID {
		t.Errorf("Expected ID %s, got %s", cw.ID, retrieved.ID)
	}
	if retrieved.RepoPath != cw.RepoPath {
		t.Errorf("Expected RepoPath %s, got %s", cw.RepoPath, retrieved.RepoPath)
	}
	if retrieved.PromptCount != cw.PromptCount {
		t.Errorf("Expected PromptCount %d, got %d", cw.PromptCount, retrieved.PromptCount)
	}
}

// TestCreateCommitWindowInitialCommit tests creating a window for the initial commit (no prev)
func TestCreateCommitWindowInitialCommit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	cw := &CommitWindow{
		ID:             "cw-initial-1",
		RepoPath:       "/test/repo",
		Branch:         "main",
		PrevCommit:     "", // No previous commit
		NextCommit:     "initial123",
		PrevCommitTime: time.Time{}, // Zero value
		NextCommitTime: time.Unix(1000, 0),
		PromptCount:    3,
	}

	err := CreateCommitWindow(db, cw)
	if err != nil {
		t.Fatalf("Failed to create initial commit window: %v", err)
	}

	retrieved, err := GetCommitWindow(db, "cw-initial-1")
	if err != nil {
		t.Fatalf("Failed to get initial commit window: %v", err)
	}

	if retrieved.PrevCommit != "" {
		t.Errorf("Expected empty PrevCommit, got %s", retrieved.PrevCommit)
	}

	if !retrieved.PrevCommitTime.IsZero() {
		t.Errorf("Expected zero PrevCommitTime, got %v", retrieved.PrevCommitTime)
	}
}

// TestGetCommitWindowForCommit tests retrieving a window by commit SHA
func TestGetCommitWindowForCommit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	cw := &CommitWindow{
		ID:             "cw-test-2",
		RepoPath:       "/test/repo",
		Branch:         "feature",
		PrevCommit:     "prev123",
		NextCommit:     "next456",
		PrevCommitTime: time.Unix(1000, 0),
		NextCommitTime: time.Unix(2000, 0),
		PromptCount:    7,
	}

	err := CreateCommitWindow(db, cw)
	if err != nil {
		t.Fatalf("Failed to create commit window: %v", err)
	}

	// Retrieve by commit SHA
	retrieved, err := GetCommitWindowForCommit(db, "/test/repo", "feature", "next456")
	if err != nil {
		t.Fatalf("Failed to get commit window for commit: %v", err)
	}

	if retrieved.ID != cw.ID {
		t.Errorf("Expected ID %s, got %s", cw.ID, retrieved.ID)
	}
}

// TestGetCommitWindowNotFound tests error handling for non-existent window
func TestGetCommitWindowNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	_, err := GetCommitWindow(db, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestListCommitWindows tests listing all windows for a branch
func TestListCommitWindows(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create multiple windows
	windows := []*CommitWindow{
		{
			ID:             "cw-1",
			RepoPath:       "/test/repo",
			Branch:         "main",
			NextCommit:     "commit1",
			NextCommitTime: time.Unix(1000, 0),
			PromptCount:    2,
		},
		{
			ID:             "cw-2",
			RepoPath:       "/test/repo",
			Branch:         "main",
			NextCommit:     "commit2",
			NextCommitTime: time.Unix(2000, 0),
			PromptCount:    3,
		},
		{
			ID:             "cw-3",
			RepoPath:       "/test/repo",
			Branch:         "feature", // Different branch
			NextCommit:     "commit3",
			NextCommitTime: time.Unix(1500, 0),
			PromptCount:    1,
		},
	}

	for _, cw := range windows {
		if err := CreateCommitWindow(db, cw); err != nil {
			t.Fatalf("Failed to create window %s: %v", cw.ID, err)
		}
	}

	// List windows for main branch
	mainWindows, err := ListCommitWindows(db, "/test/repo", "main")
	if err != nil {
		t.Fatalf("Failed to list commit windows: %v", err)
	}

	if len(mainWindows) != 2 {
		t.Fatalf("Expected 2 windows for main branch, got %d", len(mainWindows))
	}

	// Should be ordered by timestamp DESC (newest first)
	if mainWindows[0].NextCommit != "commit2" {
		t.Errorf("Expected first window to be commit2, got %s", mainWindows[0].NextCommit)
	}
}

// TestUpdateCommitWindowPromptCount tests updating the prompt count
func TestUpdateCommitWindowPromptCount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	cw := &CommitWindow{
		ID:             "cw-update-1",
		RepoPath:       "/test/repo",
		Branch:         "main",
		NextCommit:     "commit1",
		NextCommitTime: time.Unix(1000, 0),
		PromptCount:    5,
	}

	if err := CreateCommitWindow(db, cw); err != nil {
		t.Fatalf("Failed to create commit window: %v", err)
	}

	// Update prompt count
	err := UpdateCommitWindowPromptCount(db, "cw-update-1", 10)
	if err != nil {
		t.Fatalf("Failed to update prompt count: %v", err)
	}

	// Verify update
	retrieved, err := GetCommitWindow(db, "cw-update-1")
	if err != nil {
		t.Fatalf("Failed to get updated window: %v", err)
	}

	if retrieved.PromptCount != 10 {
		t.Errorf("Expected PromptCount 10, got %d", retrieved.PromptCount)
	}
}

// TestDeleteCommitWindow tests deleting a window
func TestDeleteCommitWindow(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	cw := &CommitWindow{
		ID:             "cw-delete-1",
		RepoPath:       "/test/repo",
		Branch:         "main",
		NextCommit:     "commit1",
		NextCommitTime: time.Unix(1000, 0),
		PromptCount:    2,
	}

	if err := CreateCommitWindow(db, cw); err != nil {
		t.Fatalf("Failed to create commit window: %v", err)
	}

	// Delete it
	err := DeleteCommitWindow(db, "cw-delete-1")
	if err != nil {
		t.Fatalf("Failed to delete commit window: %v", err)
	}

	// Verify it's gone
	_, err = GetCommitWindow(db, "cw-delete-1")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after deletion, got %v", err)
	}
}

// TestDeleteCommitWindowsForBranch tests deleting all windows for a branch
func TestDeleteCommitWindowsForBranch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create windows on different branches
	windows := []*CommitWindow{
		{
			ID:             "cw-1",
			RepoPath:       "/test/repo",
			Branch:         "main",
			NextCommit:     "commit1",
			NextCommitTime: time.Unix(1000, 0),
		},
		{
			ID:             "cw-2",
			RepoPath:       "/test/repo",
			Branch:         "main",
			NextCommit:     "commit2",
			NextCommitTime: time.Unix(2000, 0),
		},
		{
			ID:             "cw-3",
			RepoPath:       "/test/repo",
			Branch:         "feature",
			NextCommit:     "commit3",
			NextCommitTime: time.Unix(1500, 0),
		},
	}

	for _, cw := range windows {
		if err := CreateCommitWindow(db, cw); err != nil {
			t.Fatalf("Failed to create window %s: %v", cw.ID, err)
		}
	}

	// Delete all main branch windows
	err := DeleteCommitWindowsForBranch(db, "/test/repo", "main")
	if err != nil {
		t.Fatalf("Failed to delete windows for branch: %v", err)
	}

	// Verify main windows are gone
	mainWindows, err := ListCommitWindows(db, "/test/repo", "main")
	if err != nil {
		t.Fatalf("Failed to list windows: %v", err)
	}
	if len(mainWindows) != 0 {
		t.Errorf("Expected 0 windows for main after deletion, got %d", len(mainWindows))
	}

	// Verify feature branch windows still exist
	featureWindows, err := ListCommitWindows(db, "/test/repo", "feature")
	if err != nil {
		t.Fatalf("Failed to list feature windows: %v", err)
	}
	if len(featureWindows) != 1 {
		t.Errorf("Expected 1 window for feature branch, got %d", len(featureWindows))
	}
}
