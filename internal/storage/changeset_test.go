package storage

import (
	"testing"
	"time"
)

// TestCreateChangeSet tests creating a new change set
func TestCreateChangeSet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a session first (foreign key requirement)
	session := &Session{
		ID:        "session-123",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create a prompt event (foreign key requirement)
	event := &PromptEvent{
		ID:         "evt-test-123",
		Timestamp:  time.Now(),
		SessionID:  "session-123",
		Agent:      "claude-code",
		PromptText: "Add user authentication",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}

	err = StorePromptEvent(db, event)
	if err != nil {
		t.Fatalf("Failed to store prompt event: %v", err)
	}

	// Create a change set linking the prompt to a commit
	changeSet := &ChangeSet{
		ID:                  "cs-test-1",
		PromptID:            "evt-test-123",
		SessionID:           "session-123",
		Timestamp:           time.Now(),
		FilesChanged:        []string{"auth.go", "auth_test.go"},
		DiffSummary:         "+150 -20",
		CommitIntroduced:    "abc123def456",
		CorrelationMethod:   "git_hook",
		Confidence:          0.95,
		TimeToFirstChangeMs: 5000,
	}

	err = CreateChangeSet(db, changeSet)
	if err != nil {
		t.Fatalf("CreateChangeSet failed: %v", err)
	}

	// Verify it was stored correctly
	retrieved, err := GetChangeSet(db, "cs-test-1")
	if err != nil {
		t.Fatalf("GetChangeSet failed: %v", err)
	}

	if retrieved.PromptID != "evt-test-123" {
		t.Errorf("PromptID = %s, want evt-test-123", retrieved.PromptID)
	}

	if retrieved.CommitIntroduced != "abc123def456" {
		t.Errorf("CommitIntroduced = %s, want abc123def456", retrieved.CommitIntroduced)
	}

	if retrieved.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want 0.95", retrieved.Confidence)
	}

	if len(retrieved.FilesChanged) != 2 {
		t.Errorf("FilesChanged length = %d, want 2", len(retrieved.FilesChanged))
	}

	if retrieved.DiffSummary != "+150 -20" {
		t.Errorf("DiffSummary = %s, want +150 -20", retrieved.DiffSummary)
	}
}

// TestGetChangeSetsForPrompt tests retrieving all change sets for a specific prompt
func TestGetChangeSetsForPrompt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a session first
	session := &Session{
		ID:        "session-123",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create a prompt event
	event := &PromptEvent{
		ID:         "evt-multi-123",
		Timestamp:  time.Now(),
		SessionID:  "session-123",
		Agent:      "claude-code",
		PromptText: "Refactor authentication",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}

	err = StorePromptEvent(db, event)
	if err != nil {
		t.Fatalf("Failed to store prompt event: %v", err)
	}

	// Create multiple change sets for the same prompt
	changeSets := []*ChangeSet{
		{
			ID:                "cs-1",
			PromptID:          "evt-multi-123",
			SessionID:         "session-123",
			Timestamp:         time.Now(),
			FilesChanged:      []string{"auth.go"},
			CommitIntroduced:  "commit1",
			CorrelationMethod: "git_hook",
			Confidence:        0.9,
		},
		{
			ID:                "cs-2",
			PromptID:          "evt-multi-123",
			SessionID:         "session-123",
			Timestamp:         time.Now().Add(5 * time.Minute),
			FilesChanged:      []string{"auth_test.go"},
			CommitIntroduced:  "commit2",
			CorrelationMethod: "git_hook",
			Confidence:        0.8,
		},
	}

	for _, cs := range changeSets {
		err = CreateChangeSet(db, cs)
		if err != nil {
			t.Fatalf("Failed to create change set: %v", err)
		}
	}

	// Retrieve all change sets for this prompt
	retrieved, err := GetChangeSetsForPrompt(db, "evt-multi-123")
	if err != nil {
		t.Fatalf("GetChangeSetsForPrompt failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 change sets, got %d", len(retrieved))
	}

	// Verify they're ordered by timestamp (oldest first)
	if retrieved[0].CommitIntroduced != "commit1" {
		t.Errorf("First change set commit = %s, want commit1", retrieved[0].CommitIntroduced)
	}

	if retrieved[1].CommitIntroduced != "commit2" {
		t.Errorf("Second change set commit = %s, want commit2", retrieved[1].CommitIntroduced)
	}
}

