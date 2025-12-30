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
	// Build the command to execute
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Capture the prompt (command + args as a simple prompt)
	promptText := strings.Join(append([]string{command}, args...), " ")

	// Execute the command
	err := cmd.Run()

	// Send event to daemon (best effort, don't fail if daemon not running)
	sendEventToDaemon(agent, promptText)

	// Return exit code
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

	// Get git context
	var gitCommit, gitBranch string
	var gitDirty bool
	gitInfo, err := git.CaptureGitState(".")
	if err == nil {
		gitCommit = gitInfo.Head
		gitBranch = gitInfo.Branch
		gitDirty = gitInfo.IsDirty
	}

	// Get current directory as repo path
	repoPath, _ := os.Getwd()

	// Get username
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

	// Create PromptEvent matching storage.PromptEvent structure
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
