package main

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/albachteng/provenance/internal/config"
	"github.com/albachteng/provenance/internal/daemon"
	"github.com/albachteng/provenance/internal/git"
	"github.com/albachteng/provenance/internal/storage"
	"gopkg.in/yaml.v3"
)

// Embedded hook script templates
//
//go:embed hooks/claude-prompt.py
var claudePromptTemplate string

//go:embed hooks/claude-tool-pre.py
var claudeToolPreTemplate string

//go:embed hooks/claude-tool-post.py
var claudeToolPostTemplate string

//go:embed hooks/post-commit.sh
var postCommitHookTemplate string

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

// findGitRoot finds the root of the current git repository
// Returns empty string and error if not in a git repository
func findGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// daemonRun runs the daemon process (hidden subcommand used by daemon start)
func daemonRun() {
	provenanceHome := getProvenanceHome()

	if err := os.MkdirAll(provenanceHome, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provenance home: %v\n", err)
		os.Exit(1)
	}

	// Try to find git root, but it's OK if we're not in a repo (will use global config only)
	repoPath, _ := findGitRoot() //nolint:errcheck
	cfg, err := config.Load(provenanceHome, repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	db, err := storage.InitDatabase(cfg.Storage.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	pid := os.Getpid()
	if err := os.WriteFile(cfg.Daemon.PIDFile, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write PID file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(cfg.Daemon.PIDFile) //nolint:errcheck

	daemon, err := daemon.NewDaemon(db, cfg.Daemon.SocketPath)
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

	// TODO: implement proper querying (by session, agent, user, project, etc)
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

	repoPath, err := os.Getwd()
	if err != nil {
		repoPath = "unknown"
	}

	author := os.Getenv("USER")
	if author == "" {
		author = "unknown"
	}

	event := storage.PromptEvent{
		ID:              generateEventID(),
		Timestamp:       time.Now(),
		SessionID:       "", // V2: nullable, not used
		Agent:           agent,
		PromptText:      promptText,
		RepoPath:        repoPath,
		GitCommit:       gitCommit,
		GitBranch:       gitBranch,
		GitDirty:        gitDirty,
		Author:          author,
		IDE:             "cli",
		BranchAtCapture: gitBranch,
		PreBranchSwitch: false,
	}

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

// installs hooks in supported agents' configurations (currently only supports claude-code)
func installHooks(agent string) error {
	if agent != "claude-code" {
		return fmt.Errorf("agent '%s' not supported (supported: claude-code)", agent)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	provPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	hooksDir := filepath.Join(getProvenanceHome(), "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	scripts := map[string]string{
		"claude-prompt.py":    strings.ReplaceAll(claudePromptTemplate, "{{PROV_PATH}}", provPath),
		"claude-tool-pre.py":  strings.ReplaceAll(claudeToolPreTemplate, "{{PROV_PATH}}", provPath),
		"claude-tool-post.py": strings.ReplaceAll(claudeToolPostTemplate, "{{PROV_PATH}}", provPath),
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
	cwd, _ := hookInput["cwd"].(string)

	if cwd == "" {
		if dir, err := os.Getwd(); err == nil {
			cwd = dir
		} else {
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
		toolInputJSON, err := json.Marshal(toolInput)
		if err != nil {
			promptText = fmt.Sprintf("%s: <marshal error>", toolName)
		} else {
			promptText = fmt.Sprintf("%s: %s", toolName, string(toolInputJSON))
		}
	default:
		promptText = eventName
	}

	author := os.Getenv("USER")
	if author == "" {
		author = "unknown"
	}

	socketPath := getSocketPath()

	event := storage.PromptEvent{
		ID:              generateEventID(),
		Timestamp:       time.Now(),
		SessionID:       "", // V2: nullable, not used
		Agent:           "claude-code",
		PromptText:      promptText,
		RepoPath:        cwd,
		GitCommit:       gitCommit,
		GitBranch:       gitBranch,
		GitDirty:        gitDirty,
		Author:          author,
		IDE:             "claude-code",
		BranchAtCapture: gitBranch,
		PreBranchSwitch: false,
	}

	sendMessageToDaemon(socketPath, event)

	return nil
}

func hooksStatus() error {
	hooksDir := filepath.Join(getProvenanceHome(), "hooks")

	entries, err := os.ReadDir(hooksDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read hooks directory: %w", err)
	}

	claudeHooks := []string{}
	if err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "claude-") && strings.HasSuffix(entry.Name(), ".py") {
				claudeHooks = append(claudeHooks, entry.Name())
			}
		}
	}

	// Check for git hooks
	gitHooks := map[string]bool{}
	repoPath, err := findGitRoot()
	if err == nil {
		// Check for post-commit hook
		postCommitPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")
		if _, err := os.Stat(postCommitPath); err == nil {
			// Read hook to verify it's our hook (contains "correlate-commit")
			content, err := os.ReadFile(postCommitPath)
			if err == nil && strings.Contains(string(content), "correlate-commit") {
				gitHooks["post-commit"] = true
			}
		}
	}

	if len(claudeHooks) == 0 && len(gitHooks) == 0 {
		fmt.Println("No hooks installed")
		return nil
	}

	fmt.Println("Installed hooks:")
	fmt.Println()

	if len(claudeHooks) > 0 {
		fmt.Println("claude-code:")
		for _, hook := range claudeHooks {
			fmt.Printf("  - %s\n", hook)
		}
		fmt.Println()
	}

	if len(gitHooks) > 0 {
		fmt.Println("git:")
		for hook := range gitHooks {
			fmt.Printf("  - %s\n", hook)
		}
	}

	return nil
}

func uninstallHooks(agent string) error {
	// Check if this is a git hook type
	if agent == "post-commit" {
		return uninstallGitHook(agent)
	}

	if agent != "claude-code" {
		return fmt.Errorf("agent '%s' not supported (supported: claude-code, post-commit)", agent)
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

// installGitHook installs a git hook in the current repository
func installGitHook(hookType string) error {
	if hookType != "post-commit" {
		return fmt.Errorf("hook type '%s' not supported (supported: post-commit)", hookType)
	}

	// Find git repository root
	repoPath, err := findGitRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	// Get prov binary path
	provPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Inject prov path into hook template
	hookContent := strings.ReplaceAll(postCommitHookTemplate, "{{PROV_PATH}}", provPath)

	// Write hook script
	hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")
	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook script: %w", err)
	}

	fmt.Printf("Git post-commit hook installed: %s\n", hookPath)
	fmt.Println("Hook will automatically correlate commits with recent prompts")

	return nil
}

// uninstallGitHook removes a git hook from the current repository
func uninstallGitHook(hookType string) error {
	if hookType != "post-commit" {
		return fmt.Errorf("hook type '%s' not supported (supported: post-commit)", hookType)
	}

	// Find git repository root
	repoPath, err := findGitRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	hookPath := filepath.Join(repoPath, ".git", "hooks", "post-commit")

	if err := os.Remove(hookPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Git post-commit hook not installed")
			return nil
		}
		return fmt.Errorf("failed to remove hook: %w", err)
	}

	fmt.Println("Git post-commit hook uninstalled")
	return nil
}

// correlateCommit is a stub for v2 architecture
// V2 uses commit windows instead of pre-computed correlations
// Correlation is now done on-demand via blame commands
func correlateCommit(commitSHA, repoPath string) error {
	// V2: Commit window caching will be implemented later
	// For now, this is a no-op - blame queries compute associations on-demand
	return nil
}

func configShow() {
	cfg, err := getConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("# Current configuration (merged from all sources)")
	fmt.Println(string(data))
}

func configInit(global bool) {
	var configPath string

	if global {
		provenanceHome := getProvenanceHome()
		configPath = filepath.Join(provenanceHome, "config.yaml")
	} else {
		repoPath, err := findGitRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Not in a git repository. Use --global for global config.")
			os.Exit(1)
		}

		configDir := filepath.Join(repoPath, ".ai-provenance")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create config directory: %v\n", err)
			os.Exit(1)
		}
		configPath = filepath.Join(configDir, "config.yaml")
	}

	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(os.Stderr, "Config file already exists: %s\n", configPath)
		fmt.Fprintln(os.Stderr, "Remove it first if you want to reinitialize.")
		os.Exit(1)
	}

	cfg := config.Default()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal default config: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created config file: %s\n", configPath)
}

func configValidate() {
	cfg, err := getConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration is invalid: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Configuration is valid")
	fmt.Printf("Database path: %s\n", cfg.Storage.DBPath)
	fmt.Printf("Socket path: %s\n", cfg.Daemon.SocketPath)
}

// Session command implementations (V2: DEPRECATED - sessions removed)

func cmdSession() {
	fmt.Fprintln(os.Stderr, "Session commands have been removed in v2 architecture")
	fmt.Fprintln(os.Stderr, "V2 uses commit windows instead of sessions")
	fmt.Fprintln(os.Stderr, "Use 'prov blame <commit>' to see prompts for a commit")
	os.Exit(1)
}

// sessionList is deprecated in v2
func sessionList() {
	activeOnly := false
	for _, arg := range os.Args[3:] {
		if arg == "--active" {
			activeOnly = true
			break
		}
	}

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	sessions, err := storage.ListSessions(db, activeOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list sessions: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		if activeOnly {
			fmt.Println("No active sessions")
		} else {
			fmt.Println("No sessions found")
		}
		return
	}

	fmt.Printf("%-12s %-20s %-10s %-8s %-20s %s\n", "ID", "Start Time", "Duration", "Prompts", "Repo", "Status")
	fmt.Println(strings.Repeat("-", 100))

	for _, s := range sessions {
		startTime := s.StartTime.Format("2006-01-02 15:04:05")

		var duration string
		var status string
		if s.EndTime == nil {
			duration = formatDuration(time.Since(s.StartTime))
			status = "Active"
		} else {
			duration = formatDuration(s.EndTime.Sub(s.StartTime))
			if s.EndedBy != "" {
				status = fmt.Sprintf("Ended (%s)", s.EndedBy)
			} else {
				status = "Ended"
			}
		}

		repoName := filepath.Base(s.RepoPath)
		if repoName == "." || repoName == "/" {
			repoName = s.RepoPath
		}

		displayID := s.ID
		if len(displayID) > 12 {
			displayID = displayID[:12]
		}

		fmt.Printf("%-12s %-20s %-10s %-8d %-20s %s\n",
			displayID, startTime, duration, s.TotalPrompts, repoName, status)
	}

	fmt.Printf("\nShowing %d session(s)\n", len(sessions))
}

func sessionShow(sessionID string) {
	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	session, err := storage.GetSession(db, sessionID)
	if err != nil {
		if err == storage.ErrNotFound {
			fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionID)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Failed to get session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Session ID: %s\n", session.ID)
	fmt.Printf("Repository: %s\n", session.RepoPath)
	fmt.Printf("Start Time: %s\n", session.StartTime.Format("2006-01-02 15:04:05"))

	if session.EndTime == nil {
		fmt.Printf("Status: Active (duration: %s)\n", formatDuration(time.Since(session.StartTime)))
	} else {
		fmt.Printf("End Time: %s\n", session.EndTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("Duration: %s\n", formatDuration(session.EndTime.Sub(session.StartTime)))
		if session.EndedBy != "" {
			fmt.Printf("Ended By: %s\n", session.EndedBy)
		}
	}

	fmt.Printf("Total Prompts: %d\n", session.TotalPrompts)
	fmt.Printf("Total Tokens: %d\n", session.TotalTokens)
	fmt.Println()

	events, err := storage.ListPromptEvents(db, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list prompts: %v\n", err)
		os.Exit(1)
	}

	if len(events) == 0 {
		fmt.Println("No prompts in this session")
		return
	}

	fmt.Printf("Prompts (%d):\n", len(events))
	fmt.Printf("%-25s %-20s %-15s %s\n", "ID", "Timestamp", "Agent", "Prompt")
	fmt.Println(strings.Repeat("-", 120))

	for _, event := range events {
		timestamp := event.Timestamp.Format("2006-01-02 15:04:05")
		promptPreview := event.PromptText
		if len(promptPreview) > 60 {
			promptPreview = promptPreview[:57] + "..."
		}

		fmt.Printf("%-25s %-20s %-15s %s\n", event.ID, timestamp, event.Agent, promptPreview)
	}
}

func sessionEnd(sessionID string) {
	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	session, err := storage.GetSession(db, sessionID)
	if err != nil {
		if err == storage.ErrNotFound {
			fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionID)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Failed to get session: %v\n", err)
		os.Exit(1)
	}

	if session.EndTime != nil {
		fmt.Fprintf(os.Stderr, "Session is already ended\n")
		os.Exit(1)
	}

	if err := storage.EndSession(db, sessionID, time.Now(), "manual"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to end session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Session %s ended successfully\n", sessionID)
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

func cmdStats() {
	var sessionID string
	var sinceStr string

	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--session" && i+1 < len(os.Args) {
			sessionID = os.Args[i+1]
			i++
		} else if os.Args[i] == "--since" && i+1 < len(os.Args) {
			sinceStr = os.Args[i+1]
			i++
		}
	}

	if sessionID != "" {
		statsSession(sessionID)
		return
	}

	if sinceStr != "" {
		statsSince(sinceStr)
		return
	}

	statsRepo()
}

func statsRepo() {
	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	repoPath, err := findGitRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find git repository: %v\n", err)
		os.Exit(1)
	}

	stats, err := storage.GetRepoStats(db, repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get repository statistics: %v\n", err)
		os.Exit(1)
	}

	displayStats("Repository", stats.TotalPrompts, stats.TotalTokensIn, stats.TotalTokensOut,
		stats.SessionCount, stats.FilesMentioned, stats.ToolsInvoked)
}

func statsSession(sessionID string) {
	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	stats, err := storage.GetSessionStats(db, sessionID)
	if err != nil {
		if err == storage.ErrNotFound {
			fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionID)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Failed to get session statistics: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Session: %s\n\n", stats.SessionID)
	displayStats("Session", stats.TotalPrompts, stats.TotalTokensIn, stats.TotalTokensOut,
		1, stats.FilesMentioned, stats.ToolsInvoked)
}

func statsSince(sinceStr string) {
	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	repoPath, err := findGitRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find git repository: %v\n", err)
		os.Exit(1)
	}

	since, err := parseTimeString(sinceStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse time: %v\n", err)
		os.Exit(1)
	}

	stats, err := storage.GetTimeframeStats(db, repoPath, since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get statistics: %v\n", err)
		os.Exit(1)
	}

	displayStats(fmt.Sprintf("Since %s", sinceStr), stats.TotalPrompts, stats.TotalTokensIn,
		stats.TotalTokensOut, stats.SessionCount, stats.FilesMentioned, stats.ToolsInvoked)
}

func displayStats(title string, prompts, tokensIn, tokensOut, sessions int,
	files map[string]int, tools map[string]int) {

	fmt.Printf("%s Statistics\n", title)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Total Prompts: %d\n", prompts)
	fmt.Printf("Tokens In: %d\n", tokensIn)
	fmt.Printf("Tokens Out: %d\n", tokensOut)
	fmt.Printf("Sessions: %d\n", sessions)
	fmt.Println()

	if len(files) > 0 {
		fmt.Println("Top Files Mentioned:")
		type fileStat struct {
			path  string
			count int
		}
		var fileStats []fileStat
		for path, count := range files {
			fileStats = append(fileStats, fileStat{path, count})
		}
		for i := 0; i < len(fileStats); i++ {
			for j := i + 1; j < len(fileStats); j++ {
				if fileStats[j].count > fileStats[i].count {
					fileStats[i], fileStats[j] = fileStats[j], fileStats[i]
				}
			}
		}
		limit := 10
		if len(fileStats) < limit {
			limit = len(fileStats)
		}
		for i := 0; i < limit; i++ {
			fmt.Printf("  %3d  %s\n", fileStats[i].count, fileStats[i].path)
		}
		fmt.Println()
	}

	if len(tools) > 0 {
		fmt.Println("Top Tools Invoked:")
		type toolStat struct {
			name  string
			count int
		}
		var toolStats []toolStat
		for name, count := range tools {
			toolStats = append(toolStats, toolStat{name, count})
		}
		for i := 0; i < len(toolStats); i++ {
			for j := i + 1; j < len(toolStats); j++ {
				if toolStats[j].count > toolStats[i].count {
					toolStats[i], toolStats[j] = toolStats[j], toolStats[i]
				}
			}
		}
		limit := 10
		if len(toolStats) < limit {
			limit = len(toolStats)
		}
		for i := 0; i < limit; i++ {
			fmt.Printf("  %3d  %s\n", toolStats[i].count, toolStats[i].name)
		}
		fmt.Println()
	}
}

func parseTimeString(timeStr string) (time.Time, error) {
	timeStr = strings.TrimSpace(timeStr)

	// Parse relative time like "7 days ago", "2 hours ago", "30 minutes ago"
	parts := strings.Fields(timeStr)
	if len(parts) == 3 && parts[2] == "ago" {
		amount, err := strconv.Atoi(parts[0])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid time amount: %s", parts[0])
		}

		unit := parts[1]
		now := time.Now()

		switch {
		case strings.HasPrefix(unit, "second"):
			return now.Add(-time.Duration(amount) * time.Second), nil
		case strings.HasPrefix(unit, "minute"):
			return now.Add(-time.Duration(amount) * time.Minute), nil
		case strings.HasPrefix(unit, "hour"):
			return now.Add(-time.Duration(amount) * time.Hour), nil
		case strings.HasPrefix(unit, "day"):
			return now.Add(-time.Duration(amount) * 24 * time.Hour), nil
		case strings.HasPrefix(unit, "week"):
			return now.Add(-time.Duration(amount) * 7 * 24 * time.Hour), nil
		case strings.HasPrefix(unit, "month"):
			return now.AddDate(0, -amount, 0), nil
		case strings.HasPrefix(unit, "year"):
			return now.AddDate(-amount, 0, 0), nil
		default:
			return time.Time{}, fmt.Errorf("unknown time unit: %s", unit)
		}
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s (expected format like '7 days ago')", timeStr)
}

func cmdExport() {
	var format string
	var sessionID string
	var sinceStr string
	var outputPath string

	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--format" && i+1 < len(os.Args) {
			format = os.Args[i+1]
			i++
		} else if os.Args[i] == "--session" && i+1 < len(os.Args) {
			sessionID = os.Args[i+1]
			i++
		} else if os.Args[i] == "--since" && i+1 < len(os.Args) {
			sinceStr = os.Args[i+1]
			i++
		} else if os.Args[i] == "--output" && i+1 < len(os.Args) {
			outputPath = os.Args[i+1]
			i++
		}
	}

	if format == "" {
		format = "json"
	}

	if format != "json" && format != "csv" {
		fmt.Fprintf(os.Stderr, "Invalid format: %s (must be 'json' or 'csv')\n", format)
		os.Exit(1)
	}

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	var events []*storage.PromptEvent

	if sessionID != "" {
		events, err = storage.ListPromptEvents(db, sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list events for session: %v\n", err)
			os.Exit(1)
		}
	} else if sinceStr != "" {
		since, err := parseTimeString(sinceStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse time: %v\n", err)
			os.Exit(1)
		}
		events, err = exportEventsSince(db, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to export events: %v\n", err)
			os.Exit(1)
		}
	} else {
		events, err = exportAllEvents(db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to export events: %v\n", err)
			os.Exit(1)
		}
	}

	var output []byte
	if format == "json" {
		output, err = exportJSON(events)
	} else {
		output, err = exportCSV(events)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to export data: %v\n", err)
		os.Exit(1)
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, output, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write output file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Exported %d events to %s\n", len(events), outputPath)
	} else {
		fmt.Print(string(output))
	}
}

