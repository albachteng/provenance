# AI Provenance

Track AI-assisted code changes across your development workflow. An open-source, agent-agnostic provenance system that helps teams understand AI usage patterns, measure ROI, and maintain code quality.

> **Status**: ✅ Phase 0 complete - Core foundation! ✅ Phase 2A complete - Claude Code hooks capturing! ✅ V2 Architecture - Migrated to **commit window-based** design (simpler, faster). Day 1 complete: schema migration, git utilities, storage layer, 29 tests passing. Next: Query layer (Day 2). See `ROADMAP.md` for development plans.

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

### Link Commits to AI Prompts (Commit Windows)

AI Provenance uses **commit windows** to associate prompts with code changes. A commit window is the time period between consecutive commits on a branch.

**How it works:**
- Every prompt captures the current branch at the time of submission
- When you run `prov blame <commit>`, it finds prompts in the window: `(previous_commit, this_commit]`
- Prompts are matched by: timestamp within window + same branch

**Branch switching edge case:**
- When you switch branches without committing, prompts are flagged as `pre_branch_switch`
- This prevents prompts from one branch being incorrectly associated with commits on another branch
- The system tracks: "I was working on branch A, but switched to branch B before committing"

**Post-commit hook (optional):**
```bash
# Install hook to cache commit windows for faster queries
./prov install-hook post-commit
```

The hook caches commit window metadata (previous commit, current commit, timestamps, prompt count) to speed up blame queries. The cache is optional - queries work without it by computing windows on-demand from git history.

**Check hook status:**
```bash
./prov hooks status
```

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

### Trace Commits to AI Prompts (Blame)

Find out which AI prompts led to specific code changes in your repository:

```bash
# Blame a commit (supports full or short SHA)
./prov blame abc123def456
./prov blame abc123  # Short SHA works too

# Blame a file to see all AI prompts that modified it (future)
./prov blame src/auth.go
```

**Example output:**
```
Commit: abc123def456 (2026-01-15 17:35:00)
Branch: feature/auth

Prompts in window (2):

[1] prompt-evt-1234 (17:32:15)
    Agent: claude-code
    Author: alice@example.com
    "Implement user authentication"
    Tokens: 1200 in, 3400 out
    Tools: Write (auth.go), Edit (auth_test.go)

[2] prompt-evt-1235 (17:34:45)
    Agent: claude-code
    Author: alice@example.com
    "Add password validation"
    Tokens: 800 in, 2100 out
    Tools: Edit (auth.go)

Files changed in commit:
  auth.go (+120 -15)
  auth_test.go (+85 -0)
```

**How it works:**
- Finds the previous commit on the same branch
- Queries prompts between `prev_commit_time` and `commit_time`
- Filters to prompts on the same branch (using `branch_at_capture`)
- Excludes prompts flagged as `pre_branch_switch` (abandoned work)

**Use cases:**
- **Code review**: See which AI prompts generated specific commits
- **Debugging**: Trace bugs back to the original AI conversation
- **Learning**: Understand how specific features were built
- **Audit**: Track AI contributions to your codebase
- **Documentation**: Link code changes to their requirements/context

**Requirements:**
- Git repository (no hooks required - queries git history directly)
- Prompts captured with Claude Code hooks or other adapters

### Export Data

Export your AI interaction data to CSV or JSON for analysis, reporting, or integration with other tools:

```bash
# Export all events as JSON (default format, prints to stdout)
./prov export

# Export as CSV
./prov export --format csv

# Export to a file
./prov export --format json --output prompts.json
./prov export --format csv --output prompts.csv

# Export events from the last 7 days
./prov export --since "7 days ago" --format csv --output recent.csv

# Export events from the last 24 hours
./prov export --since "24 hours ago" --format json
```

**Export options:**
- `--format`: Output format (`json` or `csv`, defaults to `json`)
- `--output`: Write to file instead of stdout
- `--since`: Export events since a relative time (e.g., "7 days ago", "2 hours ago")

