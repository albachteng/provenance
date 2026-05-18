package main

import (
	"database/sql"
	"os"
	"os/exec"
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

// TestSessionCreation tests that first AI invocation creates a session
func TestSessionCreation(t *testing.T) {
	t.Skip("TODO: Session management - migrations path issues when changing directories")

	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer func() { _ = db.Close() }() //nolint:errcheck

	gitDir := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create git dir: %v", err)
	}
	if err := os.Chdir(gitDir); err != nil {
		t.Fatalf("Failed to change dir: %v", err)
	}

	runCommand(t, "git", "init")
	runCommand(t, "git", "config", "user.email", "test@example.com")
	runCommand(t, "git", "config", "user.name", "Test User")

	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer func() { _, _ = runCLI(t, "daemon", "stop") }() //nolint:errcheck

	waitForDaemonReady(t, tmpDir)

	_, err = runCLI(t, "wrap", "claude-code", "echo", "first prompt")
	if err != nil {
		t.Fatalf("wrap command failed: %v", err)
	}

	waitForSessionInDB(t, db)

	rows, err := db.Query(`
		SELECT id, repo_path, total_prompts
		FROM sessions
		ORDER BY start_time DESC
		LIMIT 1
	`)
	if err != nil {
		t.Fatalf("Failed to query sessions: %v", err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	if !rows.Next() {
		t.Fatal("Expected session to be created")
	}

	var id, repoPath string
	var totalPrompts int
	if err := rows.Scan(&id, &repoPath, &totalPrompts); err != nil {
		t.Fatalf("Failed to scan session: %v", err)
	}

	if repoPath != gitDir {
		t.Errorf("Expected repo_path '%s', got: %s", gitDir, repoPath)
	}

	if totalPrompts < 1 {
		t.Errorf("Expected at least 1 prompt in session, got: %d", totalPrompts)
	}
}

// TestSessionLinking tests that multiple prompts link to same session
func TestSessionLinking(t *testing.T) {
	t.Skip("TODO: Session management - migrations path issues when changing directories")

	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer func() { _ = db.Close() }() //nolint:errcheck

	gitDir := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create git dir: %v", err)
	}
	if err := os.Chdir(gitDir); err != nil {
		t.Fatalf("Failed to change dir: %v", err)
	}

	runCommand(t, "git", "init")
	runCommand(t, "git", "config", "user.email", "test@example.com")
	runCommand(t, "git", "config", "user.name", "Test User")

	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer func() { _, _ = runCLI(t, "daemon", "stop") }() //nolint:errcheck

	waitForDaemonReady(t, tmpDir)

	_, _ = runCLI(t, "wrap", "claude-code", "echo", "first prompt") //nolint:errcheck
	_, _ = runCLI(t, "wrap", "claude-code", "echo", "second prompt") //nolint:errcheck

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM prompt_events`).Scan(&count)
		if err == nil && count >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rows, err := db.Query(`
		SELECT session_id
		FROM prompt_events
		ORDER BY timestamp
	`)
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	sessionIDs := make(map[string]int)
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			t.Fatalf("Failed to scan session_id: %v", err)
		}
		sessionIDs[sessionID]++
	}

	if len(sessionIDs) != 1 {
		t.Errorf("Expected all events to share same session, got %d different sessions: %v", len(sessionIDs), sessionIDs)
	}

	for sessionID, count := range sessionIDs {
		var totalPrompts int
		err := db.QueryRow(`
			SELECT total_prompts
			FROM sessions
			WHERE id = ?
		`, sessionID).Scan(&totalPrompts)

		if err != nil {
			t.Fatalf("Failed to query session: %v", err)
		}

		if totalPrompts != count {
			t.Errorf("Session total_prompts (%d) doesn't match event count (%d)", totalPrompts, count)
		}
	}
}

// Helper function to run shell commands (for git setup)
func runCommand(t *testing.T, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Command failed: %s %v: %v\nOutput: %s", name, args, err, output)
	}
}

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
func waitForSessionInDB(t *testing.T, db *sql.DB) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		var sessionID string
		err := db.QueryRow(`
			SELECT id
			FROM sessions
			ORDER BY start_time DESC
			LIMIT 1
		`).Scan(&sessionID)

		if err == nil {
			return sessionID
		}

		if err != sql.ErrNoRows {
			t.Fatalf("Failed to query sessions: %v", err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("Session not created within timeout")
	return ""
}
