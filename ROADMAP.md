# AI Provenance - Development Roadmap

## Vision

Create an **open-source, agent-agnostic provenance system** for AI-assisted development that helps tech leads and organizations understand:
- Which AI interactions produce valuable code vs regressions
- ROI and usage patterns across teams
- Best practices for prompt engineering at scale
- Attribution chains for debugging and compliance

## Design Principles

1. **Agent-agnostic**: Work with Claude Code, Cursor, Copilot, Aider, etc.
2. **Non-invasive**: Developers barely notice it's running
3. **Local-first**: CLI tool per-developer, aggregate data later
4. **Privacy-respecting**: Built-in redaction, encryption, opt-out
5. **Team-ready**: Easy to roll out, easy to aggregate, easy to analyze

---

## Architecture Overview

```
┌────────────────────────────────────────────────────────────┐
│                    Capture Adapters                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐ │
│  │ Claude   │  │ VS Code  │  │ Neovim   │  │ Shell Hook │ │
│  │ Hooks    │  │ Extension│  │ Plugin   │  │ (Aider)    │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬───────┘ │
└───────┼─────────────┼─────────────┼─────────────┼─────────┘
        │             │             │             │
        └─────────────┴─────────────┴─────────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │  Local Storage Daemon  │
            │  (Go, Unix Socket)     │
            └────────────┬───────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │   SQLite Database      │
            │  ~/.ai-provenance/     │
            └────────────┬───────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │   CLI Analytics        │
            │   prov stats/blame     │
            └────────────────────────┘
                         │
                         ▼ (optional)
            ┌────────────────────────┐
            │  Team Aggregation      │
            │  (Phase 5+)            │
            └────────────────────────┘
```

### Component Strategy

| Component | Language | Rationale |
|-----------|----------|-----------|
| Core storage daemon | Go | Fast, single binary, Git integration |
| CLI tool | Go | Same binary as daemon |
| Claude Code hooks | Python/Shell | Hook protocol, subprocess execution |
| VS Code extension | TypeScript | Native to VS Code APIs |
| Neovim plugin | Lua | Native to Neovim |
| Shell hook (Aider, etc.) | Shell/Python | Portable, easy to test |
| MCP server (optional) | Python | Query interface for Claude (deferred) |

---

## Data Model

### Core Schema

```sql
-- Core event table
CREATE TABLE prompt_events (
    id TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    session_id TEXT NOT NULL,

    -- AI metadata
    agent TEXT NOT NULL,              -- 'claude-code', 'cursor', 'copilot'
    model_version TEXT,
    prompt_text TEXT NOT NULL,
    response_text TEXT,

    -- Usage metrics
    tokens_in INTEGER,
    tokens_out INTEGER,
    latency_ms INTEGER,

    -- Git context
    repo_path TEXT NOT NULL,
    git_commit TEXT,
    git_branch TEXT,
    git_dirty BOOLEAN,
    dirty_files TEXT,                 -- JSON array

    -- Developer context
    author TEXT NOT NULL,
    ide TEXT,                         -- 'vscode', 'nvim', 'cli'
    active_file TEXT,
    workspace_files TEXT,             -- JSON array of open files

    -- Categorization
    prompt_type TEXT,                 -- 'chat', 'inline', 'edit', 'debug'
    tools_invoked TEXT,               -- JSON array
    files_mentioned TEXT              -- JSON array
);

-- Derived change correlations
CREATE TABLE change_sets (
    id TEXT PRIMARY KEY,
    prompt_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,

    files_changed TEXT NOT NULL,      -- JSON array
    diff_summary TEXT,                -- "+10 -5" style
    commit_introduced TEXT,

    correlation_method TEXT,          -- 'file_watch' | 'git_hook' | 'manual'
    confidence REAL,                  -- 0.0 - 1.0
    time_to_first_change_ms INTEGER,

    FOREIGN KEY (prompt_id) REFERENCES prompt_events(id)
);

-- Redaction rules
CREATE TABLE redaction_rules (
    id TEXT PRIMARY KEY,
    pattern TEXT NOT NULL,            -- regex
    replacement TEXT DEFAULT '[REDACTED]',
    scope TEXT DEFAULT 'both',        -- 'prompt' | 'response' | 'both'
    enabled BOOLEAN DEFAULT TRUE
);

-- Session metadata
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    start_time INTEGER NOT NULL,
    end_time INTEGER,
    repo_path TEXT,
    total_prompts INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0
);
```

---

## Phase 0: Foundation (Weeks 1-2)

**Goal**: Core storage engine and CLI scaffold

**Development Process**: **Test-Driven Development (TDD)**
- Write failing tests first (red)
- Review failing tests before implementing
- Implement to make tests pass (green)
- Refactor for clarity and performance

