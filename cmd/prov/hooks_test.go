package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallHooksClaudeCode tests installing Claude Code hooks
func TestInstallHooksClaudeCode(t *testing.T) {
	tmpDir := setupTestEnv(t)

	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	output, err := runCLI(t, "install-hooks", "claude-code")
	if err != nil {
		t.Fatalf("install-hooks failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Claude Code hooks installed") {
		t.Errorf("Expected success message, got: %s", output)
	}

	hooksDir := filepath.Join(tmpDir, "hooks")
	expectedScripts := []string{
		"claude-prompt.py",
		"claude-tool-pre.py",
		"claude-tool-post.py",
		"claude-session.py",
	}

	for _, script := range expectedScripts {
		scriptPath := filepath.Join(hooksDir, script)
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			t.Errorf("Hook script not created: %s", scriptPath)
		}

		info, err := os.Stat(scriptPath)
		if err == nil {
			mode := info.Mode()
			if mode&0111 == 0 {
				t.Errorf("Hook script not executable: %s", scriptPath)
			}
		}
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("Claude settings.json not created")
	}

	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("Failed to parse settings.json: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("Settings missing 'hooks' section")
	}

	if _, ok := hooks["UserPromptSubmit"]; !ok {
		t.Error("Settings missing 'UserPromptSubmit' hook")
	}

	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("Settings missing 'PreToolUse' hook")
	}

	if _, ok := hooks["PostToolUse"]; !ok {
		t.Error("Settings missing 'PostToolUse' hook")
	}
}

// TestInstallHooksExistingSettings tests installing hooks when settings.json already exists
func TestInstallHooksExistingSettings(t *testing.T) {
	tmpDir := setupTestEnv(t)

	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	existingSettings := map[string]interface{}{
		"someExistingSetting": "value",
		"hooks": map[string]interface{}{
			"SomeOtherHook": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "/some/other/hook.sh",
						},
					},
				},
			},
		},
	}

	settingsData, _ := json.MarshalIndent(existingSettings, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, settingsData, 0644); err != nil {
		t.Fatalf("Failed to write existing settings: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	output, err := runCLI(t, "install-hooks", "claude-code")
	if err != nil {
		t.Fatalf("install-hooks failed: %v", err)
	}

	if !strings.Contains(output, "Claude Code hooks installed") {
		t.Errorf("Expected success message, got: %s", output)
	}

	updatedData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read updated settings: %v", err)
	}

	var updated map[string]interface{}
	if err := json.Unmarshal(updatedData, &updated); err != nil {
		t.Fatalf("Failed to parse updated settings: %v", err)
	}

	if updated["someExistingSetting"] != "value" {
		t.Error("Existing settings were not preserved")
	}

	hooks := updated["hooks"].(map[string]interface{})
	if _, ok := hooks["UserPromptSubmit"]; !ok {
		t.Error("UserPromptSubmit hook not added")
	}

	if _, ok := hooks["SomeOtherHook"]; !ok {
		t.Error("Existing hook was removed")
	}
}

// TestCaptureHookUserPrompt tests capturing a UserPromptSubmit hook event
func TestCaptureHookUserPrompt(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer runCLI(t, "daemon", "stop")

	waitForDaemonReady(t, tmpDir)

	hookInput := map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"session_id":      "ses-test-123",
		"prompt":          "Implement authentication for the API",
		"cwd":             "/home/user/project",
		"permission_mode": "auto",
		"transcript_path": "/path/to/transcript.jsonl",
	}

	inputJSON, _ := json.Marshal(hookInput)

	cmd := buildCLICommand(t, "capture-hook", "--json")
	cmd.Stdin = bytes.NewReader(inputJSON)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capture-hook failed: %v\nOutput: %s", err, output)
	}

	waitForEventInDB(t, db, func(agent, promptText string) bool {
		return agent == "claude-code" && strings.Contains(promptText, "Implement authentication")
	})

	rows, err := db.Query(`
		SELECT agent, prompt_text, session_id, ide
		FROM prompt_events
		ORDER BY timestamp DESC
		LIMIT 1
	`)
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("Expected event in database")
	}

	var agent, promptText, sessionID, ide string
	if err := rows.Scan(&agent, &promptText, &sessionID, &ide); err != nil {
		t.Fatalf("Failed to scan event: %v", err)
	}

	if agent != "claude-code" {
		t.Errorf("Expected agent 'claude-code', got: %s", agent)
	}

	if !strings.Contains(promptText, "Implement authentication") {
		t.Errorf("Expected prompt text, got: %s", promptText)
	}

	if sessionID != "ses-test-123" {
		t.Errorf("Expected session_id 'ses-test-123', got: %s", sessionID)
	}

	if ide != "claude-code" {
		t.Errorf("Expected ide 'claude-code', got: %s", ide)
	}
}

