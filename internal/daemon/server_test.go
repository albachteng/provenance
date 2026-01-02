package daemon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/config"
	"github.com/albachteng/provenance/internal/session"
	"github.com/albachteng/provenance/internal/storage"
)

func TestDaemonStartAndBind(t *testing.T) {
	tmpDir, db, sessionMgr, cfg := setupTestDaemon(t)
	defer db.Close()

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath, sessionMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Start()
	}()

	<-daemon.Ready()

	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("Socket file not created")
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to daemon socket: %v", err)
	}
	conn.Close()

	if err := daemon.Stop(); err != nil {
		t.Errorf("Failed to stop daemon: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != ErrDaemonStopped {
			t.Errorf("Daemon returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Daemon did not stop within timeout")
	}
}

func TestDaemonAcceptEvent(t *testing.T) {
	tmpDir, db, sessionMgr, cfg := setupTestDaemon(t)
	defer db.Close()

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath, sessionMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start()
	defer daemon.Stop()

	<-daemon.Ready()

	// Create a test session first (for foreign key constraint)
	session := &storage.Session{
		ID:        "test-session-daemon",
		StartTime: time.Now(),
		RepoPath:  "/home/user/test",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	event := &storage.PromptEvent{
		ID:         "daemon-event-1",
		Timestamp:  time.Now(),
		SessionID:  "test-session-daemon",
		Agent:      "test-agent",
		PromptText: "Test prompt from daemon",
		RepoPath:   "/home/user/test",
		Author:     "testuser",
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(event); err != nil {
		t.Fatalf("Failed to send event: %v", err)
	}

	retrieved := waitForEvent(t, db, event.ID, 1*time.Second)

	if retrieved.PromptText != event.PromptText {
		t.Errorf("Expected prompt %s, got %s", event.PromptText, retrieved.PromptText)
	}
}

func TestDaemonInvalidJSON(t *testing.T) {
	tmpDir, db, sessionMgr, cfg := setupTestDaemon(t)
	defer db.Close()

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath, sessionMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start()
	defer daemon.Stop()

	<-daemon.Ready()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("{invalid json}"))
	if err != nil {
		t.Fatalf("Failed to write to socket: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	conn2, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Error("Daemon crashed on invalid JSON")
	} else {
		conn2.Close()
	}
}

func TestDaemonGracefulShutdown(t *testing.T) {
	tmpDir, db, sessionMgr, cfg := setupTestDaemon(t)
	defer db.Close()

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath, sessionMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Start()
	}()

	<-daemon.Ready()

	if err := daemon.Stop(); err != nil {
		t.Errorf("Failed to stop daemon: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != ErrDaemonStopped {
			t.Errorf("Expected ErrDaemonStopped, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Daemon did not stop within timeout")
	}

	if _, err := os.Stat(socketPath); err == nil {
		t.Error("Socket file not cleaned up after shutdown")
	}
}

func TestDaemonConcurrentEvents(t *testing.T) {
	tmpDir, db, sessionMgr, cfg := setupTestDaemon(t)
	defer db.Close()

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath, sessionMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start()
	defer daemon.Stop()

	<-daemon.Ready()

	session := &storage.Session{
		ID:        "concurrent-session",
		StartTime: time.Now(),
		RepoPath:  "/home/user/concurrent",
	}
	if err := storage.CreateSession(db, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	numEvents := 10
	var wg sync.WaitGroup
	wg.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		go func(index int) {
			defer wg.Done()

			event := &storage.PromptEvent{
				ID:         fmt.Sprintf("concurrent-%d", index),
				Timestamp:  time.Now(),
				SessionID:  "concurrent-session",
				Agent:      "test-agent",
				PromptText: fmt.Sprintf("Concurrent prompt %d", index),
				RepoPath:   "/home/user/concurrent",
				Author:     "testuser",
			}

			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Errorf("Failed to connect: %v", err)
				return
			}
			defer conn.Close()

			encoder := json.NewEncoder(conn)
			if err := encoder.Encode(event); err != nil {
				t.Errorf("Failed to send event: %v", err)
			}
		}(i)
	}

	wg.Wait()

	events := waitForEventCount(t, db, "concurrent-session", numEvents, 2*time.Second)

	if len(events) != numEvents {
		t.Errorf("Expected %d events, got %d", numEvents, len(events))
	}
}

func TestDaemonSessionEvent(t *testing.T) {
	tmpDir, db, sessionMgr, cfg := setupTestDaemon(t)
	defer db.Close()

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath, sessionMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start()
	defer daemon.Stop()

	<-daemon.Ready()

	sessionEvent := &SessionEvent{
		Type: "session_start",
		Session: storage.Session{
			ID:        "daemon-session-1",
			StartTime: time.Now(),
			RepoPath:  "/home/user/project",
		},
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(sessionEvent); err != nil {
		t.Fatalf("Failed to send session event: %v", err)
	}

	session := waitForSession(t, db, "/home/user/project", 1*time.Second)

	if session.ID != "daemon-session-1" {
		t.Errorf("Expected session ID daemon-session-1, got %s", session.ID)
	}
}

func TestDaemonMultipleConnections(t *testing.T) {
	tmpDir, db, sessionMgr, cfg := setupTestDaemon(t)
	defer db.Close()

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath, sessionMgr, cfg)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start()
	defer daemon.Stop()

	<-daemon.Ready()

	numConns := 5
	conns := make([]net.Conn, numConns)

	for i := 0; i < numConns; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("Failed to open connection %d: %v", i, err)
		}
		conns[i] = conn
	}

	for i, conn := range conns {
		if err := conn.Close(); err != nil {
			t.Errorf("Failed to close connection %d: %v", i, err)
		}
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Error("Daemon not responsive after multiple connections")
	} else {
		conn.Close()
	}
}