func exportAllEvents(db *sql.DB) ([]*storage.PromptEvent, error) {
	query := `
		SELECT id, timestamp, session_id, agent, model_version, prompt_text, response_text,
		       tokens_in, tokens_out, latency_ms, repo_path, git_commit, git_branch, git_dirty,
		       dirty_files, author, ide, active_file, workspace_files, prompt_type,
		       tools_invoked, files_mentioned
		FROM prompt_events
		ORDER BY timestamp DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanPromptEvents(rows)
}

func exportEventsSince(db *sql.DB, since time.Time) ([]*storage.PromptEvent, error) {
	query := `
		SELECT id, timestamp, session_id, agent, model_version, prompt_text, response_text,
		       tokens_in, tokens_out, latency_ms, repo_path, git_commit, git_branch, git_dirty,
		       dirty_files, author, ide, active_file, workspace_files, prompt_type,
		       tools_invoked, files_mentioned
		FROM prompt_events
		WHERE timestamp >= ?
		ORDER BY timestamp DESC
	`

	rows, err := db.Query(query, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanPromptEvents(rows)
}

func scanPromptEvents(rows *sql.Rows) ([]*storage.PromptEvent, error) {
	var events []*storage.PromptEvent

	for rows.Next() {
		var e storage.PromptEvent
		var timestampUnix int64
		var modelVersion, responseText, gitCommit, gitBranch, ide, activeFile sql.NullString
		var latencyMs sql.NullInt64
		var gitDirty sql.NullBool
		var dirtyFilesJSON, workspaceFilesJSON, promptType, toolsInvokedJSON, filesMentionedJSON string

		err := rows.Scan(
			&e.ID, &timestampUnix, &e.SessionID, &e.Agent, &modelVersion,
			&e.PromptText, &responseText, &e.TokensIn, &e.TokensOut, &latencyMs,
			&e.RepoPath, &gitCommit, &gitBranch, &gitDirty, &dirtyFilesJSON,
			&e.Author, &ide, &activeFile, &workspaceFilesJSON, &promptType,
			&toolsInvokedJSON, &filesMentionedJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		e.Timestamp = time.Unix(timestampUnix, 0)

		if modelVersion.Valid {
			e.ModelVersion = modelVersion.String
		}
		if responseText.Valid {
			e.ResponseText = responseText.String
		}
		if latencyMs.Valid {
			e.LatencyMs = int(latencyMs.Int64)
		}
		if gitCommit.Valid {
			e.GitCommit = gitCommit.String
		}
		if gitBranch.Valid {
			e.GitBranch = gitBranch.String
		}
		if gitDirty.Valid {
			e.GitDirty = gitDirty.Bool
		}
		if ide.Valid {
			e.IDE = ide.String
		}
		if activeFile.Valid {
			e.ActiveFile = activeFile.String
		}
		if promptType != "" {
			e.PromptType = promptType
		}

		json.Unmarshal([]byte(dirtyFilesJSON), &e.DirtyFiles)         //nolint:errcheck
		json.Unmarshal([]byte(workspaceFilesJSON), &e.WorkspaceFiles) //nolint:errcheck
		json.Unmarshal([]byte(toolsInvokedJSON), &e.ToolsInvoked)     //nolint:errcheck
		json.Unmarshal([]byte(filesMentionedJSON), &e.FilesMentioned) //nolint:errcheck

		events = append(events, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return events, nil
}

func exportJSON(events []*storage.PromptEvent) ([]byte, error) {
	return json.MarshalIndent(events, "", "  ")
}

func exportCSV(events []*storage.PromptEvent) ([]byte, error) {
	var buf strings.Builder

	buf.WriteString("id,timestamp,session_id,agent,model_version,prompt_text,response_text,")
	buf.WriteString("tokens_in,tokens_out,latency_ms,repo_path,git_commit,git_branch,git_dirty,")
	buf.WriteString("author,ide,active_file,prompt_type,tools_invoked,files_mentioned\n")

	for _, e := range events {
		fields := []string{
			e.ID,
			e.Timestamp.Format(time.RFC3339),
			e.SessionID,
			e.Agent,
			e.ModelVersion,
			escapeCSV(e.PromptText),
			escapeCSV(e.ResponseText),
			fmt.Sprintf("%d", e.TokensIn),
			fmt.Sprintf("%d", e.TokensOut),
			fmt.Sprintf("%d", e.LatencyMs),
			e.RepoPath,
			e.GitCommit,
			e.GitBranch,
			fmt.Sprintf("%t", e.GitDirty),
			e.Author,
			e.IDE,
			e.ActiveFile,
			e.PromptType,
			escapeCSV(strings.Join(e.ToolsInvoked, ";")),
			escapeCSV(strings.Join(e.FilesMentioned, ";")),
		}

		buf.WriteString(strings.Join(fields, ","))
		buf.WriteString("\n")
	}

	return []byte(buf.String()), nil
}

func escapeCSV(field string) string {
	if strings.ContainsAny(field, ",\"\n\r") {
		field = strings.ReplaceAll(field, "\"", "\"\"")
		return "\"" + field + "\""
	}
	return field
}

// cmdBlame shows which AI prompts led to changes in a commit or file
func cmdBlame() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: prov blame <commit-sha|file-path>")
		os.Exit(1)
	}

	target := os.Args[2]

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	var changeSets []*storage.ChangeSet
	changeSets, err = storage.GetChangeSetsForCommitPrefix(db, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query change sets: %v\n", err)
		os.Exit(1)
	}

	if len(changeSets) == 0 {
		changeSets, err = storage.GetChangeSetsForFile(db, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to query change sets: %v\n", err)
			os.Exit(1)
		}
	}

	if len(changeSets) == 0 {
		fmt.Println("No prompts found for this commit or file")
		return
	}

	fmt.Printf("Found %d prompt(s) that led to changes:\n\n", len(changeSets))

	for i, cs := range changeSets {
		prompt, err := storage.GetPromptEvent(db, cs.PromptID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to get prompt %s: %v\n", cs.PromptID, err)
			continue
		}

		correlationIndicator := ""
		if cs.CorrelationMethod == "manual" {
			correlationIndicator = " [manual]"
		}

		fmt.Printf("[%d] Commit: %s (Confidence: %.2f / %.0f%%)%s\n", i+1, cs.CommitIntroduced, cs.Confidence, cs.Confidence*100, correlationIndicator)
		fmt.Printf("    Prompt ID: %s\n", prompt.ID)
		fmt.Printf("    Timestamp: %s\n", prompt.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("    Agent: %s\n", prompt.Agent)
		fmt.Printf("    Author: %s\n", prompt.Author)
		fmt.Printf("    Prompt: %s\n", truncatePrompt(prompt.PromptText, 200))

		if len(cs.FilesChanged) > 0 {
			fmt.Printf("    Files Changed:\n")
			for _, file := range cs.FilesChanged {
				fmt.Printf("      - %s\n", file)
			}
		}

		if cs.DiffSummary != "" {
			fmt.Printf("    Diff: %s\n", cs.DiffSummary)
		}

		fmt.Println()
	}
}

func truncatePrompt(prompt string, maxLen int) string {
	if len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen-3] + "..."
}

func cmdTag() {
	// Check minimum args before parsing flags
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: prov tag <prompt-id> --commit <sha>")
		fmt.Fprintln(os.Stderr, "   or: prov tag <prompt-id> --file <path>")
		os.Exit(1)
	}

	// First arg after "tag" is prompt ID
	// Check if it's a flag instead of a prompt-id
	if strings.HasPrefix(os.Args[2], "-") {
		fmt.Fprintln(os.Stderr, "Usage: prov tag <prompt-id> --commit <sha>")
		fmt.Fprintln(os.Stderr, "   or: prov tag <prompt-id> --file <path>")
		os.Exit(1)
	}

	promptID := os.Args[2]

	// Parse flags starting from the third argument
	fs := flag.NewFlagSet("tag", flag.ExitOnError)
	commitFlag := fs.String("commit", "", "Commit SHA to tag")
	fileFlag := fs.String("file", "", "File path to tag")
	fs.Parse(os.Args[3:]) //nolint:errcheck

	// Validate flags
	if *commitFlag == "" && *fileFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: Must specify either --commit or --file")
		os.Exit(1)
	}

	if *commitFlag != "" && *fileFlag != "" {
		fmt.Fprintln(os.Stderr, "Error: Cannot specify both --commit and --file (mutually exclusive)")
		os.Exit(1)
	}

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	// Verify prompt exists
	prompt, err := storage.GetPromptEvent(db, promptID)
	if err != nil {
		if err == storage.ErrNotFound {
			fmt.Fprintf(os.Stderr, "Error: Prompt not found: %s\n", promptID)
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to query prompt: %v\n", err)
		}
		os.Exit(1)
	}

	// Create manual change set
	changeSet := &storage.ChangeSet{
		ID:                fmt.Sprintf("cs-%d-%s", time.Now().UnixNano(), promptID[:8]),
		PromptID:          promptID,
		SessionID:         prompt.SessionID,
		Timestamp:         time.Now(),
		CorrelationMethod: "manual",
		Confidence:        1.0,
	}

	if *commitFlag != "" {
		changeSet.CommitIntroduced = *commitFlag
		changeSet.FilesChanged = []string{} // Empty for commit-only tags
	} else {
		changeSet.FilesChanged = []string{*fileFlag}
		changeSet.CommitIntroduced = "" // Empty for file-only tags
	}

	if err := storage.CreateChangeSet(db, changeSet); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create tag: %v\n", err)
		os.Exit(1)
	}

	if *commitFlag != "" {
		fmt.Printf("Tagged prompt %s to commit %s\n", promptID, *commitFlag)
	} else {
		fmt.Printf("Tagged prompt %s to file %s\n", promptID, *fileFlag)
	}
}

func cmdUntag() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: prov untag <prompt-id> <commit-sha>")
		os.Exit(1)
	}

	promptID := os.Args[2]
	commitSHA := os.Args[3]

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	// Delete the manual tag
	err = storage.DeleteManualChangeSet(db, promptID, commitSHA)
	if err != nil {
		if strings.Contains(err.Error(), "no manual tag found") {
			fmt.Fprintf(os.Stderr, "Error: No manual tag found for prompt %s and commit %s\n", promptID, commitSHA)
		} else if strings.Contains(err.Error(), "not a manual tag") {
			fmt.Fprintf(os.Stderr, "Error: Cannot untag - this is %s\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to untag: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("Untagged prompt %s from commit %s\n", promptID, commitSHA)
}