// TestCaptureHookToolUse tests capturing PreToolUse and PostToolUse events
func TestCaptureHookToolUse(t *testing.T) {
	tmpDir := setupTestEnv(t)
	db := setupTestDB(t, tmpDir)
	defer db.Close()

	_, err := runCLI(t, "daemon", "start")
	if err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer runCLI(t, "daemon", "stop")

	waitForDaemonReady(t, tmpDir)

	preToolInput := map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"session_id":      "ses-test-456",
		"tool_name":       "Edit",
		"tool_use_id":     "tool-123",
		"tool_input": map[string]interface{}{
			"file_path":  "/home/user/project/main.go",
			"old_string": "func old()",
			"new_string": "func new()",
		},
		"cwd":             "/home/user/project",
		"permission_mode": "auto",
	}

	inputJSON, _ := json.Marshal(preToolInput)

	cmd := buildCLICommand(t, "capture-hook", "--json")
	cmd.Stdin = bytes.NewReader(inputJSON)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("capture-hook PreToolUse failed: %v\nOutput: %s", err, output)
	}

	waitForEventInDB(t, db, func(agent, promptText string) bool {
		return agent == "claude-code" && strings.Contains(promptText, "Edit")
	})

	postToolInput := map[string]interface{}{
		"hook_event_name": "PostToolUse",
		"session_id":      "ses-test-456",
		"tool_name":       "Edit",
		"tool_use_id":     "tool-123",
		"tool_input": map[string]interface{}{
			"file_path": "/home/user/project/main.go",
		},
		"tool_response": map[string]interface{}{
			"status":  "success",
			"message": "File edited successfully",
		},
		"cwd": "/home/user/project",
	}

	postJSON, _ := json.Marshal(postToolInput)

	cmd = buildCLICommand(t, "capture-hook", "--json")
	cmd.Stdin = bytes.NewReader(postJSON)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("capture-hook PostToolUse failed: %v\nOutput: %s", err, output)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prompt_events`).Scan(&count); err != nil {
		t.Fatalf("Failed to count events: %v", err)
	}

	if count < 2 {
		t.Errorf("Expected at least 2 events, got: %d", count)
	}
}

// TestHooksStatus tests the hooks status command
func TestHooksStatus(t *testing.T) {
	tmpDir := setupTestEnv(t)

	hooksDir := filepath.Join(tmpDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks dir: %v", err)
	}

	hookScripts := []string{"claude-prompt.py", "claude-tool-pre.py"}
	for _, script := range hookScripts {
		scriptPath := filepath.Join(hooksDir, script)
		if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env python3\n"), 0755); err != nil {
			t.Fatalf("Failed to create hook script: %v", err)
		}
	}

	output, err := runCLI(t, "hooks", "status")
	if err != nil {
		t.Fatalf("hooks status failed: %v", err)
	}

	if !strings.Contains(output, "claude-code") {
		t.Errorf("Expected 'claude-code' in status output, got: %s", output)
	}

	for _, script := range hookScripts {
		if !strings.Contains(output, script) {
			t.Errorf("Expected '%s' in status output, got: %s", script, output)
		}
	}
}

// TestHooksStatusNoHooks tests status when no hooks are installed
func TestHooksStatusNoHooks(t *testing.T) {
	setupTestEnv(t)

	output, err := runCLI(t, "hooks", "status")
	if err != nil {
		t.Fatalf("hooks status failed: %v", err)
	}

	if !strings.Contains(output, "No hooks installed") {
		t.Errorf("Expected 'No hooks installed' message, got: %s", output)
	}
}

// TestUninstallHooks tests uninstalling Claude Code hooks
func TestUninstallHooks(t *testing.T) {
	tmpDir := setupTestEnv(t)

	hooksDir := filepath.Join(tmpDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks dir: %v", err)
	}

	hookScripts := []string{
		"claude-prompt.py",
		"claude-tool-pre.py",
		"claude-tool-post.py",
		"claude-session.py",
	}

	for _, script := range hookScripts {
		scriptPath := filepath.Join(hooksDir, script)
		if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env python3\n"), 0755); err != nil {
			t.Fatalf("Failed to create hook script: %v", err)
		}
	}

	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"UserPromptSubmit": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": filepath.Join(hooksDir, "claude-prompt.py"),
						},
					},
				},
			},
		},
	}

	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, settingsData, 0644); err != nil {
		t.Fatalf("Failed to write settings: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	output, err := runCLI(t, "hooks", "uninstall", "claude-code")
	if err != nil {
		t.Fatalf("hooks uninstall failed: %v", err)
	}

	if !strings.Contains(output, "Claude Code hooks uninstalled") {
		t.Errorf("Expected success message, got: %s", output)
	}

	for _, script := range hookScripts {
		scriptPath := filepath.Join(hooksDir, script)
		if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
			t.Errorf("Hook script still exists: %s", scriptPath)
		}
	}

	updatedData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read updated settings: %v", err)
	}

	var updated map[string]interface{}
	if err := json.Unmarshal(updatedData, &updated); err != nil {
		t.Fatalf("Failed to parse updated settings: %v", err)
	}

	hooks, ok := updated["hooks"].(map[string]interface{})
	if ok && len(hooks) > 0 {
		if _, exists := hooks["UserPromptSubmit"]; exists {
			t.Error("UserPromptSubmit hook still in settings")
		}
	}
}

// TestInstallHooksUnsupportedAgent tests installing hooks for unsupported agent
func TestInstallHooksUnsupportedAgent(t *testing.T) {
	setupTestEnv(t)

	output, err := runCLI(t, "install-hooks", "unsupported-agent")
	if err == nil {
		t.Error("Expected error for unsupported agent")
	}

	if !strings.Contains(output, "not supported") && !strings.Contains(output, "unknown") {
		t.Errorf("Expected error message about unsupported agent, got: %s", output)
	}
}

// TestInstallHooksEmbedProvPath tests that hook scripts contain full path to prov binary
func TestInstallHooksEmbedProvPath(t *testing.T) {
	tmpDir := setupTestEnv(t)

	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	_, err := runCLI(t, "install-hooks", "claude-code")
	if err != nil {
		t.Fatalf("install-hooks failed: %v", err)
	}

	hooksDir := filepath.Join(tmpDir, "hooks")

	// Check that each hook script contains the full path to prov
	hookScripts := []string{
		"claude-prompt.py",
		"claude-tool-pre.py",
		"claude-tool-post.py",
	}

	for _, script := range hookScripts {
		scriptPath := filepath.Join(hooksDir, script)
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("Failed to read hook script %s: %v", script, err)
		}

		scriptContent := string(content)

		// Verify the script contains an absolute path (starts with /)
		// In test environment this will be the test binary path
		if !strings.Contains(scriptContent, "subprocess.run(\n        ['/") &&
			!strings.Contains(scriptContent, "subprocess.run(\n        [\"/") {
			t.Errorf("Hook script %s does not contain absolute path to prov binary\nContent: %s",
				script, scriptContent)
		}

		// Verify it's NOT using just "prov" without a path
		if strings.Contains(scriptContent, "['prov',") || strings.Contains(scriptContent, "[\"prov\",") {
			t.Errorf("Hook script %s still contains hardcoded 'prov' instead of full path\nContent: %s",
				script, scriptContent)
		}
	}
}

// buildCLICommand creates an exec.Cmd for the CLI tool (helper for stdin tests)
func buildCLICommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(testBinary, args...)
	return cmd
}
