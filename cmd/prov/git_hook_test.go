package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInstallPostCommitHook tests installing the post-commit git hook
func TestInstallPostCommitHook(t *testing.T) {
	// Create a test git repository
	repoPath := setupGitRepo(t)

	// Run install command
	output, err := runProvCommand(t, repoPath, "install-hook", "post-commit")
	if err != nil {
		t.Fatalf("install-hook failed: %v\nOutput: %s", err, output)
	}

	// Verify hook script was created
	hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Errorf("Hook script not created at %s", hookPath)
	}

	// Verify hook is executable
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("Failed to stat hook: %v", err)
	}

	if info.Mode()&0111 == 0 {
		t.Error("Hook script is not executable")
	}

	// Verify hook content includes prov path
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("Failed to read hook: %v", err)
	}

	hookContent := string(content)

	// Should contain shebang
	if !strings.HasPrefix(hookContent, "#!/") {
		t.Error("Hook should start with shebang")
	}

	// Should reference prov binary
	if !strings.Contains(hookContent, "prov") {
		t.Error("Hook should reference prov binary")
	}

	// Should call correlation command
	if !strings.Contains(hookContent, "correlate-commit") || !strings.Contains(hookContent, "post-commit") {
		t.Error("Hook should call prov correlate-commit or post-commit command")
	}
}

// TestInstallPostCommitHookNotInGitRepo tests error handling when not in a git repo
func TestInstallPostCommitHookNotInGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	output, err := runProvCommand(t, tmpDir, "install-hook", "post-commit")
	if err == nil {
		t.Error("Expected error when not in git repository")
	}

	if !strings.Contains(output, "not a git repository") && !strings.Contains(output, "git") {
		t.Errorf("Error message should mention git repository, got: %s", output)
	}
}

// TestInstallPostCommitHookOverwrite tests that reinstalling overwrites existing hook
func TestInstallPostCommitHookOverwrite(t *testing.T) {
	repoPath := setupGitRepo(t)

	// Create a dummy hook first
	hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")
	err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho 'old hook'\n"), 0755)
	if err != nil {
		t.Fatalf("Failed to create dummy hook: %v", err)
	}

	// Install prov hook (should overwrite)
	output, err := runProvCommand(t, repoPath, "install-hook", "post-commit")
	if err != nil {
		t.Fatalf("install-hook failed: %v\nOutput: %s", err, output)
	}

	// Verify it was overwritten
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("Failed to read hook: %v", err)
	}

	if strings.Contains(string(content), "old hook") {
		t.Error("Hook was not overwritten")
	}

	if !strings.Contains(string(content), "prov") {
		t.Error("New hook should contain prov reference")
	}
}

// TestPostCommitHookExecution tests that the hook executes correctly
func TestPostCommitHookExecution(t *testing.T) {
	t.Skip("Skipping integration test - requires 'blame' command implementation")

	// Start daemon for this test
	tmpDir := t.TempDir()
	os.Setenv("AI_PROVENANCE_HOME", tmpDir)
	defer os.Unsetenv("AI_PROVENANCE_HOME")

	// Create test repo
	repoPath := setupGitRepo(t)

	// Build prov binary
	provPath := buildProvBinary(t)

	// Install hook using the built binary
	cmd := exec.Command(provPath, "install-hook", "post-commit")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-hook failed: %v\nOutput: %s", err, output)
	}

	// Start daemon
	cmd = exec.Command(provPath, "daemon", "start")
	cmd.Env = append(os.Environ(), "AI_PROVENANCE_HOME="+tmpDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("daemon start failed: %v\nOutput: %s", err, output)
	}
	defer func() {
		cmd := exec.Command(provPath, "daemon", "stop")
		cmd.Env = append(os.Environ(), "AI_PROVENANCE_HOME="+tmpDir)
		cmd.Run()
	}()

	// Create a prompt event (so hook has something to correlate)
	// This would normally come from Claude Code hooks, but we'll create it directly
	createTestPromptEvent(t, tmpDir, repoPath)

	// Make a commit (triggers hook)
	createCommitInRepo(t, repoPath, "test.txt", "test content", "Test commit")

	// Wait a bit for hook to execute
	waitForCondition(t, func() bool {
		// Check if change_sets were created
		cmd := exec.Command(provPath, "blame", "HEAD")
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(), "AI_PROVENANCE_HOME="+tmpDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return false
		}
		return len(output) > 0
	}, 5000, "change sets created")
}

