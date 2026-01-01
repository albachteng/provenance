package session

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/storage"
)

// TestManagerCreation tests creating a new session manager
func TestManagerCreation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)
	manager := NewManager(db, strategy)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.GetCurrentSession() != nil {
		t.Error("New manager should not have an active session")
	}
}

// TestManagerStartSession tests starting a new session
func TestManagerStartSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)
	manager := NewManager(db, strategy)

	repoPath := "/test/repo"
	sessionID, err := manager.StartSession(repoPath)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("StartSession returned empty session ID")
	}

	currentSession := manager.GetCurrentSession()
	if currentSession == nil {
		t.Fatal("Expected active session after StartSession")
	}

	if currentSession.ID != sessionID {
		t.Errorf("Current session ID = %s, want %s", currentSession.ID, sessionID)
	}

	if currentSession.RepoPath != repoPath {
		t.Errorf("Current session RepoPath = %s, want %s", currentSession.RepoPath, repoPath)
	}
}

// TestManagerEndSession tests ending the current session
func TestManagerEndSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)
	manager := NewManager(db, strategy)

	sessionID, err := manager.StartSession("/test/repo")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	err = manager.EndSession(sessionID, EndReasonManual)
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}

	if manager.GetCurrentSession() != nil {
		t.Error("Expected no active session after EndSession")
	}

	// Verify session was persisted with end time
	session, err := storage.GetSession(db, sessionID)
	if err != nil {
		t.Fatalf("Failed to get session from database: %v", err)
	}

	if session.EndTime.IsZero() {
		t.Error("Session EndTime should be set after EndSession")
	}
}

// TestManagerRecordEvent tests recording an event updates activity tracking
func TestManagerRecordEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)
	manager := NewManager(db, strategy)

	sessionID, err := manager.StartSession("/test/repo")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Record an event
	eventID := "evt-test-123"
	err = manager.RecordEvent(sessionID, eventID)
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	// Verify activity was updated
	session := manager.GetCurrentSession()
	if session == nil {
		t.Fatal("Expected active session")
	}

	if session.EventCount != 1 {
		t.Errorf("EventCount = %d, want 1", session.EventCount)
	}

	// Last event time should be recent
	timeSinceLastEvent := time.Since(session.LastEventTime)
	if timeSinceLastEvent > 1*time.Second {
		t.Errorf("LastEventTime too old: %v ago", timeSinceLastEvent)
	}
}

// TestManagerSetLLMActive tests setting LLM activity state
func TestManagerSetLLMActive(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)
	manager := NewManager(db, strategy)

	_, err := manager.StartSession("/test/repo")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Set LLM active
	manager.SetLLMActive(true)

	session := manager.GetCurrentSession()
	if !session.IsLLMActive {
		t.Error("Expected IsLLMActive = true")
	}

	// Set LLM idle
	manager.SetLLMActive(false)

	session = manager.GetCurrentSession()
	if session.IsLLMActive {
		t.Error("Expected IsLLMActive = false")
	}
}

// TestManagerAutoEndSession tests automatic session ending based on strategy
func TestManagerAutoEndSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Use a very short timeout for testing
	strategy := NewSmartTimeStrategy(100*time.Millisecond, 50*time.Millisecond, false)
	manager := NewManager(db, strategy)

	sessionID, err := manager.StartSession("/test/repo")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Check if session should end
	shouldEnd := manager.CheckSessionBoundaries()
	if !shouldEnd {
		t.Error("Expected session to end after timeout")
	}

	// Verify session ended
	if manager.GetCurrentSession() != nil {
		t.Error("Expected no active session after auto-end")
	}

	// Verify session persisted
	session, err := storage.GetSession(db, sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if session.EndTime.IsZero() {
		t.Error("Session should have EndTime after auto-end")
	}
}

// TestManagerMultipleSessionsSequential tests creating multiple sessions sequentially
func TestManagerMultipleSessionsSequential(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)
	manager := NewManager(db, strategy)

	// Start first session
	session1ID, err := manager.StartSession("/test/repo1")
	if err != nil {
		t.Fatalf("StartSession 1 failed: %v", err)
	}

	// End first session
	err = manager.EndSession(session1ID, EndReasonManual)
	if err != nil {
		t.Fatalf("EndSession 1 failed: %v", err)
	}

	// Start second session
	session2ID, err := manager.StartSession("/test/repo2")
	if err != nil {
		t.Fatalf("StartSession 2 failed: %v", err)
	}

	if session1ID == session2ID {
		t.Error("Sequential sessions should have different IDs")
	}

	currentSession := manager.GetCurrentSession()
	if currentSession.ID != session2ID {
		t.Errorf("Current session ID = %s, want %s", currentSession.ID, session2ID)
	}
}

// TestManagerCannotStartSessionWhileActive tests that you can't start a new session while one is active
func TestManagerCannotStartSessionWhileActive(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)
	manager := NewManager(db, strategy)

	_, err := manager.StartSession("/test/repo1")
	if err != nil {
		t.Fatalf("StartSession 1 failed: %v", err)
	}

	// Try to start another session
	_, err = manager.StartSession("/test/repo2")
	if err == nil {
		t.Error("Expected error when starting session while one is active")
	}
}

// setupTestDB creates a temporary test database
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	return db
}