// TestGetChangeSetsForCommit tests retrieving all change sets for a specific commit
func TestGetChangeSetsForCommit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a session first
	session := &Session{
		ID:        "session-123",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create multiple prompt events
	events := []*PromptEvent{
		{
			ID:         "evt-1",
			Timestamp:  time.Now(),
			SessionID:  "session-123",
			Agent:      "claude-code",
			PromptText: "Add feature A",
			RepoPath:   "/test/repo",
			Author:     "testuser",
		},
		{
			ID:         "evt-2",
			Timestamp:  time.Now(),
			SessionID:  "session-123",
			Agent:      "claude-code",
			PromptText: "Add feature B",
			RepoPath:   "/test/repo",
			Author:     "testuser",
		},
	}

	for _, event := range events {
		err := StorePromptEvent(db, event)
		if err != nil {
			t.Fatalf("Failed to store prompt event: %v", err)
		}
	}

	// Create change sets for different prompts but same commit
	changeSets := []*ChangeSet{
		{
			ID:                "cs-commit-1",
			PromptID:          "evt-1",
			SessionID:         "session-123",
			Timestamp:         time.Now(),
			FilesChanged:      []string{"feature_a.go"},
			CommitIntroduced:  "shared-commit-123",
			CorrelationMethod: "git_hook",
			Confidence:        0.95,
		},
		{
			ID:                "cs-commit-2",
			PromptID:          "evt-2",
			SessionID:         "session-123",
			Timestamp:         time.Now(),
			FilesChanged:      []string{"feature_b.go"},
			CommitIntroduced:  "shared-commit-123",
			CorrelationMethod: "git_hook",
			Confidence:        0.90,
		},
	}

	for _, cs := range changeSets {
		err := CreateChangeSet(db, cs)
		if err != nil {
			t.Fatalf("Failed to create change set: %v", err)
		}
	}

	// Retrieve all change sets for this commit
	retrieved, err := GetChangeSetsForCommit(db, "shared-commit-123")
	if err != nil {
		t.Fatalf("GetChangeSetsForCommit failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 change sets, got %d", len(retrieved))
	}

	// Should be ordered by confidence (highest first)
	if retrieved[0].Confidence < retrieved[1].Confidence {
		t.Error("Change sets should be ordered by confidence (highest first)")
	}

	// Verify prompt IDs
	foundEvt1 := false
	foundEvt2 := false
	for _, cs := range retrieved {
		if cs.PromptID == "evt-1" {
			foundEvt1 = true
		}
		if cs.PromptID == "evt-2" {
			foundEvt2 = true
		}
	}

	if !foundEvt1 || !foundEvt2 {
		t.Error("Expected to find change sets for both evt-1 and evt-2")
	}
}

// TestChangeSetForeignKeyConstraint tests that creating a change set with invalid prompt ID fails
func TestChangeSetForeignKeyConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	changeSet := &ChangeSet{
		ID:                "cs-invalid",
		PromptID:          "nonexistent-prompt",
		SessionID:         "nonexistent-session",
		Timestamp:         time.Now(),
		FilesChanged:      []string{"test.go"},
		CommitIntroduced:  "abc123",
		CorrelationMethod: "git_hook",
		Confidence:        0.5,
	}

	err := CreateChangeSet(db, changeSet)
	if err == nil {
		t.Fatal("Expected error when creating change set with invalid prompt ID, got nil")
	}

	// Error should mention foreign key constraint
	if err != nil && err.Error() == "" {
		t.Error("Expected meaningful error message for foreign key violation")
	}
}

// TestGetChangeSetNotFound tests handling of non-existent change set
func TestGetChangeSetNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := GetChangeSet(db, "nonexistent-id")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestChangeSetFilesChangedJSON tests that files_changed array is properly serialized
func TestChangeSetFilesChangedJSON(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create session first
	session := &Session{
		ID:        "session-123",
		StartTime: time.Now(),
		RepoPath:  "/test/repo",
	}
	err := CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create prompt
	event := &PromptEvent{
		ID:         "evt-json-test",
		Timestamp:  time.Now(),
		SessionID:  "session-123",
		Agent:      "claude-code",
		PromptText: "Test",
		RepoPath:   "/test/repo",
		Author:     "testuser",
	}

	err = StorePromptEvent(db, event)
	if err != nil {
		t.Fatalf("Failed to store prompt event: %v", err)
	}

	// Create change set with various file paths
	files := []string{
		"src/main.go",
		"internal/api/handler.go",
		"cmd/server/main.go",
		"README.md",
	}

	changeSet := &ChangeSet{
		ID:                "cs-json",
		PromptID:          "evt-json-test",
		SessionID:         "session-123",
		Timestamp:         time.Now(),
		FilesChanged:      files,
		CommitIntroduced:  "abc123",
		CorrelationMethod: "git_hook",
		Confidence:        0.85,
	}

	err = CreateChangeSet(db, changeSet)
	if err != nil {
		t.Fatalf("CreateChangeSet failed: %v", err)
	}

	retrieved, err := GetChangeSet(db, "cs-json")
	if err != nil {
		t.Fatalf("GetChangeSet failed: %v", err)
	}

	if len(retrieved.FilesChanged) != len(files) {
		t.Fatalf("Expected %d files, got %d", len(files), len(retrieved.FilesChanged))
	}

	for i, file := range files {
		if retrieved.FilesChanged[i] != file {
			t.Errorf("File[%d] = %s, want %s", i, retrieved.FilesChanged[i], file)
		}
	}
}
