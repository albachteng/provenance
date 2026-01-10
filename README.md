# AI Provenance

Track AI-assisted code changes across your development workflow. An open-source, agent-agnostic provenance system that helps teams understand AI usage patterns, measure ROI, and maintain code quality.

> **Status**: ✅ Phase 0 complete - Core foundation with session management and configuration! ✅ Phase 2A complete - Claude Code hooks actively capturing! All prompts and tool invocations automatically logged with intelligent session boundaries. See `ROADMAP.md` for development plans.

## Quick Start

### Build

```bash
git clone https://github.com/albachteng/provenance.git
cd provenance
go build -o prov ./cmd/prov
```

### Start the Daemon

The daemon runs in the background and stores AI interaction events in a local SQLite database.

```bash
# Start the daemon
./prov daemon start

# Check daemon status
./prov daemon status

# Stop the daemon (when needed)
./prov daemon stop
```

The daemon:
- Listens on a Unix socket at `~/.ai-provenance/daemon.sock`
- Stores events in `~/.ai-provenance/db.sqlite`
- Runs in the background until stopped

### Capture AI Tool Usage

**Option 1: Claude Code Hooks (Automatic)**

For Claude Code users, install hooks to automatically capture all interactions:

```bash
./prov install-hooks claude-code
```

This installs hook scripts to `~/.ai-provenance/hooks/` and updates `~/.claude/settings.json`. The hooks automatically embed the full path to your `prov` binary, so you **don't need to add `prov` to your PATH**.

**What gets captured:**
- Every prompt you send to Claude (`UserPromptSubmit`)
- All tool invocations (Read, Write, Edit, Bash, etc.) with inputs/outputs
- Session context and git state
- Zero configuration after installation

**Verify installation:**
```bash
./prov hooks status

# Output:
# Installed hooks:
#
# claude-code:
#   - claude-prompt.py
#   - claude-tool-pre.py
#   - claude-tool-post.py
#   - claude-session.py
```

**Uninstall (if needed):**
```bash
./prov hooks uninstall claude-code
```

**Using with multiple Claude projects:**

The hooks are installed globally for your user. Every Claude Code project will automatically capture events to the same database. To view events for a specific project, navigate to that directory and run `./prov list`.

**Option 2: Shell Hook (For CLI Tools)**

Add provenance tracking to your shell:

```bash
# For bash users - add to ~/.bashrc
eval "$(./prov hook bash)"

# For zsh users - add to ~/.zshrc
eval "$(./prov hook zsh)"

# Then create aliases for your AI tools
alias ai='prov wrap claude-code'
alias aider='prov wrap aider aider'
```

Now when you run `ai "your prompt"`, it automatically captures the interaction.

**Option 3: Direct Wrapper**

Manually wrap AI commands:

```bash
# Wrap any command to track it
./prov wrap claude-code echo "Implement authentication"
./prov wrap aider aider --message "Add tests"
```

The wrapper:
- Executes the command normally (passes through stdin/stdout/stderr)
- Captures prompt text, git context, and metadata
- Sends event to daemon for storage
- Preserves exit codes

### Query Events

Once the daemon is running and capturing events, you can query them:

```bash
# List recent prompts (default: 10)
./prov list

# List more events
./prov list --limit 50

# Show detailed information about a specific event
./prov show <event-id>

# Search for prompts containing specific text
./prov search "authentication"
```

### Export Session Data

Export your AI interaction data to CSV or JSON for analysis, reporting, or integration with other tools:

```bash
# Export all events as JSON (default format, prints to stdout)
./prov export

# Export as CSV
./prov export --format csv

# Export to a file
./prov export --format json --output sessions.json
./prov export --format csv --output sessions.csv

# Export a specific session
./prov export --session <session-id> --output session.json

# Export events from the last 7 days
./prov export --since "7 days ago" --format csv --output recent.csv

# Export events from the last 24 hours
./prov export --since "24 hours ago" --format json
```

**Export options:**
- `--format`: Output format (`json` or `csv`, defaults to `json`)
- `--output`: Write to file instead of stdout
- `--session`: Export only events from a specific session ID
- `--since`: Export events since a relative time (e.g., "7 days ago", "2 hours ago")

**CSV format includes:**
- Event metadata (ID, timestamp, session ID, agent, model version)
- Prompt and response text
- Token counts and latency metrics
- Git context (commit, branch, dirty state)
- Author, IDE, and file information
- Tools invoked and files mentioned (semicolon-separated)

**JSON format** provides the complete event data structure with all fields and nested arrays.

### Configuration

AI Provenance supports a hierarchical configuration system with sensible defaults.

**Configuration Hierarchy** (highest to lowest priority):
1. Environment variables (`AI_PROVENANCE_*`)
2. Per-repository config (`.ai-provenance/config.yaml`)
3. Global config (`~/.ai-provenance/config.yaml`)
4. Built-in defaults

**View current configuration:**
```bash
./prov config show
```

**Create a config file:**
```bash
# Create global config
./prov config init --global

# Create per-repo config (in current git repository)
./prov config init
```

**Validate configuration:**
```bash
./prov config validate
```

**Configuration options include:**
- **Session strategy**: `smart-time` (activity-based) or `git-event` (commit/branch-based)
- **Session timeouts**: Base timeout, activity check interval, fallback timeout
- **Storage paths**: Database, socket, PID file locations
- **Daemon settings**: Startup timeout, shutdown timeout, session check interval
- **Redaction rules**: Built-in patterns and custom redaction