// TestUninstallPostCommitHook tests removing the hook
func TestUninstallPostCommitHook(t *testing.T) {
	repoPath := setupGitRepo(t)

	// Install hook first
	_, err := runProvCommand(t, repoPath, "install-hook", "post-commit")
	if err != nil {
		t.Fatalf("install-hook failed: %v", err)
	}

	hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")

	// Verify it exists
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Fatal("Hook was not installed")
	}

	// Uninstall
	output, err := runProvCommand(t, repoPath, "hooks", "uninstall", "post-commit")
	if err != nil {
		t.Fatalf("hooks uninstall failed: %v\nOutput: %s", err, output)
	}

	// Verify it was removed
	if _, err := os.Stat(hookPath); err == nil {
		t.Error("Hook was not removed")
	}
}

// TestHooksStatusPostCommit tests that post-commit hook shows in status
func TestHooksStatusPostCommit(t *testing.T) {
	repoPath := setupGitRepo(t)

	// Install hook
	_, err := runProvCommand(t, repoPath, "install-hook", "post-commit")
	if err != nil {
		t.Fatalf("install-hook failed: %v", err)
	}

	// Check status
	output, err := runProvCommand(t, repoPath, "hooks", "status")
	if err != nil {
		t.Fatalf("hooks status failed: %v\nOutput: %s", err, output)
	}

	// Should mention post-commit hook
	if !strings.Contains(output, "post-commit") {
		t.Errorf("hooks status should mention post-commit hook, got: %s", output)
	}

	// Should show it's installed
	if !strings.Contains(output, "git") || !strings.Contains(output, "post-commit") {
		t.Errorf("hooks status should show git hook status, got: %s", output)
	}
}

// Helper functions

func setupGitRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	runGitCmd(t, tmpDir, "init")
	runGitCmd(t, tmpDir, "config", "user.email", "test@example.com")
	runGitCmd(t, tmpDir, "config", "user.name", "Test User")

	// Create initial commit
	createCommitInRepo(t, tmpDir, "README.md", "# Test", "Initial commit")

	return tmpDir
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, output)
	}
}

func createCommitInRepo(t *testing.T, repoPath, filename, content, message string) {
	t.Helper()

	filePath := filepath.Join(repoPath, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	runGitCmd(t, repoPath, "add", filename)
	runGitCmd(t, repoPath, "commit", "-m", message)
}

func runProvCommand(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()

	provPath := buildProvBinary(t)

	cmd := exec.Command(provPath, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()

	return string(output), err
}

func buildProvBinary(t *testing.T) string {
	t.Helper()

	// Build prov binary if not already built
	tmpDir := t.TempDir()
	provPath := filepath.Join(tmpDir, "prov")

	cmd := exec.Command("go", "build", "-o", provPath, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build prov: %v\nOutput: %s", err, output)
	}

	return provPath
}

func createTestPromptEvent(t *testing.T, provenanceHome, repoPath string) {
	t.Helper()

	// This would use the actual daemon socket in a real scenario
	// For now, we'll just note that the test would need to create
	// a prompt event in the database that the hook can find
	// The implementation will handle this via the daemon
}

func waitForCondition(t *testing.T, check func() bool, timeoutMs int, message string) {
	t.Helper()

	// Implementation from existing tests
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Condition '%s' not met within %dms", message, timeoutMs)
}
