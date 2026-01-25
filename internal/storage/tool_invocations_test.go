package storage

import (
	"testing"
	"time"
)

// TestCreateToolInvocation smoke test
func TestCreateToolInvocation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create a prompt first (tool invocations reference prompts)
	// Insert directly to avoid session dependency
	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, session_id, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "prompt-1", time.Now().Unix(), "session-1", "test-agent", "test prompt", "/test/repo", "test-user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	ti := &ToolInvocation{
		ID:        "ti-1",
		PromptID:  "prompt-1",
		ToolName:  "Read",
		ToolArgs:  `{"file_path": "test.go"}`,
		Timestamp: time.Now(),
	}

	err = CreateToolInvocation(db, ti)
	if err != nil {
		t.Fatalf("Failed to create tool invocation: %v", err)
	}

	// Verify it was created
	retrieved, err := GetToolInvocation(db, "ti-1")
	if err != nil {
		t.Fatalf("Failed to get tool invocation: %v", err)
	}

	if retrieved.ID != ti.ID {
		t.Errorf("Expected ID %s, got %s", ti.ID, retrieved.ID)
	}
	if retrieved.ToolName != ti.ToolName {
		t.Errorf("Expected ToolName %s, got %s", ti.ToolName, retrieved.ToolName)
	}
}

// TestGetToolInvocation smoke test
func TestGetToolInvocation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create prompt directly
	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, session_id, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "prompt-2", time.Now().Unix(), "session-2", "test-agent", "test", "/test", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	ti := &ToolInvocation{
		ID:        "ti-2",
		PromptID:  "prompt-2",
		ToolName:  "Write",
		ToolArgs:  `{"file_path": "output.txt"}`,
		Timestamp: time.Now(),
	}

	if err := CreateToolInvocation(db, ti); err != nil {
		t.Fatalf("Failed to create tool invocation: %v", err)
	}

	retrieved, err := GetToolInvocation(db, "ti-2")
	if err != nil {
		t.Fatalf("Failed to get tool invocation: %v", err)
	}

	if retrieved.ToolArgs != ti.ToolArgs {
		t.Errorf("Expected ToolArgs %s, got %s", ti.ToolArgs, retrieved.ToolArgs)
	}
}

// TestGetToolInvocationNotFound tests error handling
func TestGetToolInvocationNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	_, err := GetToolInvocation(db, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestGetToolInvocationsForPrompt smoke test
func TestGetToolInvocationsForPrompt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create prompt directly
	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, session_id, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "prompt-3", time.Now().Unix(), "session-3", "test-agent", "test", "/test", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	// Create multiple tool invocations
	tools := []*ToolInvocation{
		{
			ID:        "ti-3a",
			PromptID:  "prompt-3",
			ToolName:  "Read",
			ToolArgs:  `{"file_path": "a.go"}`,
			Timestamp: time.Now(),
		},
		{
			ID:        "ti-3b",
			PromptID:  "prompt-3",
			ToolName:  "Edit",
			ToolArgs:  `{"file_path": "a.go"}`,
			Timestamp: time.Now().Add(1 * time.Second),
		},
		{
			ID:        "ti-3c",
			PromptID:  "prompt-3",
			ToolName:  "Write",
			ToolArgs:  `{"file_path": "b.go"}`,
			Timestamp: time.Now().Add(2 * time.Second),
		},
	}

	for _, ti := range tools {
		if err := CreateToolInvocation(db, ti); err != nil {
			t.Fatalf("Failed to create tool invocation %s: %v", ti.ID, err)
		}
	}

	// Get all invocations for this prompt
	invocations, err := GetToolInvocationsForPrompt(db, "prompt-3")
	if err != nil {
		t.Fatalf("Failed to get tool invocations: %v", err)
	}

	if len(invocations) != 3 {
		t.Fatalf("Expected 3 invocations, got %d", len(invocations))
	}

	// Should be ordered by timestamp ASC
	if invocations[0].ToolName != "Read" {
		t.Errorf("Expected first tool to be Read, got %s", invocations[0].ToolName)
	}
	if invocations[2].ToolName != "Write" {
		t.Errorf("Expected last tool to be Write, got %s", invocations[2].ToolName)
	}
}

// TestDeleteToolInvocation smoke test
func TestDeleteToolInvocation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create prompt directly
	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, session_id, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "prompt-4", time.Now().Unix(), "session-4", "test-agent", "test", "/test", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	ti := &ToolInvocation{
		ID:        "ti-4",
		PromptID:  "prompt-4",
		ToolName:  "Bash",
		ToolArgs:  `{"command": "ls"}`,
		Timestamp: time.Now(),
	}

	if err := CreateToolInvocation(db, ti); err != nil {
		t.Fatalf("Failed to create tool invocation: %v", err)
	}

	// Delete it
	err = DeleteToolInvocation(db, "ti-4")
	if err != nil {
		t.Fatalf("Failed to delete tool invocation: %v", err)
	}

	// Verify it's gone
	_, err = GetToolInvocation(db, "ti-4")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after deletion, got %v", err)
	}
}

// TestDeleteToolInvocationsForPrompt smoke test
func TestDeleteToolInvocationsForPrompt(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create prompt directly
	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, session_id, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "prompt-5", time.Now().Unix(), "session-5", "test-agent", "test", "/test", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	// Create multiple invocations
	for i := 0; i < 3; i++ {
		ti := &ToolInvocation{
			ID:        "ti-5" + string(rune('a'+i)),
			PromptID:  "prompt-5",
			ToolName:  "Read",
			Timestamp: time.Now(),
		}
		if err := CreateToolInvocation(db, ti); err != nil {
			t.Fatalf("Failed to create tool invocation: %v", err)
		}
	}

	// Delete all for this prompt
	err = DeleteToolInvocationsForPrompt(db, "prompt-5")
	if err != nil {
		t.Fatalf("Failed to delete tool invocations: %v", err)
	}

	// Verify they're all gone
	invocations, err := GetToolInvocationsForPrompt(db, "prompt-5")
	if err != nil {
		t.Fatalf("Failed to query invocations: %v", err)
	}

	if len(invocations) != 0 {
		t.Errorf("Expected 0 invocations after deletion, got %d", len(invocations))
	}
}

// TestToolInvocationCascadeDelete tests that deleting a prompt cascades to tool invocations
func TestToolInvocationCascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close() //nolint:errcheck

	// Create prompt directly
	_, err := db.Exec(`
		INSERT INTO prompt_events (id, timestamp, session_id, agent, prompt_text, repo_path, author)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "prompt-6", time.Now().Unix(), "session-6", "test-agent", "test", "/test", "user")
	if err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	// Create tool invocation
	ti := &ToolInvocation{
		ID:        "ti-6",
		PromptID:  "prompt-6",
		ToolName:  "Read",
		Timestamp: time.Now(),
	}
	if err := CreateToolInvocation(db, ti); err != nil {
		t.Fatalf("Failed to create tool invocation: %v", err)
	}

	// Delete the prompt (should cascade to tool invocations)
	_, err = db.Exec("DELETE FROM prompt_events WHERE id = ?", "prompt-6")
	if err != nil {
		t.Fatalf("Failed to delete prompt: %v", err)
	}

	// Verify tool invocation is also gone
	_, err = GetToolInvocation(db, "ti-6")
	if err != ErrNotFound {
		t.Errorf("Expected tool invocation to be cascade deleted, got err: %v", err)
	}
}