**Example configuration** (`~/.ai-provenance/config.yaml`):
```yaml
session:
  strategy: "smart-time"  # or "git-event"
  smart_time:
    base_timeout: "30m"
    activity_check_interval: "5m"
    extend_if_active: true

storage:
  db_path: "db.sqlite"
  wal_mode: true

daemon:
  session_check_interval: "1m"
```

**Environment variable overrides:**
```bash
export AI_PROVENANCE_HOME=/custom/path
export AI_PROVENANCE_SESSION_STRATEGY=git-event
export AI_PROVENANCE_SESSION_TIMEOUT=45m
./prov daemon start
```

## Try It Out

### With Claude Code

```bash
# Build and start daemon
go build -o prov ./cmd/prov
./prov daemon start

# Install Claude Code hooks (one-time setup)
./prov install-hooks claude-code

# Now use Claude Code normally - all interactions are captured automatically!

# View captured events
./prov list

# Output:
# ID                   Timestamp            Agent           Prompt
# ----------------------------------------------------------------------------------------------------
# evt-1735567890-12345 2025-12-30 10:30:00 claude-code     Implement user authentication
# evt-1735567891-67890 2025-12-30 10:30:05 claude-code     Edit: {"file_path":"auth.go",...}
#
# Showing 2 event(s)

# See detailed event info
./prov show evt-1735567890-12345

# Search for specific prompts
./prov search "authentication"

# Export your session data
./prov export --format csv --output my-ai-sessions.csv
./prov export --format json --output my-ai-sessions.json
```

### With CLI Tools

```bash
# Build and start daemon
go build -o prov ./cmd/prov
./prov daemon start

# Wrap a command to track it
./prov wrap aider aider --message "Add user authentication"

# View captured events
./prov list
```

The captured event includes:
- Prompt text and agent used
- Git context (branch, commit, dirty state)
- Timestamp and author
- Repository path and session ID

## How It Works

A background daemon stores AI interaction events in a local SQLite database with intelligent session management:

```
Capture Adapters → Unix Socket → Storage Daemon → SQLite Database
Claude Code Hooks     ↓         Session Manager        ↓
Shell Wrappers     Commands      Event Store      ~/.ai-provenance/
```

**Session Management:**
- Automatically groups related prompts into sessions
- Two strategies: `smart-time` (activity-based) and `git-event` (commit/branch-based)
- Sessions end on inactivity timeout or git events (configurable)
- Background process checks session boundaries every minute

**Event Capture:**
Each AI interaction captures metadata (agent, model, prompt/response), git context (branch, commit, dirty files), and developer context (author, IDE, workspace).

See `ROADMAP.md` for architecture details and data schema.

## Development

### Running Tests

```bash
# All tests
go test ./...

# With race detector
go test -race ./...

# Specific package
go test ./internal/storage/...
```

Test suite runs in **~1.5s** for CLI integration tests.

### Project Structure

```
provenance/
├── cmd/prov/              # CLI and daemon entry point
├── internal/
│   ├── config/            # Configuration system (YAML + env vars)
│   ├── daemon/            # Unix socket server
│   ├── session/           # Session management strategies
│   ├── storage/           # SQLite database layer
│   └── git/               # Git integration
├── ROADMAP.md             # Development roadmap and design decisions
└── DESIGN.md              # Technical design document
```

## Documentation

- **`README.md`**: This file - quick start and usage guide
- **`TESTING.md`**: Manual testing guide with step-by-step examples
- **`ROADMAP.md`**: Development roadmap, phase breakdown, and design decisions
- **`DESIGN.md`**: Technical design document and architecture details

## Troubleshooting

### Hooks not capturing events

**Problem**: `./prov list` shows no events after installing hooks.

**Diagnosis**:
```bash
# 1. Check if daemon is running
./prov daemon status

# 2. Verify hooks are installed
./prov hooks status

# 3. Check Claude settings
cat ~/.claude/settings.json | grep -A 10 hooks

# 4. Test hook manually
echo '{"hook_event_name":"UserPromptSubmit","session_id":"test","prompt":"test prompt","cwd":"'$(pwd)'"}' | ~/.ai-provenance/hooks/claude-prompt.py
./prov list
```

**Common fixes**:
- Restart Claude Code after installing hooks
- Reinstall hooks: `./prov hooks uninstall claude-code && ./prov install-hooks claude-code`
- Check hook script permissions: `ls -l ~/.ai-provenance/hooks/`

### Database locked errors

**Problem**: "database is locked" error when running CLI commands.

**Cause**: SQLite WAL mode can have locks if daemon crashed.

**Fix**:
```bash
# Stop daemon cleanly
./prov daemon stop

# Force clean if needed
rm -f ~/.ai-provenance/daemon.sock
rm -f ~/.ai-provenance/daemon.pid

# Restart
./prov daemon start
```

### Events missing after moving prov binary

**Problem**: Hooks stopped working after moving `prov` binary to a new location.

**Cause**: Hook scripts contain the full path to the `prov` binary embedded at install time.

**Fix**: Reinstall hooks to update the embedded path:
```bash
./prov install-hooks claude-code
```

### No events for user prompts

**Problem**: Tool invocations are captured, but user prompts are missing.

**Cause**: User prompts require the Claude Code interactive shell to trigger `UserPromptSubmit` hooks.

**Verify**: Check that you're seeing `UserPromptSubmit` events:
```bash
./prov search "UserPromptSubmit"
```

If missing, ensure hooks are properly registered in `~/.claude/settings.json`.

## Contributing

See `CONTRIBUTING.md` (coming soon) for guidelines.

## License

MIT (to be added)

## Questions & Feedback

File issues at: https://github.com/albachteng/provenance/issues
