package storage

import (
	"testing"
	"time"
)

func TestCreateTag(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "p-tag-1", time.Now().Unix(), "claude-code", "test prompt", "/repo", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	tag := &PromptTag{
		ID:        "tag-1",
		PromptID:  "p-tag-1",
		CommitSHA: "abc123",
		Note:      "test note",
		CreatedAt: time.Now(),
	}

	if err := CreateTag(db, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	// Duplicate should fail
	tag2 := &PromptTag{
		ID:        "tag-1b",
		PromptID:  "p-tag-1",
		CommitSHA: "abc123",
		CreatedAt: time.Now(),
	}
	if err := CreateTag(db, tag2); err == nil {
		t.Error("Expected error on duplicate tag, got nil")
	}
}

func TestGetTagsForPrompt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "p-tag-2", time.Now().Unix(), "claude-code", "test prompt", "/repo", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	for i, sha := range []string{"sha-a", "sha-b", "sha-c"} {
		tag := &PromptTag{
			ID:        "tag-2" + string(rune('a'+i)),
			PromptID:  "p-tag-2",
			CommitSHA: sha,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := CreateTag(db, tag); err != nil {
			t.Fatalf("CreateTag failed: %v", err)
		}
	}

	tags, err := GetTagsForPrompt(db, "p-tag-2")
	if err != nil {
		t.Fatalf("GetTagsForPrompt failed: %v", err)
	}

	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
	if tags[0].CommitSHA != "sha-a" {
		t.Errorf("Expected first tag commit sha-a, got %s", tags[0].CommitSHA)
	}
}

func TestGetTagsForCommit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	for i, pid := range []string{"p-tag-3a", "p-tag-3b"} {
		_, err := db.Exec(`
			INSERT INTO prompt_events (id, timestamp, agent, prompt_text, repo_path, author)
			VALUES (?, ?, ?, ?, ?, ?)
		`, pid, time.Now().Unix(), "claude-code", "test", "/repo", "user")
		if err != nil {
			t.Fatalf("Failed to create prompt %d: %v", i, err)
		}
		tag := &PromptTag{
			ID:        "tag-3" + string(rune('a'+i)),
			PromptID:  pid,
			CommitSHA: "deadbeef",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := CreateTag(db, tag); err != nil {
			t.Fatalf("CreateTag failed: %v", err)
		}
	}

	tags, err := GetTagsForCommit(db, "deadbeef")
	if err != nil {
		t.Fatalf("GetTagsForCommit failed: %v", err)
	}

	if len(tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tags))
	}
}

func TestDeleteTag(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "p-tag-4", time.Now().Unix(), "claude-code", "test", "/repo", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	tag := &PromptTag{
		ID:        "tag-4",
		PromptID:  "p-tag-4",
		CommitSHA: "cafe1234",
		CreatedAt: time.Now(),
	}
	if err := CreateTag(db, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if err := DeleteTag(db, "p-tag-4", "cafe1234"); err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}

	// Should return ErrNotFound after deletion
	if err := DeleteTag(db, "p-tag-4", "cafe1234"); err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestTagCascadeDeleteWithPrompt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "p-tag-5", time.Now().Unix(), "claude-code", "test", "/repo", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	tag := &PromptTag{
		ID:        "tag-5",
		PromptID:  "p-tag-5",
		CommitSHA: "beef0000",
		CreatedAt: time.Now(),
	}
	if err := CreateTag(db, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	_, err = db.Exec("DELETE FROM prompt_events WHERE id = ?", "p-tag-5")
	if err != nil {
		t.Fatalf("Failed to delete prompt: %v", err)
	}

	// Tag should be cascade deleted
	if err := DeleteTag(db, "p-tag-5", "beef0000"); err != ErrNotFound {
		t.Errorf("Expected tag to be cascade deleted, got: %v", err)
	}
}
