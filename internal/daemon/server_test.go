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

	"github.com/albachteng/provenance/internal/storage"
)

func TestDaemonStartAndBind(t *testing.T) {
	tmpDir, db := setupTestDaemon(t)
	defer db.Close() //nolint:errcheck

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath)
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
	_ = conn.Close()

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
	tmpDir, db := setupTestDaemon(t)
	defer db.Close() //nolint:errcheck

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start() //nolint:errcheck
	defer daemon.Stop() //nolint:errcheck

	<-daemon.Ready()

	event := &storage.PromptEvent{
		ID:              "daemon-event-1",
		Timestamp:       time.Now(),
		SessionID:       "", // V2: nullable
		Agent:           "test-agent",
		PromptText:      "Test prompt from daemon",
		RepoPath:        "/home/user/test",
		Author:          "testuser",
		BranchAtCapture: "main",
		PreBranchSwitch: false,
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close() //nolint:errcheck

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
	tmpDir, db := setupTestDaemon(t)
	defer db.Close() //nolint:errcheck

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start() //nolint:errcheck
	defer daemon.Stop() //nolint:errcheck

	<-daemon.Ready()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close() //nolint:errcheck

	_, err = conn.Write([]byte("{invalid json}"))
	if err != nil {
		t.Fatalf("Failed to write to socket: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	conn2, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Error("Daemon crashed on invalid JSON")
	} else {
		_ = conn2.Close()
	}
}

func TestDaemonGracefulShutdown(t *testing.T) {
	tmpDir, db := setupTestDaemon(t)
	defer db.Close() //nolint:errcheck

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath)
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
	tmpDir, db := setupTestDaemon(t)
	defer db.Close() //nolint:errcheck

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start() //nolint:errcheck
	defer daemon.Stop() //nolint:errcheck

	<-daemon.Ready()

	numEvents := 10
	var wg sync.WaitGroup
	wg.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		go func(index int) {
			defer wg.Done()

			event := &storage.PromptEvent{
				ID:              fmt.Sprintf("concurrent-%d", index),
				Timestamp:       time.Now(),
				SessionID:       "", // V2: nullable
				Agent:           "test-agent",
				PromptText:      fmt.Sprintf("Concurrent prompt %d", index),
				RepoPath:        "/home/user/concurrent",
				Author:          "testuser",
				BranchAtCapture: "main",
				PreBranchSwitch: false,
			}

			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Errorf("Failed to connect: %v", err)
				return
			}
			defer conn.Close() //nolint:errcheck

			encoder := json.NewEncoder(conn)
			if err := encoder.Encode(event); err != nil {
				t.Errorf("Failed to send event: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all events to be stored
	time.Sleep(100 * time.Millisecond)

	// Verify all events were stored
	for i := 0; i < numEvents; i++ {
		eventID := fmt.Sprintf("concurrent-%d", i)
		_, err := storage.GetPromptEvent(db, eventID)
		if err != nil {
			t.Errorf("Event %s not found: %v", eventID, err)
		}
	}
}

func TestDaemonMultipleConnections(t *testing.T) {
	tmpDir, db := setupTestDaemon(t)
	defer db.Close() //nolint:errcheck

	socketPath := filepath.Join(tmpDir, "test.sock")

	daemon, err := NewDaemon(db, socketPath)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	go daemon.Start() //nolint:errcheck
	defer daemon.Stop() //nolint:errcheck

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
		_ = conn.Close()
	}
}

// Helper functions

func setupTestDaemon(t *testing.T) (string, *sql.DB) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) }) //nolint:errcheck

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	return tmpDir, db
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
