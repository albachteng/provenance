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
	"github.com/albachteng/provenance/internal/session"
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

//go:embed hooks/claude-session.py
var claudeSessionScript string

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

	strategy, err := cfg.CreateSessionStrategy()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session strategy: %v\n", err)
		os.Exit(1)
	}

	sessionMgr := session.NewManager(db, strategy)

	daemon, err := daemon.NewDaemon(db, cfg.Daemon.SocketPath, sessionMgr, cfg)
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

	// Get or create a session for this repo
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
		"claude-session.py":   claudeSessionScript,
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
	fmt.Printf("Active strategy: %s\n", cfg.Session.Strategy)
	fmt.Printf("Database path: %s\n", cfg.Storage.DBPath)
	fmt.Printf("Socket path: %s\n", cfg.Daemon.SocketPath)

	strategy, err := cfg.CreateSessionStrategy()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session strategy: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Session strategy: %s\n", strategy.Name())
}

// Session command implementations

func cmdSession() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: prov session <list|show|end>")
		os.Exit(1)
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "list":
		sessionList()
	case "show":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: prov session show <id>")
			os.Exit(1)
		}
		sessionShow(os.Args[3])
	case "end":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: prov session end <id>")
			os.Exit(1)
		}
		sessionEnd(os.Args[3])
	default:
		fmt.Fprintf(os.Stderr, "Unknown session command: %s\n", subcommand)
		fmt.Fprintln(os.Stderr, "Usage: prov session <list|show|end>")
		os.Exit(1)
	}
}

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer rows.Close()

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
	defer rows.Close()

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

		json.Unmarshal([]byte(dirtyFilesJSON), &e.DirtyFiles)
		json.Unmarshal([]byte(workspaceFilesJSON), &e.WorkspaceFiles)
		json.Unmarshal([]byte(toolsInvokedJSON), &e.ToolsInvoked)
		json.Unmarshal([]byte(filesMentionedJSON), &e.FilesMentioned)

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