### Deliverables
- [x] **Go daemon**: Local Unix socket server
  - [x] SQLite schema + migrations (using golang-migrate)
  - [x] Write-ahead logging (WAL) mode
  - [x] Database initialization with proper permissions
  - [x] Event storage implementation (PromptEvent and Session CRUD)
  - [x] Accepts JSON events via Unix socket
  - [x] Graceful shutdown with socket cleanup
  - [x] Ready() channel for startup synchronization
  - [x] Concurrent connection handling
  - [x] PromptEvent and SessionEvent routing
  - [x] Session management (auto-start, timeout-based end)
  - [ ] HTTP fallback for Windows compatibility (deferred)

- [x] **CLI basics** (COMPLETE):
  ```bash
  prov version                 # Show version information
  prov daemon start            # Start background daemon
  prov daemon stop             # Stop background daemon
  prov daemon status           # Check daemon status
  prov list [--limit N]        # List recent prompts (default: 10)
  prov show <id>               # Show full prompt + response
  prov search "query"          # Search prompt text
  ```
  - [x] Version command with build info
  - [x] Daemon lifecycle management (start/stop/status)
  - [x] Query commands (list/show/search)
  - [x] Integration tests (9/9 tests passing)
  - [x] Environment variable configuration (AI_PROVENANCE_HOME)
  - [ ] prov init (deferred - daemon auto-initializes)
  - [ ] prov session commands (deferred to session management)
  - [ ] prov disable/enable (deferred to enhanced features)

- [x] **Git integration library** (COMPLETE):
  - [x] Capture HEAD, branch, dirty state
  - [x] List dirty files with diff summaries
  - [x] Compute ahead/behind vs remote
  - [x] Comprehensive edge case handling (14 tests)
  - [x] Swappable provider pattern for future migration
  - [ ] Session → commit correlation logic (deferred to session implementation)

- [x] **Configuration system** (COMPLETE):
  - [x] Global config: `~/.ai-provenance/config.yaml`
  - [x] Per-repo config: `.ai-provenance/config.yaml`
  - [x] Environment variable overrides (`AI_PROVENANCE_*`)
  - [x] Configuration schema with Duration wrapper for YAML
  - [x] Session strategies: `smart-time` (activity-based) and `git-event` (commit/branch-based)
  - [x] Session timeouts (base, activity check, fallback) - all configurable
  - [x] Storage paths (database, socket, PID file, hooks directory)
  - [x] Daemon settings (startup timeout, shutdown timeout, session check interval)
  - [x] Redaction configuration (builtin patterns + custom)
  - [x] CLI commands: `prov config show`, `prov config init`, `prov config validate`
  - [x] Hierarchical config loading with proper merging
  - [x] Pointer booleans to distinguish explicit false from unset
  - [x] Path resolution (relative → absolute based on provenance home)
  - [x] Comprehensive validation (19 test functions, 34 subtests passing)
  - [ ] Write batching (deferred - immediate writes for now)

- [x] **Session management** (COMPLETE):
  - [x] Session manager with pluggable strategy pattern
  - [x] SmartTimeStrategy: Activity-based with configurable timeout and extension
  - [x] GitEventStrategy: Commit/branch-based with fallback timeout
  - [x] Auto-start sessions on first prompt
  - [x] Background session boundary checking (configurable interval)
  - [x] Session lifecycle: StartSession, RecordEvent, EndSession
  - [x] LLM activity tracking for smart timeout extension
  - [x] Integration with daemon (ticker-based boundary checks)
  - [x] Comprehensive tests (13 test functions, 24 subtests passing)

- [x] **Test harness** (TDD):
  - [x] Unit tests for all core packages
  - [x] Mock events via Unix socket
  - [x] Integration tests: daemon + CLI
  - [x] Git state capture tests with fixture repos
  - [x] Session lifecycle tests
  - [x] Configuration loading and validation tests
  - [x] Strategy pattern tests

### Success Criteria
- [x] Database schema created with all required tables
- [x] WAL mode enabled for concurrent access
- [x] All schema tests passing (4/4)
- [x] Git state accurately captured in test repos (14/14 tests)
- [x] Edge cases handled (detached HEAD, merge conflicts, submodules, etc.)
- [x] Event storage CRUD operations implemented and tested (10/10 tests)
- [x] JSON array fields properly serialized/deserialized
- [x] Foreign key constraints validated
- [x] Daemon starts, accepts events via socket, stores in database (7/7 tests)
- [x] Daemon handles invalid JSON without crashing
- [x] Daemon graceful shutdown with socket cleanup
- [x] Concurrent event submissions handled correctly (10 simultaneous tested)
- [x] Can query events back via CLI (9/9 CLI tests passing)
- [x] CLI daemon control works (start/stop/status with PID management)
- [x] CLI query operations work (list/show/search with correct output)
- [x] Sessions auto-start and timeout correctly (session manager integrated)
- [x] Configuration override hierarchy works (env → repo → global → defaults)
- [x] Config CLI commands work (show, init, validate)
- [x] Session strategies work (smart-time and git-event implemented and tested)
- [x] Background session checking integrated with daemon
- [ ] Daemon survives crashes (WAL recovery - needs testing)
- [ ] All tests pass on Linux and macOS (currently Linux/WSL2 only)

### Refactoring Notes (Deferred)
Track technical debt and future improvements:

