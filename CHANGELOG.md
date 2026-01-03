# Changelog

All notable changes to AI Provenance will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
