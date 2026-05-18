package main

import (
	"database/sql"
	_ "embed"
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

	"github.com/albachteng/provenance/internal/config"
	"github.com/albachteng/provenance/internal/daemon"
	"github.com/albachteng/provenance/internal/git"
	"github.com/albachteng/provenance/internal/queries"
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
// V2 NOTE: Session commands removed - v2 uses commit windows instead of sessions
// Removed functions: sessionList(), sessionShow(), sessionEnd(), formatDuration()

func cmdStats() {
	fmt.Fprintln(os.Stderr, "Stats commands not yet implemented in v2 architecture")
	fmt.Fprintln(os.Stderr, "Planned features:")
	fmt.Fprintln(os.Stderr, "  - prov stats --branch <name>  # Per-branch aggregation")
	fmt.Fprintln(os.Stderr, "  - prov stats --since <date>   # Timeframe statistics")
	fmt.Fprintln(os.Stderr, "  - prov stats                  # Repository-wide stats")
	os.Exit(1)
}

// V2 NOTE: Old stats and export functions removed - will be reimplemented with commit window queries
// Removed: statsRepo(), statsSession(), statsSince(), displayStats(), parseTimeString()
// Removed: exportAllEvents(), exportEventsSince(), scanPromptEvents(), exportJSON(), exportCSV(), escapeCSV()

func cmdExport() {
	fmt.Fprintln(os.Stderr, "Export commands not yet fully implemented in v2 architecture")
	fmt.Fprintln(os.Stderr, "Planned usage:")
	fmt.Fprintln(os.Stderr, "  prov export --format json|csv [--since <time>] [--output <file>]")
	fmt.Fprintln(os.Stderr, "  prov export --branch <name> --format json")
	os.Exit(1)
}
// cmdBlame shows which AI prompts led to changes in a commit or file
func cmdBlame() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: prov blame <commit-sha>")
		os.Exit(1)
	}

	commitSHA := os.Args[2]

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	// Get current repo path
	repoPath, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	// Get the branch for this commit
	branch, err := git.GetBranchForCommit(repoPath, commitSHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to determine branch for commit: %v\n", err)
		os.Exit(1)
	}

	// Query prompts in commit window
	prompts, err := queries.GetPromptsForCommit(db, repoPath, commitSHA, branch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query prompts: %v\n", err)
		os.Exit(1)
	}

	if len(prompts) == 0 {
		fmt.Println("No prompts found in commit window")
		return
	}

	// Get commit time for display
	commitTime, err := git.GetCommitTime(repoPath, commitSHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get commit time: %v\n", err)
		os.Exit(1)
	}

	// Get files changed in commit
	filesChanged, err := git.GetFilesChanged(repoPath, commitSHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to get files changed: %v\n", err)
		filesChanged = []string{}
	}

	fmt.Printf("Commit: %s (%s)\n", commitSHA, commitTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Branch: %s\n", branch)
	fmt.Printf("\nFound %d prompt(s) in commit window:\n\n", len(prompts))

	for i, prompt := range prompts {
		fmt.Printf("[%d] Prompt ID: %s\n", i+1, prompt.ID)
		fmt.Printf("    Timestamp: %s\n", prompt.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("    Agent: %s\n", prompt.Agent)
		if prompt.Author != "" {
			fmt.Printf("    Author: %s\n", prompt.Author)
		}
		fmt.Printf("    Prompt: %s\n", truncatePrompt(prompt.PromptText, 200))

		if len(prompt.ToolsInvoked) > 0 {
			fmt.Printf("    Tools: %v\n", prompt.ToolsInvoked)
		}

		fmt.Println()
	}

	if len(filesChanged) > 0 {
		fmt.Printf("Files changed in commit:\n")
		for _, file := range filesChanged {
			fmt.Printf("  - %s\n", file)
		}
	}
}

func truncatePrompt(prompt string, maxLen int) string {
	if len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen-3] + "..."
}

func cmdTag() {
	fmt.Fprintln(os.Stderr, "Manual tagging not yet implemented in v2 architecture")
	fmt.Fprintln(os.Stderr, "Planned usage:")
	fmt.Fprintln(os.Stderr, "  prov tag <commit-sha> <prompt-id>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "This will override automated commit window detection")
	os.Exit(1)
}

func cmdUntag() {
	fmt.Fprintln(os.Stderr, "Manual untagging not yet implemented in v2 architecture")
	fmt.Fprintln(os.Stderr, "Planned usage:")
	fmt.Fprintln(os.Stderr, "  prov untag <commit-sha> <prompt-id>")
	os.Exit(1)
}