**Storage Module** (`internal/storage/db.go`):
- [ ] Extract hard-coded values to configuration (WAL mode, permissions, timeouts)
- [ ] Add structured logging for database operations
- [ ] Make migration path detection more robust (env var override?)
- [ ] Review SQLite connection pool settings for optimal performance
- [ ] Consider adding database health checks endpoint

**Event Storage** (`internal/storage/events.go`):
- [ ] Extract repeated JSON marshaling logic into helper functions
- [ ] Consider using prepared statements pool for frequently-executed queries
- [ ] Add batch insert operations for high-throughput scenarios
- [ ] Implement pagination for ListPromptEvents to handle large result sets
- [ ] Add structured logging for storage operations (debug level)
- [ ] Consider caching active sessions to reduce database queries

**Daemon** (`internal/daemon/server.go`):
- [ ] Add structured logging instead of log.Printf
- [ ] Consider adding metrics (events processed, connections, errors)
- [ ] Extract message routing logic into separate handler registry
- [ ] Add connection timeout configuration
- [ ] Consider response protocol (ACK messages for clients)
- [ ] Add connection pooling limits to prevent resource exhaustion
- [ ] HTTP listener implementation for Windows compatibility (Phase 2+)

**Git Module** (`internal/git/cli_provider.go`):
- [ ] Extract common git command execution logic
- [ ] Add context support for command cancellation
- [ ] Add structured logging for git operations
- [ ] Performance testing with large repositories
- [ ] Consider caching git state for rapid successive calls
- [ ] Evaluate migration to go-git library (eliminate git dependency)

**CLI Module** (`cmd/prov/`):
- [ ] Add structured logging instead of fmt.Fprintf to stderr
- [ ] Consider cobra/viper for more sophisticated CLI framework
- [ ] Add shell completion support (bash/zsh/fish)
- [ ] Extract repeated table formatting logic into helper package
- [ ] Add color output support (controlled by --color flag or env var)
- [ ] Consider adding --json output mode for all commands
- [ ] Add signal handling for daemon (SIGHUP for config reload)
- [ ] Improve daemon startup polling (use ready channel via socket handshake)

**Rationale for deferring**: Code is clean and functional. Wait until we have:
1. Configuration system implemented
2. Real-world usage patterns
3. Performance profiling data
4. User feedback on git command overhead
5. User feedback on CLI ergonomics and desired features

### Design Patterns Established

**Ready Channel Pattern** (introduced in daemon implementation):
```go
type AsyncComponent struct {
    ready chan struct{}
}

func (c *AsyncComponent) Ready() <-chan struct{} {
    return c.ready
}

func (c *AsyncComponent) Start() error {
    // ... setup work
    close(c.ready)  // Signal ready
    // ... continue
}
```

