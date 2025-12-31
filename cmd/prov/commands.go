package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/albachteng/provenance/internal/daemon"
	"github.com/albachteng/provenance/internal/git"
	"github.com/albachteng/provenance/internal/storage"
)

// daemonStart starts the daemon in the background
func daemonStart() {
	socketPath := getSocketPath()

	if isDaemonRunning(socketPath) {
		fmt.Println("Daemon is already running")
		return
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(self, "daemon", "_run")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	for i := 0; i < 50; i++ {
		if isDaemonRunning(socketPath) {
			fmt.Println("Daemon started")
			fmt.Printf("Socket: %s\n", socketPath)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Fprintln(os.Stderr, "Daemon started but socket not ready within timeout")
	os.Exit(1)
}

// daemonStop stops the running daemon
func daemonStop() {
	socketPath := getSocketPath()

	if !isDaemonRunning(socketPath) {
		fmt.Println("Daemon is not running")
		return
	}

	pidFile := filepath.Join(getProvenanceHome(), "daemon.pid")
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read PID file: %v\n", err)
		os.Exit(1)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid PID in file: %v\n", err)
		os.Exit(1)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find process: %v\n", err)
		os.Exit(1)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send SIGTERM: %v\n", err)
		os.Exit(1)
	}

	for i := 0; i < 20; i++ {
		if !isDaemonRunning(socketPath) {
			fmt.Println("Daemon stopped")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Fprintln(os.Stderr, "Daemon did not stop within timeout")
	os.Exit(1)
}

// daemonStatus checks if the daemon is running
func daemonStatus() {
	socketPath := getSocketPath()

	if isDaemonRunning(socketPath) {
		fmt.Println("Daemon is running")
		fmt.Printf("Socket: %s\n", socketPath)
	} else {
		fmt.Println("Daemon is not running")
		os.Exit(1)
	}
}

// isDaemonRunning checks if the daemon is running by trying to connect to the socket
func isDaemonRunning(socketPath string) bool {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return false
	}
	conn.Close() //nolint:errcheck
	return true
}

// daemonRun runs the daemon process (hidden subcommand used by daemon start)
func daemonRun() {
	dbPath := getDBPath()
	socketPath := getSocketPath()
	pidFile := filepath.Join(getProvenanceHome(), "daemon.pid")

	if err := os.MkdirAll(getProvenanceHome(), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provenance home: %v\n", err)
		os.Exit(1)
	}

	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	pid := os.Getpid()
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write PID file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(pidFile) //nolint:errcheck

	daemon, err := daemon.NewDaemon(db, socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create daemon: %v\n", err)
		os.Exit(1)
	}

	if err := daemon.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
		os.Exit(1)
	}
}

// listEvents lists recent prompt events
func listEvents(limit int) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck

	// Get all events (we'll implement proper querying later)
	// For now, get events from all sessions
	rows, err := db.Query(`
		SELECT id, timestamp, session_id, agent, prompt_text, author
		FROM prompt_events
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	fmt.Printf("%-20s %-20s %-15s %s\n", "ID", "Timestamp", "Agent", "Prompt")
	fmt.Println(strings.Repeat("-", 100))

	count := 0
	for rows.Next() {
		var id, sessionID, agent, promptText, author string
		var timestamp int64

		if err := rows.Scan(&id, &timestamp, &sessionID, &agent, &promptText, &author); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		ts := time.Unix(timestamp, 0)

		if len(promptText) > 60 {
			promptText = promptText[:57] + "..."
		}

		fmt.Printf("%-20s %-20s %-15s %s\n",
			truncate(id, 20),
			ts.Format("2006-01-02 15:04:05"),
			truncate(agent, 15),
			promptText,
		)
		count++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	fmt.Printf("\nShowing %d event(s)\n", count)
	return nil
}

// showEvent shows detailed information about a specific event
func showEvent(eventID string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck

	event, err := storage.GetPromptEvent(db, eventID)
	if err != nil {
		if err == storage.ErrNotFound {
			return fmt.Errorf("event not found: %s", eventID)
		}
		return fmt.Errorf("failed to get event: %w", err)
	}

	fmt.Println("Event Details")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("ID:           %s\n", event.ID)
	fmt.Printf("Timestamp:    %s\n", event.Timestamp.Format(time.RFC3339))
	fmt.Printf("Session ID:   %s\n", event.SessionID)
	fmt.Printf("Agent:        %s\n", event.Agent)
	fmt.Printf("Model:        %s\n", event.ModelVersion)
	fmt.Printf("Author:       %s\n", event.Author)
	fmt.Printf("IDE:          %s\n", event.IDE)
	fmt.Printf("Repo:         %s\n", event.RepoPath)
	fmt.Printf("Git Commit:   %s\n", event.GitCommit)
	fmt.Printf("Git Branch:   %s\n", event.GitBranch)
	fmt.Printf("Git Dirty:    %v\n", event.GitDirty)
	fmt.Println()
	fmt.Println("Prompt:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println(event.PromptText)
	fmt.Println()

	if event.ResponseText != "" {
		fmt.Println("Response:")
		fmt.Println(strings.Repeat("-", 80))
		fmt.Println(event.ResponseText)
		fmt.Println()
	}

	if len(event.DirtyFiles) > 0 {
		fmt.Println("Dirty Files:")
		for _, file := range event.DirtyFiles {
			fmt.Printf("  %s\n", file)
		}
		fmt.Println()
	}

	return nil
}

// searchEvents searches for events matching a query
func searchEvents(query string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck

	rows, err := db.Query(`
		SELECT id, timestamp, session_id, agent, prompt_text
		FROM prompt_events
		WHERE prompt_text LIKE ? OR response_text LIKE ?
		ORDER BY timestamp DESC
		LIMIT 50
	`, "%"+query+"%", "%"+query+"%")
	if err != nil {
		return fmt.Errorf("failed to search events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	fmt.Printf("Search results for: %q\n", query)
	fmt.Printf("%-20s %-20s %-15s %s\n", "ID", "Timestamp", "Agent", "Prompt")
	fmt.Println(strings.Repeat("-", 100))

	count := 0
	for rows.Next() {
		var id, sessionID, agent, promptText string
		var timestamp int64

		if err := rows.Scan(&id, &timestamp, &sessionID, &agent, &promptText); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		ts := time.Unix(timestamp, 0)

		// Truncate prompt text to fit
		if len(promptText) > 60 {
			promptText = promptText[:57] + "..."
		}

		fmt.Printf("%-20s %-20s %-15s %s\n",
			truncate(id, 20),
			ts.Format("2006-01-02 15:04:05"),
			truncate(agent, 15),
			promptText,
		)
		count++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	fmt.Printf("\nFound %d event(s)\n", count)
	return nil
}

// openDB opens the database connection
func openDB() (*sql.DB, error) {
	dbPath := getDBPath()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database not initialized (run 'prov init' first)")
	}

	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return db, nil
}

// truncate truncates a string to a maximum length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// generateHook generates shell hook code for the specified shell
func generateHook(shell string) error {
	var template string

	switch shell {
	case "bash":
		template = bashHookTemplate
	case "zsh":
		template = zshHookTemplate
	default:
		return fmt.Errorf("shell '%s' not supported (supported: bash, zsh)", shell)
	}

	fmt.Println(template)
	return nil
}

const bashHookTemplate = `# AI Provenance shell hook for bash
# Add this to your ~/.bashrc: eval "$(prov hook bash)"

# Wrapper function for AI commands
function prov_ai_wrapper() {
    local agent="$1"
    shift
    prov wrap "$agent" "$@"
}

# Example aliases for common AI tools
# Uncomment and customize as needed:
# alias claude-code='prov_ai_wrapper claude-code claude-code'
# alias aider='prov_ai_wrapper aider aider'
`

const zshHookTemplate = `# AI Provenance shell hook for zsh
# Add this to your ~/.zshrc: eval "$(prov hook zsh)"

# Wrapper function for AI commands
function prov_ai_wrapper() {
    local agent="$1"
    shift
    prov wrap "$agent" "$@"
}

# Example aliases for common AI tools
# Uncomment and customize as needed:
# alias claude-code='prov_ai_wrapper claude-code claude-code'
# alias aider='prov_ai_wrapper aider aider'
`

// wrapCommand wraps a command execution and captures it for provenance tracking
func wrapCommand(agent, command string, args []string) int {
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	promptText := strings.Join(append([]string{command}, args...), " ")

	err := cmd.Run()

	sendEventToDaemon(agent, promptText)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	return 0
}

// sendEventToDaemon sends a prompt event to the running daemon
func sendEventToDaemon(agent, promptText string) {
	socketPath := getSocketPath()

	var gitCommit, gitBranch string
	var gitDirty bool
	gitInfo, err := git.CaptureGitState(".")
	if err == nil {
		gitCommit = gitInfo.Head
		gitBranch = gitInfo.Branch
		gitDirty = gitInfo.IsDirty
	}

	// Get current directory as repo path, fallback to "unknown" if unavailable
	repoPath, err := os.Getwd()
	if err != nil {
		repoPath = "unknown"
	}

	author := os.Getenv("USER")
	if author == "" {
		author = "unknown"
	}

	// Get or create a session for this repo
	// For now, use a simple session per repo (TODO: proper session management)
	sessionID := fmt.Sprintf("session-%s", filepath.Base(repoPath))

	// Send session start event first (on separate connection - daemon reads one message per connection)
	sessionEvent := struct {
		Type    string          `json:"type"`
		Session storage.Session `json:"session"`
	}{
		Type: "session_start",
		Session: storage.Session{
			ID:        sessionID,
			StartTime: time.Now(),
			RepoPath:  repoPath,
		},
	}
	sendMessageToDaemon(socketPath, sessionEvent)

	// Note: No "type" field - daemon routes based on absence of "type"
	event := storage.PromptEvent{
		ID:         generateEventID(),
		Timestamp:  time.Now(),
		SessionID:  sessionID,
		Agent:      agent,
		PromptText: promptText,
		RepoPath:   repoPath,
		GitCommit:  gitCommit,
		GitBranch:  gitBranch,
		GitDirty:   gitDirty,
		Author:     author,
		IDE:        "cli",
	}

	// Send the prompt event (on separate connection)
	sendMessageToDaemon(socketPath, event)
}

// sendMessageToDaemon sends a single JSON message to the daemon
func sendMessageToDaemon(socketPath string, message interface{}) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		// Daemon not running, silently skip
		return
	}
	defer conn.Close() //nolint:errcheck

	encoder := json.NewEncoder(conn)
	encoder.Encode(message) //nolint:errcheck
}

// generateEventID creates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt-%d-%d", time.Now().Unix(), time.Now().UnixNano()%1000000)
}

func installHooks(agent string) error {
	if agent != "claude-code" {
		return fmt.Errorf("agent '%s' not supported (supported: claude-code)", agent)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	hooksDir := filepath.Join(getProvenanceHome(), "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	scripts := map[string]string{
		"claude-prompt.py":     claudePromptHookScript,
		"claude-tool-pre.py":   claudeToolPreHookScript,
		"claude-tool-post.py":  claudeToolPostHookScript,
		"claude-session.py":    claudeSessionHookScript,
	}

	for name, content := range scripts {
		scriptPath := filepath.Join(hooksDir, name)
		if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
			return fmt.Errorf("failed to write hook script %s: %w", name, err)
		}
	}

	claudeDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")

	var settings map[string]interface{}
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read settings.json: %w", err)
		}
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(settingsData, &settings); err != nil {
			return fmt.Errorf("failed to parse settings.json: %w", err)
		}
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
		settings["hooks"] = hooks
	}

	promptScript := filepath.Join(hooksDir, "claude-prompt.py")
	toolPreScript := filepath.Join(hooksDir, "claude-tool-pre.py")
	toolPostScript := filepath.Join(hooksDir, "claude-tool-post.py")

	hooks["UserPromptSubmit"] = []interface{}{
		map[string]interface{}{
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": promptScript,
				},
			},
		},
	}

	hooks["PreToolUse"] = []interface{}{
		map[string]interface{}{
			"matcher": "*",
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": toolPreScript,
				},
			},
		},
	}

	hooks["PostToolUse"] = []interface{}{
		map[string]interface{}{
			"matcher": "*",
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": toolPostScript,
				},
			},
		},
	}

	updatedSettings, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, updatedSettings, 0644); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	fmt.Println("Claude Code hooks installed successfully")
	fmt.Printf("Hook scripts: %s\n", hooksDir)
	fmt.Printf("Settings updated: %s\n", settingsPath)

	return nil
}

func captureHook() error {
	var hookInput map[string]interface{}
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&hookInput); err != nil {
		return fmt.Errorf("failed to decode JSON from stdin: %w", err)
	}

	eventName, _ := hookInput["hook_event_name"].(string)
	sessionID, _ := hookInput["session_id"].(string)
	cwd, _ := hookInput["cwd"].(string)

	if cwd == "" {
		cwd, _ = os.Getwd()
		if cwd == "" {
			cwd = "unknown"
		}
	}

	var promptText string
	var gitCommit, gitBranch string
	var gitDirty bool

	gitInfo, err := git.CaptureGitState(cwd)
	if err == nil {
		gitCommit = gitInfo.Head
		gitBranch = gitInfo.Branch
		gitDirty = gitInfo.IsDirty
	}

	switch eventName {
	case "UserPromptSubmit":
		promptText, _ = hookInput["prompt"].(string)
	case "PreToolUse", "PostToolUse":
		toolName, _ := hookInput["tool_name"].(string)
		toolInput, _ := hookInput["tool_input"].(map[string]interface{})
		toolInputJSON, _ := json.Marshal(toolInput)
		promptText = fmt.Sprintf("%s: %s", toolName, string(toolInputJSON))
	default:
		promptText = eventName
	}

	author := os.Getenv("USER")
	if author == "" {
		author = "unknown"
	}

	if sessionID == "" {
		sessionID = fmt.Sprintf("session-%s", filepath.Base(cwd))
	}

	socketPath := getSocketPath()

	sessionEvent := struct {
		Type    string          `json:"type"`
		Session storage.Session `json:"session"`
	}{
		Type: "session_start",
		Session: storage.Session{
			ID:        sessionID,
			StartTime: time.Now(),
			RepoPath:  cwd,
		},
	}
	sendMessageToDaemon(socketPath, sessionEvent)

	event := storage.PromptEvent{
		ID:         generateEventID(),
		Timestamp:  time.Now(),
		SessionID:  sessionID,
		Agent:      "claude-code",
		PromptText: promptText,
		RepoPath:   cwd,
		GitCommit:  gitCommit,
		GitBranch:  gitBranch,
		GitDirty:   gitDirty,
		Author:     author,
		IDE:        "claude-code",
	}

	sendMessageToDaemon(socketPath, event)

	return nil
}

func hooksStatus() error {
	hooksDir := filepath.Join(getProvenanceHome(), "hooks")

	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No hooks installed")
			return nil
		}
		return fmt.Errorf("failed to read hooks directory: %w", err)
	}

	claudeHooks := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "claude-") && strings.HasSuffix(entry.Name(), ".py") {
			claudeHooks = append(claudeHooks, entry.Name())
		}
	}

	if len(claudeHooks) == 0 {
		fmt.Println("No hooks installed")
		return nil
	}

	fmt.Println("Installed hooks:")
	fmt.Println()
	fmt.Println("claude-code:")
	for _, hook := range claudeHooks {
		fmt.Printf("  - %s\n", hook)
	}

	return nil
}

func uninstallHooks(agent string) error {
	if agent != "claude-code" {
		return fmt.Errorf("agent '%s' not supported (supported: claude-code)", agent)
	}

	hooksDir := filepath.Join(getProvenanceHome(), "hooks")

	scripts := []string{
		"claude-prompt.py",
		"claude-tool-pre.py",
		"claude-tool-post.py",
		"claude-session.py",
	}

	for _, script := range scripts {
		scriptPath := filepath.Join(hooksDir, script)
		if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove hook script %s: %w", script, err)
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Claude Code hooks uninstalled")
			return nil
		}
		return fmt.Errorf("failed to read settings.json: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return fmt.Errorf("failed to parse settings.json: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if ok {
		delete(hooks, "UserPromptSubmit")
		delete(hooks, "PreToolUse")
		delete(hooks, "PostToolUse")
	}

	updatedSettings, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, updatedSettings, 0644); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	fmt.Println("Claude Code hooks uninstalled")

	return nil
}

const claudePromptHookScript = `#!/usr/bin/env python3
import json
import sys
import subprocess

hook_input = json.load(sys.stdin)

event = {
    'hook_event_name': hook_input.get('hook_event_name'),
    'session_id': hook_input.get('session_id'),
    'prompt': hook_input.get('prompt'),
    'cwd': hook_input.get('cwd'),
    'permission_mode': hook_input.get('permission_mode'),
    'transcript_path': hook_input.get('transcript_path'),
}

try:
    subprocess.run(
        ['prov', 'capture-hook', '--json'],
        input=json.dumps(event),
        text=True,
        check=False,
        capture_output=True
    )
except Exception:
    pass

sys.exit(0)
`

const claudeToolPreHookScript = `#!/usr/bin/env python3
import json
import sys
import subprocess

hook_input = json.load(sys.stdin)

event = {
    'hook_event_name': 'PreToolUse',
    'session_id': hook_input.get('session_id'),
    'tool_name': hook_input.get('tool_name'),
    'tool_use_id': hook_input.get('tool_use_id'),
    'tool_input': hook_input.get('tool_input'),
    'cwd': hook_input.get('cwd'),
    'permission_mode': hook_input.get('permission_mode'),
}

try:
    subprocess.run(
        ['prov', 'capture-hook', '--json'],
        input=json.dumps(event),
        text=True,
        check=False,
        capture_output=True
    )
except Exception:
    pass

sys.exit(0)
`

const claudeToolPostHookScript = `#!/usr/bin/env python3
import json
import sys
import subprocess

hook_input = json.load(sys.stdin)

event = {
    'hook_event_name': 'PostToolUse',
    'session_id': hook_input.get('session_id'),
    'tool_name': hook_input.get('tool_name'),
    'tool_use_id': hook_input.get('tool_use_id'),
    'tool_input': hook_input.get('tool_input'),
    'tool_response': hook_input.get('tool_response'),
    'cwd': hook_input.get('cwd'),
}

try:
    subprocess.run(
        ['prov', 'capture-hook', '--json'],
        input=json.dumps(event),
        text=True,
        check=False,
        capture_output=True
    )
except Exception:
    pass

sys.exit(0)
`

const claudeSessionHookScript = `#!/usr/bin/env python3
import json
import sys

sys.exit(0)
`