**CSV format includes:**
- Event metadata (ID, timestamp, agent, model version)
- Prompt and response text
- Token counts and latency metrics
- Git context (commit, branch at capture, dirty state)
- Author, IDE, and file information
- Tools invoked and files mentioned (semicolon-separated)

**JSON format** provides the complete event data structure with all fields and nested arrays.

**Note:** Export currently includes legacy `session_id` field (nullable) from v1 architecture. This field will be removed in a future update.

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
- **Storage paths**: Database, socket, PID file locations
- **Daemon settings**: Startup timeout, shutdown timeout
- **Redaction rules**: Built-in patterns and custom redaction

**Example configuration** (`~/.ai-provenance/config.yaml`):
```yaml
storage:
  db_path: "db.sqlite"
  wal_mode: true

daemon:
  startup_timeout: "10s"
  shutdown_timeout: "5s"

redaction:
  enabled: true
  builtin_patterns: true
```

**Environment variable overrides:**
```bash
export AI_PROVENANCE_HOME=/custom/path
./prov daemon start
```

**Note:** Configuration system currently includes legacy session settings from v1 architecture. These are ignored and will be removed in a future update.

## Try It Out

### With Claude Code

```bash
# Build and start daemon
go build -o prov ./cmd/prov
./prov daemon start

# Install Claude Code hooks (one-time setup)
./prov install-hooks claude-code

# Install git correlation hook (in your repository)
./prov install-hook post-commit

# Now use Claude Code normally - all interactions are captured automatically!
# When you commit, the hook links commits to your prompts

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

# After making a commit, blame it to see which prompts generated it
git add .
git commit -m "Add authentication feature"
./prov blame HEAD  # or use the commit SHA

# Blame a specific file to see its AI history
./prov blame src/auth.go
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

A background daemon stores AI interaction events in a local SQLite database using a commit window-based architecture:

```
Capture Adapters → Unix Socket → Storage Daemon → SQLite Database
Claude Code Hooks     ↓              ↓                    ↓
Shell Wrappers     Commands      Event Store      ~/.ai-provenance/
```

**Commit Window Architecture:**
- Prompts are associated with commits based on timestamps and branch tracking
- Each prompt captures the branch at submission time (`branch_at_capture`)
- Commit windows defined as: `(previous_commit, current_commit, branch)`
- Branch switch detection prevents incorrect associations (`pre_branch_switch` flag)
- Query prompts for a commit on-demand (no pre-computation needed)

**Event Capture:**
Each AI interaction captures:
- **Metadata**: Agent, model, prompt/response text, tokens, latency
- **Git context**: Current branch (`branch_at_capture`), commit, dirty files
- **Developer context**: Author, IDE, workspace, active file
- **Tool invocations**: Separate table tracks Read, Write, Edit, Bash calls with file paths

**Performance:**
- Prompt capture: < 10ms (immediate write to SQLite)
- Blame query: 50-100ms (on-demand from git + database)
- Optional commit window cache for even faster queries

See `ROADMAP.md` for architecture details and v2 migration notes.

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
│   └── hooks/             # Embedded hook scripts (Python)
├── internal/
│   ├── config/            # Configuration system (YAML + env vars)
│   ├── daemon/            # Unix socket server
│   ├── storage/           # SQLite database layer (v2: commit windows)
│   │   ├── migrations/    # Schema migrations
│   │   ├── windows.go     # Commit window cache CRUD
│   │   └── tool_invocations.go  # Tool tracking
│   └── git/               # Git integration
│       ├── commits.go     # Commit metadata extraction
│       ├── blame.go       # Git blame parsing
│       └── branch.go      # Branch tracking
├── ROADMAP.md             # Development roadmap and v2 architecture notes
└── DESIGN.md              # Technical design document
```

**Note:** `internal/session/` and `internal/correlation/` from v1 architecture will be removed in a future cleanup (Day 5).

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