**Benefits**:
- Tests synchronize instantly (no arbitrary sleeps)
- Deterministic (no race conditions)
- Self-documenting (clear when component is ready)
- Better error messages (timeouts show what we're waiting for)

**Apply to future async components**:
- Session manager background goroutines
- Batch writer flush loops
- HTTP server startup
- Any long-running background tasks

**Polling Helper Pattern** (deterministic async testing):
```go
func waitForCondition(t *testing.T, check func() bool, timeout time.Duration, message string) {
    t.Helper()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if check() {
            return
        }
        time.Sleep(10 * time.Millisecond)  // Short poll interval
    }
    t.Fatalf("%s not met within %v timeout", message, timeout)
}
```

**Benefits**:
- **No arbitrary sleeps**: Tests don't waste time waiting fixed durations
- **Fast and deterministic**: Succeeds as soon as condition is met
- **Clear failure messages**: Timeout errors describe what was being waited for
- **Consistent pattern**: All async tests use same approach

**Examples**:
```go
// Wait for daemon socket to exist
waitForDaemonReady(t, tmpDir)

// Wait for event to appear in database
waitForEventInDB(t, db, func(agent, promptText string) bool {
    return agent == "claude-code" && strings.Contains(promptText, "test")
})

// Wait for session creation
sessionID := waitForSessionInDB(t, db)
```

**When to use**:
- Daemon/server startup (socket creation, port binding)
- Database operations (async event storage, session creation)
- File system operations (daemon writes PID file, creates sockets)
- Any asynchronous side effect verification

**Anti-pattern**:
```go
// WRONG: Arbitrary sleep
time.Sleep(100 * time.Millisecond)
db.Query(...)  // Might still fail if daemon is slow

// RIGHT: Poll until condition is met
waitForEventInDB(t, db, checkFunc)
```

Used in: `daemon/server_test.go`, `cmd/prov/hook_test.go`, all integration tests

**Test Helper Pattern** (polling with timeout - DEPRECATED, use Polling Helper instead):
```go
func waitForCondition(t *testing.T, check func() bool, timeout time.Duration) {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if check() {
            return
        }
        time.Sleep(10 * time.Millisecond)
    }
    t.Fatal("condition not met within timeout")
}
```

Used in: `waitForEvent()`, `waitForSession()`, `waitForEventCount()`

---

## Phase 1: Shell Hook Integration (Weeks 3-4)

**Goal**: Transparent AI tool capture with excellent ergonomics

**Design Philosophy**: Shell hook for best UX, but keep modular for fallback options

### Deliverables
- [ ] **Shell hook system**:
  ```bash
  # In ~/.bashrc or ~/.zshrc
  eval "$(prov hook bash)"   # or zsh, fish
  ```

  - Intercepts configured AI commands transparently
  - Similar to `atuin`, `direnv`, or `starship` integration
  - Supports bash, zsh, fish

- [ ] **Hook behavior**:
  - Detects AI tool invocations (configurable patterns)
  - Captures stdin/stdout/stderr
  - Extracts prompt + response (tool-specific heuristics)
  - Sends to daemon via Unix socket
  - Passes through exit codes unchanged
  - Near-zero latency overhead

- [ ] **Fallback wrapper** (modular design):
  ```bash
  # If hooks prove difficult, fall back to:
  prov exec claude-code "task"
  # or alias: alias ai='prov exec claude-code'
  ```

- [ ] **Session-based file tracking**:
  - Start session on first AI invocation
  - Watch git status in background (60s after each prompt)
  - End session on commit OR 30 min timeout
  - Link all prompts in session to ending commit

- [ ] **Pre-configured tool support**:
  - `claude-code`
  - `aider`
  - `cursor` (CLI if available)
  - Generic pattern matching for custom tools

### Success Criteria
- After `eval "$(prov hook bash)"`, AI commands work transparently
- `prov list` shows captured events with correct session IDs
- Sessions auto-end on commit with all prompts linked
- Can disable hook with `prov disable` or env var
- Fallback wrapper works if hooks fail

---

## Phase 2A: Claude Code Hook Integration (Weeks 5-6) ✅ COMPLETE

**Goal**: Deep integration with Claude Code via native hooks system

**Status**: Phase 2A completed on 2026-01-01. All hooks installed and actively capturing events.

**Research completed**: Claude Code provides comprehensive hook system for capturing interactions
- Documentation: https://code.claude.com/docs/en/hooks.md
- Hook events: UserPromptSubmit, PreToolUse, PostToolUse, SessionStart/End
- Configuration: `.claude/settings.json` (project) or `~/.claude/settings.json` (user)

### Deliverables
- [x] **Hook scripts** (Python/Shell):
  - [x] `UserPromptSubmit` hook: Captures user prompts with session context
  - [x] `PreToolUse` hook: Logs tool invocations (Read, Write, Edit, Bash, etc.)
  - [x] `PostToolUse` hook: Logs tool results and outcomes
  - [x] `SessionStart`/`SessionEnd` hooks: Track session boundaries (placeholder for Phase 0 session management)

- [x] **Hook implementation**: Python scripts embedded in binary using Go's `embed` package
  - Hook scripts stored in `cmd/prov/hooks/` as actual `.py` files (not string literals)
  - Full syntax highlighting and linting support during development
  - `{{PROV_PATH}}` template replaced with full path to `prov` binary at install time
  - No PATH dependency - hooks work regardless of prov installation location
  - Implementation: `cmd/prov/commands.go:22-34`, hook scripts: `cmd/prov/hooks/*.py`

- [x] **Configuration installation**:
  ```bash
  prov install-hooks claude-code
  # Installs hook scripts to ~/.ai-provenance/hooks/
  # Updates ~/.claude/settings.json with hook configuration
  ```

- [x] **Hook configuration**: Automatically added to `~/.claude/settings.json`
  - `UserPromptSubmit`: Captures user prompts with full context
  - `PreToolUse`: Logs tool invocations before execution
  - `PostToolUse`: Logs tool results after execution
  - All hooks include full path to installed scripts (no PATH dependency)

- [x] **CLI enhancements**:
  ```bash
  prov install-hooks claude-code      # Install Claude Code hooks
  prov capture-hook --json <data>     # Receive hook data from stdin
  prov hooks status                   # Show installed hooks
  prov hooks uninstall claude-code    # Remove hooks
  ```
  - Implementation: `cmd/prov/commands.go`, tests: `cmd/prov/hooks_test.go`
  - 25/25 tests passing (including hook path verification)

- [x] **Enhanced event capture**:
  - [x] Tool use sequences (chain of Read → Edit → Bash)
  - [x] Permission mode context (auto, ask, disabled)
  - [x] Working directory tracking
  - [ ] Session transcripts (deferred - requires session management from Phase 0)

### Success Criteria
- [x] After `prov install-hooks claude-code`, all prompts captured automatically
- [x] Tool invocations logged with inputs and outputs
- [x] Hooks don't block Claude Code execution (async subprocess, capture_output=True)
- [x] Works on Linux/WSL2 (macOS untested but should work)
- [x] User can disable via `prov hooks uninstall` or remove from `.claude/settings.json`
- [x] No PATH dependency - hooks embed full path to prov binary
- [x] Test coverage: 25/25 tests passing
- [ ] Session boundaries properly tracked (deferred to Phase 0 session management)

### Technical Notes
**Hook Input Schema** (from Claude Code):
```json
{
  "hook_event_name": "UserPromptSubmit",
  "session_id": "ses-123",
  "prompt": "Implement authentication",
  "cwd": "/path/to/project",
  "permission_mode": "auto",
  "transcript_path": "~/.claude/projects/.../session.jsonl"
}
```

**Design Decision**: Hooks vs MCP Server
- **Hooks chosen as primary**: Better coverage (captures prompts, tools, sessions)
- **MCP server as optional**: Only captures tool invocations Claude makes, misses user prompts
- Hook scripts send events to daemon via `prov capture-hook` CLI command

**Related Documentation**:
- Hooks guide: https://code.claude.com/docs/en/hooks-guide.md
- Hook reference: https://code.claude.com/docs/en/hooks.md
- MCP integration: https://code.claude.com/docs/en/mcp.md (deferred to optional track)

### Implementation Improvements

**Python Script Embedding** (2026-01-01):
- **Problem**: Hook scripts were embedded as Go string literals, losing syntax highlighting and linting
- **Solution**: Use Go's `embed` package with actual `.py` files in `cmd/prov/hooks/`
- **Benefits**:
  - Full Python IDE support during development
  - Easier to maintain and modify
  - Template-based path injection via `{{PROV_PATH}}` placeholder
  - Scripts still embedded in single binary for distribution
- **Implementation**: `cmd/prov/commands.go:22-34` uses `//go:embed` directives

**PATH Dependency Fix** (2026-01-01):
- **Problem**: Hook scripts called `prov capture-hook` without full path, failed when prov not in PATH
- **Root cause**: Hooks run as subprocesses in Claude Code's environment, which may not include prov's location
- **Solution**: Inject full path to prov binary at install time
  - Template: `['{{PROV_PATH}}', 'capture-hook', '--json']`
  - Install time: `{{PROV_PATH}}` → `/home/user/projects/go/provenance/prov`
  - Hooks work regardless of prov installation location
- **Test coverage**: `TestInstallHooksEmbedProvPath` verifies full path embedding
- **User benefit**: No need to add prov to PATH or modify shell environment

**Error Handling** (2026-01-01):
- Added proper error checking for `os.Getwd()` in `captureHook()` (commands.go:653)
- Added proper error checking for `json.Marshal()` in tool event capture (commands.go:677)
- Graceful fallbacks prevent silent failures

---

## Phase 2B: Generic VS Code Extension (Weeks 7-9)

**Goal**: Support other AI tools in VS Code (Copilot, Continue, Codeium, etc.)

**Note**: This is separate from Phase 2A Claude Code integration, which uses native hooks

### Deliverables
- [ ] **TypeScript extension**:
  - Hooks into workspace AI providers (GitHub Copilot, Continue, etc.)
  - Captures chat panel interactions
  - Captures inline suggestions (if API available)
  - Sends events to daemon via HTTP (fallback if Unix socket unavailable on Windows)

- [ ] **Workspace context enrichment**:
  - Open files, active editor, selection ranges
  - Prompt type inference (chat vs inline vs edit)

- [ ] **Status bar indicator**:
  - Shows active session ID
  - Click to disable for sensitive work
  - Shows connection status to daemon

- [ ] **Settings**:
  ```json
  "aiProvenance.enabled": true,
  "aiProvenance.daemonUrl": "http://127.0.0.1:8765",
  "aiProvenance.redactSecrets": true
  ```

### Success Criteria
- VS Code + Copilot chat logged automatically
- Workspace files captured in event
- Can disable via status bar (opt-out works)
- Works on Windows + macOS + Linux

---

## Phase 3: Neovim Plugin (Weeks 7-9)

**Goal**: Support tmux/Neovim workflows (your use case + power users)

### Deliverables
- [ ] **Lua plugin**: `nvim-ai-provenance`
  - Integrates with popular AI plugins:
    - `copilot.vim`
    - `codeium.nvim`
    - `avante.nvim`
    - Generic command wrapper (`:AIPrompt`)

  - Captures buffer context, visual selections
  - Sends to daemon via Unix socket

- [ ] **Buffer tracking**:
  - Files open in buffers
  - Active buffer + cursor position
  - Dirty buffers (unsaved changes)

- [ ] **Status line integration**:
  ```vim
  set statusline+=%{ai_provenance#status()}
  ```
  Shows active session, prompt count

- [ ] **Commands**:
  ```vim
  :AIProvenanceEnable
  :AIProvenanceDisable
  :AIProvenanceSession   " Show current session ID
  ```

### Success Criteria
- Using `:AIPrompt <text>` logs to daemon
- Copilot inline suggestions captured (if API allows)
- Works in tmux sessions
- No performance impact on large buffers

---

## Phase 4: Enhanced Analytics CLI (Weeks 10-12)

**Goal**: Make the data actionable for tech leads

**Status**: Breaking into 6 independent features for incremental delivery

### Feature Breakdown (Recommended Order)

#### Feature 1: Session Query Commands ✅
**Goal**: Make sessions queryable and useful

**Commands:**
```bash
prov session list                    # List all sessions
prov session list --active           # Show only active sessions
prov session show <id>               # Show session details + all prompts
prov session end <id>                # Manually end a session
```

**Database work:**
- Queries on existing `sessions` and `prompt_events` tables
- No schema changes needed

**Value**: Understand what sessions exist, when they started/ended, what happened in them

**Estimated complexity**: Low (mostly SQL queries + formatting)

---

#### Feature 2: Basic Statistics Commands ✅ COMPLETE
**Goal**: Aggregate usage metrics

**Storage layer implementation** (COMPLETE):
```go
// Aggregate statistics for a repository
func GetRepoStats(db *sql.DB, repoPath string) (*RepoStats, error)

// Statistics for a specific session
func GetSessionStats(db *sql.DB, sessionID string) (*SessionStats, error)

// Time-filtered statistics
func GetTimeframeStats(db *sql.DB, repoPath string, since time.Time) (*RepoStats, error)
```

**CLI implementation** (COMPLETE):
```bash
prov stats                    # Repository-level statistics
prov stats --session <id>     # Session-specific statistics
prov stats --since "7 days ago"  # Time-filtered statistics
```

**RepoStats includes:**
- Total prompts, tokens in/out
- Session count
- File mention counts (which files were worked on)
- Tool usage distribution (read_file, write_file, bash, etc.)

**SessionStats includes:**
- Session-specific prompt/token metrics
- Session timing (start/end times)
- File mentions within session
- Tool usage for session

**Database work:** ✅
- Aggregation queries (COUNT, SUM, GROUP BY)
- JSON array unpacking for file/tool aggregation
- Time-based filtering

**CLI work:** ✅
- Command routing and flag parsing
- Time string parsing ("7 days ago", "2 weeks ago", etc.)
- Formatted output with top 10 files and tools
- Integration tests (4/4 tests passing)

**Value**: Foundation for analytics CLI - provides queryable metrics about AI tool usage

**Estimated complexity**: Low-Medium (SQL aggregations + JSON parsing)

**Status**: Complete - Storage layer + CLI commands fully implemented with comprehensive test coverage (9 test functions, all passing)

---

#### Feature 3: Export Functionality
**Goal**: Extract data for external analysis

**Commands:**
```bash
prov export --format json            # Export all data as JSON
prov export --format csv             # Export as CSV
prov export --session <id>           # Export specific session
prov export --since "2 weeks ago"    # Time-filtered export
prov export --output report.json     # Custom output file
```

**Database work:**
- SELECT queries with filters
- JSON/CSV serialization

**Value**: Enable custom analysis in spreadsheets, notebooks, BI tools

**Estimated complexity**: Medium (format conversion, streaming for large datasets)

---

#### Feature 4: Git Post-Commit Hook (The Big One)
**Goal**: Automatically link prompts to commits

**Implementation:**
```bash
prov install-hook post-commit        # Install git hook
```

**Hook behavior:**
- On commit, find recent session (or prompts from last 10 minutes)
- Create `change_sets` entries linking prompts → commit
- Calculate confidence based on:
  - Time delta (closer = higher)
  - File overlap (mentioned files in prompt vs changed files)

**Database work:**
- Implement `change_sets` table (already in schema!)
- Store prompt → commit associations
- Confidence scoring logic

**Value**: **This is the killer feature** - trace code back to AI prompts

**Estimated complexity**: High (git hook, correlation logic, confidence scoring)

---

#### Feature 5: Blame/Trace Commands
**Goal**: Reverse lookup - code → prompts

**Commands:**
```bash
prov blame <file>                    # Prompts that touched this file
prov blame <commit>                  # Prompts linked to this commit
prov trace <file> --lines 10-20      # Prompts for specific lines (future)
```

**Database work:**
- Query `change_sets` table
- Join with `prompt_events` for full context

**Value**: "Who (or what AI) wrote this code?"

**Estimated complexity**: Medium (depends on Feature 4 completion)

**Dependency**: Requires Feature 4 (git correlation)

---

#### Feature 6: Manual Tagging
**Goal**: User corrections for correlation

**Commands:**
```bash
prov tag <prompt-id> --commit <sha>  # Manual association
prov tag <prompt-id> --file <path>   # Associate with file
prov untag <prompt-id> <commit>      # Remove association
```

**Database work:**
- Insert into `change_sets` with confidence = 1.0
- Mark as `correlation_method = 'manual'`

**Value**: Override auto-correlation when it's wrong

**Estimated complexity**: Low (simple database inserts)

**Dependency**: Requires Feature 4 schema

---

### Overall Success Criteria
- [x] Phase broken into 6 independent features
- [x] Feature 1: Session queries (1-2 hours) - COMPLETE
- [x] Feature 2: Basic statistics commands (2-3 hours) - COMPLETE
- [ ] Feature 3: Export (2-4 hours)
- [ ] Feature 4: Git correlation (6-8 hours)
- [ ] Feature 5: Blame commands (2-3 hours)
- [ ] Feature 6: Manual tagging (1-2 hours)

**Total estimated time**: 14-22 hours across 6 features
**Completed**: 5-7 hours (Features 1-2)

---

## Phase 5: Team Aggregation (Weeks 13-16)

**Goal**: Aggregate insights across team without sacrificing privacy

### Deliverables
- [ ] **Aggregation strategy**:
  - Developers opt-in to sync anonymized/redacted data
  - `prov sync --team <team-id>` pushes to shared Postgres
  - Alternatively: `prov export` → centralized analysis server

- [ ] **Team dashboard** (basic):
  - Web UI (Go + htmx or static site)
  - Metrics:
    - Token usage trends
    - Most active repos/files
    - Agent/model distribution
    - Prompt patterns (common keywords)

- [ ] **Privacy controls**:
  - Per-user redaction rules enforced before sync
  - Option to exclude prompt/response text (metrics only)
  - Admin-defined retention policies

- [ ] **Cost tracking**:
  - Configurable pricing per model
  - Estimated monthly costs
  - Alert on unusual usage spikes

### Success Criteria
- Small team (5-10 devs) can aggregate data securely
- Dashboard shows actionable insights without exposing sensitive code
- Developers trust the privacy controls

---

## Phase 6: Advanced Features (Weeks 17+)

**Goal**: Power features for mature adoption

### Potential Features
- [ ] **Prompt quality scoring**:
  - ML model trained on outcomes (accepted vs reverted changes)
  - Suggests prompt improvements

- [ ] **Diff heatmaps**:
  - Visualize which lines came from AI vs human
  - Annotated `git blame` output

- [ ] **CI/CD integration**:
  - On test failure, show prompts from affected commits
  - `prov bisect --bad <sha>` helper

- [ ] **LLM linting**:
  - Warn before sending prompts with common anti-patterns
  - Team-defined prompt templates

- [ ] **Review workflow**:
  - `ReviewEvent` table populated from PR approvals
  - Correlate reviewer feedback with prompts

- [ ] **Postgres migration**:
  - For teams outgrowing SQLite
  - Retain local SQLite for fast queries, sync to Postgres

---

## MCP Server (Optional Track - Deferred)

**Goal**: Provide audit/query capabilities via MCP for Claude to query its own interaction history

**Research findings**: MCP servers are **tool providers**, not middleware for observing prompts
- Cannot intercept or observe prompts/responses at transport layer
- Can only log tool invocations that Claude makes *through* the MCP server
- **Better approach**: Use Claude Code hooks (Phase 2A) for comprehensive capture

### Potential Use Case (Future)
If useful, an MCP server could provide:
- Query tools for Claude to search its own provenance history
- Audit tools to check "what prompts modified file X"
- Statistics tools to report token usage

### Deliverables (if pursued)
- [ ] **Python MCP server**: `mcp-ai-provenance`
  - Provides tools: `query_prompts`, `get_file_history`, `get_session_stats`
  - Queries local SQLite database
  - Returns provenance data to Claude

- [ ] **Installation**:
  ```bash
  prov install-mcp  # Adds to ~/.claude/.mcp.json
  ```

- [ ] **Example tool definition**:
  ```python
  @mcp.tool()
  def query_prompts(query: str, limit: int = 10):
      """Search provenance database for prompts matching query"""
      # Query local SQLite database
      # Return results to Claude
  ```

### Success Criteria
- Claude can query its own interaction history via MCP tools
- Zero latency impact on normal operations
- Useful for debugging "when did I ask about X?"

**Decision**: Deferred - Phase 2A Claude Code hooks provide comprehensive capture without needing MCP

---

## Testing Strategy

### Per Phase
- **Unit tests**: Core logic (Go packages, TS modules)
- **Integration tests**: Daemon + CLI interactions
- **E2E tests**: Full workflows (wrapper script → query results)

### Cross-Phase
- **Performance testing**: 1000s of events, query latency
- **Privacy audits**: Ensure redaction works correctly
- **Security review**: SQLite injection prevention, socket permissions

---

## Documentation Plan

### Developer Docs
- `README.md`: Quick start, installation
- `ARCHITECTURE.md`: Technical deep-dive
- `CONTRIBUTING.md`: How to add adapters, report bugs

### User Docs
- **Quick start guide**: 5-minute setup per IDE
- **CLI reference**: All commands + flags
- **Privacy & security**: What's captured, how to redact
- **Team deployment**: Rollout guide for tech leads

### Examples
- Jupyter notebook: Analyzing exported data
- Dashboard screenshots
- Prompt quality analysis workflow

---

## Success Metrics

### Adoption
- 100 developers using it daily (6 months)
- 10 organizations with team deployments (1 year)

### Community
- 1000 GitHub stars
- Active contributors adding adapters
- Blog posts / conference talks

### Impact
- Documented case studies:
  - "Reduced AI-introduced bugs by 40%"
  - "Identified high-ROI prompt patterns"
  - "Saved $X in wasted tokens"

---

## Open Source Strategy

### Licensing
- **MIT or Apache 2.0**: Permissive for commercial adoption

### Repository Structure
```
provenance/
├── cmd/prov/           # CLI + daemon
├── internal/           # Core Go packages
│   ├── storage/
│   ├── git/
│   └── correlation/
├── adapters/
│   ├── vscode/         # TypeScript extension
│   ├── nvim/           # Lua plugin
│   ├── wrapper/        # Shell scripts
│   └── mcp/            # Python MCP server
├── docs/
├── examples/
└── tests/
```

### Community Building
- **Discord/Slack**: For users + contributors
- **Monthly roadmap updates**: Transparent development
- **Bounties**: For priority adapters (IntelliJ, Emacs, etc.)
- **Case studies**: Highlight successful deployments

---

## Risk Mitigation

### Technical Risks
| Risk | Mitigation |
|------|-----------|
| High latency impact | Benchmark per-phase, async logging, batching |
| Secrets in prompts | Built-in redaction, pre-send hooks, user review |
| Storage bloat | Compression, retention policies, blob external storage |
| Cross-platform issues | CI testing on Linux/macOS/Windows |

### Adoption Risks
| Risk | Mitigation |
|------|-----------|
| Too invasive | Make opt-out trivial, status indicators |
| Perceived surveillance | Emphasize local-first, open source code |
| Low value perception | Ship analytics early, show ROI examples |

---

## Next Steps

1. **Validate design**: Get feedback from potential users (tech leads, devs)
2. **Spike Phase 0**: Prove Unix socket + SQLite performance
3. **Create demo video**: Show vision for adoption pitch
4. **Set up repo**: GitHub, CI, initial docs
5. **Start Phase 0**: Build the foundation

---

## Design Decisions (Resolved)

### Core Design Philosophy
**Decision**: **Sensible defaults, highly configurable**
- 80/20 rule: Works out-of-the-box for most users
- Power users can customize every behavior
- Progressive disclosure: simple → powerful as needed

### Correlation Strategy
**Decision**: Session-based with hybrid boundaries
- Primary: All prompts in session → commit that ends session
- Session ends on: commit OR 30-minute timeout (configurable) OR manual end
- Fallback: 60s file watch window for individual prompts
- Can expand to ML-based correlation later

**Rationale**: Balances simplicity (easy to implement) with flexibility (can improve over time)

### Storage Architecture
**Decision**: Global index + per-repo stores (default), configurable
- Default: `~/.ai-provenance/index.sqlite` + `.ai-provenance/db.sqlite` per repo
- Alternative: Global-only or per-repo-only (user choice)
- Migration strategy: `golang-migrate` for schema versions

**Rationale**: Enables per-repo isolation while maintaining cross-repo queries

### Privacy Defaults
**Decision**: Log by default, opt-out available
- Users who install the tool intend to use it
- Easy disable: `prov disable` or `AI_PROVENANCE_DISABLED=1`
- Per-repo configuration supported

**Rationale**: Maximize utility while respecting user choice

### Redaction Strategy
**Decision**: Builtin patterns + user extensions
- Ship with common secret patterns (AWS keys, private keys, tokens)
- Users can add custom regex patterns
- Users can disable builtins if desired

**Rationale**: Secure by default, extensible for specific needs

### Integration Approach
**Decision**: Shell hook with modular fallback
- Primary: `eval "$(prov hook bash)"` for transparency
- Fallback: `prov exec <cmd>` if hooks prove difficult
- Keep implementation modular to pivot if needed

**Rationale**: Best UX via hooks, but prepared for edge cases

### Daemon Lifecycle
**Decision**: Auto-start initially, OS service management later
- Phase 0-1: Auto-start daemon on first `prov` command
- Phase 4+: Systemd/launchd integration for proper service management
- Always allow manual control (`prov daemon start/stop`)

**Rationale**: Lowest friction for initial adoption, proper integration later

### Write Performance
**Decision**: Immediate writes initially, batched writes later (configurable)
- Phase 0-1: Write each event immediately (simpler, easier to debug)
- Phase 4+: Configurable batching (N events or M seconds)
- Profile first, optimize if needed

**Rationale**: Premature optimization avoided, informed by real-world usage

### Windows Support
**Decision**: Design for it, test later (Phase 5+)
- Use HTTP fallback for daemon communication (not just Unix sockets)
- Test on Linux/macOS first
- Avoid Unix-specific assumptions in core logic

**Rationale**: Focus on majority (Unix) users first, but don't block future Windows support

---

## Questions to Resolve

- [ ] **Pricing data**: Where to source token costs per model? (Static config vs API?)
- [ ] **VS Code API limits**: Can we hook into all AI providers or just specific ones?
- [ ] **Neovim plugin distribution**: Plugin managers (lazy.nvim, packer) vs manual install?
- [ ] **Team aggregation**: Custom protocol or leverage existing tools (Prometheus, etc.)?
- [ ] **Shell hook implementation**: Use preexec hooks or command wrapping? Test with different shells.
- [ ] **Git hook robustness**: How to handle pre-commit failures, rebases, squashes?

---

*Last updated: 2026-01-03 - Phase 0 complete (session management & configuration system), Phase 2A complete (Claude Code hooks)*