// Helper functions

func setupTestDaemon(t *testing.T) (string, *sql.DB, *session.Manager, *config.Config) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Create test config
	cfg := config.Default()
	cfg.Daemon.SocketPath = filepath.Join(tmpDir, "test.sock")
	cfg.Storage.DBPath = dbPath

	// Create session manager with smart-time strategy
	strategy, err := cfg.CreateSessionStrategy()
	if err != nil {
		t.Fatalf("Failed to create session strategy: %v", err)
	}
	sessionMgr := session.NewManager(db, strategy)

	return tmpDir, db, sessionMgr, cfg
}

// waitForEvent polls the database until the event exists or timeout expires
func waitForEvent(t *testing.T, db *sql.DB, eventID string, timeout time.Duration) *storage.PromptEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		event, err := storage.GetPromptEvent(db, eventID)
		if err == nil {
			return event
		}
		if err != storage.ErrNotFound {
			t.Fatalf("Unexpected error querying event: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Event %s not found within %v timeout", eventID, timeout)
	return nil
}

// waitForSession polls the database until the session exists or timeout expires
func waitForSession(t *testing.T, db *sql.DB, repoPath string, timeout time.Duration) *storage.Session {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, err := storage.GetActiveSession(db, repoPath)
		if err == nil {
			return session
		}
		if err != storage.ErrNotFound {
			t.Fatalf("Unexpected error querying session: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Session for repo %s not found within %v timeout", repoPath, timeout)
	return nil
}

// waitForEventCount polls until the expected number of events exist for a session
func waitForEventCount(t *testing.T, db *sql.DB, sessionID string, expectedCount int, timeout time.Duration) []*storage.PromptEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events, err := storage.ListPromptEvents(db, sessionID)
		if err != nil {
			t.Fatalf("Failed to list events: %v", err)
		}
		if len(events) == expectedCount {
			return events
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Get current count for better error message
	events, _ := storage.ListPromptEvents(db, sessionID)
	t.Fatalf("Expected %d events for session %s, got %d after %v timeout",
		expectedCount, sessionID, len(events), timeout)
	return nil
}
