# Changelog

All notable changes to AI Provenance will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added - Phase 4 Feature 4 Complete (2026-01-11)

**Git Commit Correlation**
- Automatic linking of git commits to AI prompts with confidence scoring
- Storage layer for change_sets:
  - CreateChangeSet, GetChangeSet, GetChangeSetsForPrompt, GetChangeSetsForCommit
  - Foreign key constraints to sessions and prompt_events
  - JSON array support for files_changed
  - 6 comprehensive tests passing
- Correlation algorithms:
  - CalculateTimeConfidence: Piecewise decay (< 30s: 0.95-1.0, < 2min: 0.85-0.95, < 10min: 0.45-0.70)
  - CalculateFileOverlapConfidence: Perfect: 0.975, high (2/3+): 0.75, partial: 0.40
  - CombineConfidenceFactors: Weighted average (40% time, 60% file overlap)
  - CorrelateCommitToPrompts: Full workflow with 15-minute time window
  - 5 test suites, 25+ test cases passing
- Git hook integration:
  - `prov install-hook post-commit` - Install hook in current repository
  - `prov correlate-commit <sha> <repo>` - Manual correlation (called by hook)
  - `prov hooks status` - Updated to show both claude-code and git hooks
  - `prov hooks uninstall post-commit` - Remove git hook
  - Embedded post-commit.sh script with PROV_PATH injection
  - 6 tests passing (install, uninstall, overwrite, status, error handling)
- Test coverage: 137 tests passing across all packages

**Session Management Fix**
- Git state polling: Added UpdateGitState() method to session manager
- Background polling integrated with daemon's session check loop
- Enables GitEventStrategy to detect commits and branch switches
- 4 new tests for git state tracking

### Added - Phase 4 Features 1-3 Complete (2026-01-08)

**Session Query Commands**
- `prov session list` - List all sessions with summary information
- `prov session list --active` - Show only active sessions
- `prov session show <id>` - Detailed session view with all prompts
- `prov session end <id>` - Manually end a session
- Session statistics in list view (prompts, tokens, duration)
- Integration tests: 4/4 tests passing

**Statistics Commands**
- `prov stats` - Repository-level aggregate statistics
- `prov stats --session <id>` - Session-specific statistics
- `prov stats --since "7 days ago"` - Time-filtered statistics
- Metrics include: total prompts, tokens in/out, session count, top files, top tools
- Storage layer: GetRepoStats, GetSessionStats, GetTimeframeStats
- Integration tests: 9/9 tests passing (storage + CLI)

**Export Functionality**
- `prov export` - Export all data as JSON (default)
- `prov export --format csv` - Export as CSV
- `prov export --session <id>` - Export specific session
- `prov export --since "7 days ago"` - Time-filtered export
- `prov export --output file.json` - Write to file
- CSV format: Flat structure with semicolon-separated arrays
- JSON format: Complete nested structure
- Integration tests: 6/6 tests passing

### Added - Phase 0 Complete (2026-01-03)

**Session Management**
- Session manager with pluggable strategy pattern
- Two session strategies:
  - `smart-time`: Activity-based sessions with configurable timeout (default: 30m)
  - `git-event`: Commit/branch-based sessions with fallback timeout (default: 4h)
- Auto-start sessions on first prompt in a repository
- Background session boundary checking (configurable interval, default: 1m)
- LLM activity tracking for smart timeout extension
- Session lifecycle management: StartSession, RecordEvent, EndSession, CheckSessionBoundaries
- Integration with daemon for periodic session checks

**Configuration System**
- Hierarchical configuration with proper merging:
  1. Environment variables (`AI_PROVENANCE_*`)
  2. Per-repository config (`.ai-provenance/config.yaml`)
  3. Global config (`~/.ai-provenance/config.yaml`)
  4. Built-in defaults
- Configuration CLI commands:
  - `prov config show` - Display current merged configuration
  - `prov config init [--global]` - Create config file
  - `prov config validate` - Validate configuration
- Configurable options:
  - Session strategies and timeouts
  - Storage paths (database, socket, PID file, hooks directory)
  - Daemon settings (startup timeout, shutdown timeout, session check interval)
  - Redaction rules (builtin patterns + custom)
- Custom Duration type for YAML marshaling ("30m", "2h", etc.)
- Pointer booleans for explicit false vs unset distinction
- Path resolution (relative → absolute based on provenance home)

**Test Coverage**
- Config package: 19 test functions, 34 subtests passing
- Session package: 13 test functions, 24 subtests passing
- Daemon integration tests: 7 test functions passing
- All tests passing (60+ tests total)

### Changed

- Daemon now requires session manager and config on startup
- Daemon runs background ticker for session boundary checking
- Path helpers use configuration system instead of hard-coded defaults

## [0.2.0] - Phase 2A Complete (2026-01-01)

### Added

**Claude Code Integration**
- Native hook system integration for comprehensive capture
- Hook scripts:
  - `UserPromptSubmit` - Captures user prompts with session context
  - `PreToolUse` - Logs tool invocations (Read, Write, Edit, Bash, etc.)
  - `PostToolUse` - Logs tool results and outcomes
  - Session hooks (placeholders for Phase 0 session management)
- CLI commands:
  - `prov install-hooks claude-code` - Install Claude Code hooks
  - `prov hooks status` - Show installed hooks
  - `prov hooks uninstall claude-code` - Remove hooks
  - `prov capture-hook` - Receive hook data from stdin
- Automatic hook installation to `~/.ai-provenance/hooks/`
- Configuration updates to `~/.claude/settings.json`
- Full path embedding - no PATH dependency required
- Test coverage: 25/25 tests passing

## [0.1.0] - Phase 0 Foundation (2025-12-30)

### Added

**Core Storage Daemon**
- SQLite database with WAL mode
- Schema for prompt events and sessions
- Unix socket server for event ingestion
- Graceful startup/shutdown with socket cleanup
- Concurrent connection handling
- JSON event routing (PromptEvent and SessionEvent)

**CLI Basics**
- Version command with build info
- Daemon lifecycle: `start`, `stop`, `status`
- Query commands: `list`, `show`, `search`
- PID-based daemon management
- Integration tests (9/9 passing)

**Git Integration**
- Capture HEAD, branch, dirty state
- List dirty files with diff summaries
- Compute ahead/behind vs remote
- Comprehensive edge case handling (14 tests)
- Swappable provider pattern

### Technical

- Test-Driven Development (TDD) methodology
- Ready channel pattern for async components
- Polling helper pattern for deterministic testing
- Go embed for hook script distribution
