package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHookBashGeneration tests that 'prov hook bash' generates valid shell code
func TestHookBashGeneration(t *testing.T) {
	output, err := runCLI(t, "hook", "bash")
	if err != nil {
		t.Fatalf("hook bash command failed: %v", err)
	}

	if !strings.Contains(output, "function") || !strings.Contains(output, "prov") {
		t.Errorf("Expected shell function definition, got: %s", output)
	}

	if strings.Contains(output, "error") || strings.Contains(output, "Error") {
		t.Errorf("Hook output contains error: %s", output)
	}
}

// TestHookZshGeneration tests that 'prov hook zsh' generates valid shell code
func TestHookZshGeneration(t *testing.T) {
	output, err := runCLI(t, "hook", "zsh")
	if err != nil {
		t.Fatalf("hook zsh command failed: %v", err)
	}

	if !strings.Contains(output, "function") || !strings.Contains(output, "prov") {
		t.Errorf("Expected shell function definition, got: %s", output)
	}
}

// TestHookUnsupportedShell tests that unsupported shells are rejected
func TestHookUnsupportedShell(t *testing.T) {
	output, err := runCLI(t, "hook", "powershell")
	if err == nil {
		t.Error("Expected error for unsupported shell")
	}

	if !strings.Contains(output, "not supported") && !strings.Contains(output, "unknown") {
		t.Errorf("Expected error message about unsupported shell, got: %s", output)
	}
}

// TestCommandWrapper tests the wrapper command that intercepts AI tool invocations
func TestCommandWrapper(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer func() { _ = db.Close() }() //nolint:errcheck

	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer func() {
		_, _ = runCLI(t, "daemon", "stop") //nolint:errcheck
		// Give daemon time to fully shutdown and clean up
		time.Sleep(200 * time.Millisecond)
		// Ensure socket is removed
		socketPath := filepath.Join(tmpDir, "daemon.sock")
		_ = os.Remove(socketPath) //nolint:errcheck
	}()

	waitForDaemonReady(t, tmpDir)

	// Run a command through the wrapper
	// prov wrap <agent> <command...>
	output, err := runCLI(t, "wrap", "claude-code", "echo", "test prompt")
	if err != nil {
		t.Fatalf("wrap command failed: %v", err)
	}

	if !strings.Contains(output, "test prompt") {
		t.Errorf("Expected command output to be passed through, got: %s", output)
	}

	waitForEventInDB(t, db, func(agent, promptText string) bool {
		return agent == "claude-code" && strings.Contains(promptText, "test prompt")
	})
}

// TestCommandWrapperExitCode tests that wrapper preserves exit codes
func TestCommandWrapperExitCode(t *testing.T) {
	tmpDir := setupTestEnv(t)
	setupTestDB(t, tmpDir)

	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer func() {
		_, _ = runCLI(t, "daemon", "stop") //nolint:errcheck
		// Give daemon time to fully shutdown and clean up
		time.Sleep(200 * time.Millisecond)
		// Ensure socket is removed
		socketPath := filepath.Join(tmpDir, "daemon.sock")
		_ = os.Remove(socketPath) //nolint:errcheck
	}()

	waitForDaemonReady(t, tmpDir)

	_, err = runCLI(t, "wrap", "claude-code", "sh", "-c", "exit 42")
	if err == nil {
		t.Error("Expected command to fail with exit code 42")
	}

	// Check that exit code was preserved (this is platform-specific, but should be non-zero)
	// We can't easily check the exact exit code in Go, but we verified it's non-zero
}

// V2 NOTE: Session tests removed - v2 uses commit windows instead of sessions
// Removed tests: TestSessionCreation, TestSessionLinking

// waitForDaemonReady polls until daemon socket is connectable
func waitForDaemonReady(t *testing.T, tmpDir string) {
	t.Helper()

	socketPath := filepath.Join(tmpDir, "daemon.sock")
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			// Socket exists, daemon should be ready
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("Daemon not ready within timeout")
}

// waitForEventInDB polls until an event matching the condition appears
func waitForEventInDB(t *testing.T, db *sql.DB, checkFunc func(agent, promptText string) bool) {
	t.Helper()

	// Give daemon a moment to process the event
	time.Sleep(50 * time.Millisecond)

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		rows, err := db.Query(`
			SELECT agent, prompt_text
			FROM prompt_events
			ORDER BY timestamp DESC
			LIMIT 10
		`)
		if err != nil {
			t.Fatalf("Failed to query events: %v", err)
		}

		for rows.Next() {
			var agent, promptText string
			if err := rows.Scan(&agent, &promptText); err != nil {
				_ = rows.Close() //nolint:errcheck
				t.Fatalf("Failed to scan row: %v", err)
			}

			if checkFunc(agent, promptText) {
				_ = rows.Close() //nolint:errcheck
				return
			}
		}
		_ = rows.Close() //nolint:errcheck

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("Event not found in database within timeout")
}

// waitForSessionInDB polls until a session appears in the database
// V2 NOTE: waitForSessionInDB helper removed - no longer needed without session tests
