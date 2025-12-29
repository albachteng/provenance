# Agentic AI Prompt Provenance – MVP Design

## Problem Statement

Agentic AI tools (e.g. Claude, Copilot, Cursor) introduce productivity gains but also new risks:

* Hallucinated or misunderstood changes
* Subtle regressions introduced across refactors
* Loss of attribution: *which prompt caused which change?*
* Difficulty quantifying ROI or understanding which prompts/tasks work well

The goal is to **capture high‑fidelity provenance** for AI‑assisted code changes with minimal disruption to developer workflows.

## Target Audience

**Primary**: Tech leads and engineering organizations seeking:
- AI usage analytics across teams
- ROI metrics for AI tooling investments
- Attribution chains for debugging AI-introduced bugs
- Best practices for prompt engineering at scale
- Compliance and governance trails

**Secondary**: Individual developers wanting to understand their own AI usage patterns

## Design Philosophy

This is an **open-source, agent-agnostic, local-first observability layer** for AI-assisted development.

**Core Principle**: **Sensible defaults, highly configurable**
- Out-of-the-box functionality that works for 80% of users
- Every behavior customizable via configuration files
- Progressive disclosure: simple by default, powerful when needed

---

## Core Idea

Introduce a **multi-adapter capture system** that:

* Records AI prompts and responses from any AI tool (not just MCP-compatible ones)
* Correlates them with **Git state**, **sessions**, and **review events**
* Produces an auditable trail usable for debugging, analytics, and governance
* Stores data locally with optional team aggregation

The system should be:

* **Agent-agnostic**: Works with Claude Code, Cursor, Copilot, Aider, etc.
* **Non-invasive**: Developers barely notice it's running
* **Local-first**: Per-developer CLI tool, aggregate data later
* **Privacy-respecting**: Log by default with easy opt-out, built-in redaction
* **Useful even if partially adopted**: Value from day one

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Capture Adapters                      │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────┐ │
│  │ Shell    │  │ VS Code  │  │ Neovim   │  │ MCP Srv │ │
│  │ Hook     │  │ Extension│  │ Plugin   │  │(optional)│ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬────┘ │
└───────┼─────────────┼─────────────┼─────────────┼──────┘
        │             │             │             │
        └─────────────┴─────────────┴─────────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │  Local Storage Daemon  │
            │  (Go, Unix Socket/HTTP)│
            └────────────┬───────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │   SQLite Database      │
            │  ~/.ai-provenance/     │
            │  (per-repo optional)   │
            └────────────┬───────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │   CLI Analytics        │
            │   prov stats/blame     │
            └────────────────────────┘
                         │
                         ▼ (Phase 5+)
            ┌────────────────────────┐
            │  Team Aggregation      │
            │  (Postgres optional)   │
            └────────────────────────┘
```

### Component Strategy

| Component           | Language     | Rationale                                    |
| ------------------- | ------------ | -------------------------------------------- |
| Core daemon + CLI   | Go           | Fast, single binary, Git plumbing, concurrency |
| Shell hook          | Shell/Go     | Portable, minimal dependencies               |
| VS Code extension   | TypeScript   | Native to VS Code APIs                       |
| Neovim plugin       | Lua          | Native to Neovim                             |
| MCP server (opt)    | Python       | MCP SDK compatibility                        |

### Performance Considerations

**Go Garbage Collector**: For this daemon use case, Go's GC should not be a bottleneck:
- Not real-time or ultra-low-latency critical
- Main performance concerns are SQLite I/O and disk writes
- Mitigation strategies if needed:
  - Use `GOGC` environment variable to tune GC frequency
  - Use `sync.Pool` for frequently allocated objects (JSON buffers)
  - Profile with `pprof` to verify no issues
  - Batch writes to reduce syscall overhead

**Primary Performance Focus**:
- SQLite WAL mode for concurrent access
- Batched writes (flush every N events or 5 seconds)
- Async logging to avoid blocking AI tool responses
- Compression for large prompt/response text

---

## Data Model (MVP)

### `PromptEvent`

Represents a single AI interaction.

```ts
PromptEvent {
  id: UUID
  timestamp: time
  session_id: UUID

  // AI metadata
  agent: string            // 'claude-code', 'cursor', 'copilot'
  model_version: string
  prompt_text: string
  response_text: string

  // Usage metrics
  tokens_in: int
  tokens_out: int
  latency_ms: int

  // Git context
  repo_path: string
  git_commit: string       // HEAD at prompt time
  git_branch: string
  git_dirty: bool
  dirty_files: string[]    // Specific uncommitted files

  // Developer context
  author: string           // OS user / git user
  ide: string              // 'vscode', 'nvim', 'cli'
  active_file: string      // File with focus
  workspace_files: string[] // Open files in IDE

  // Categorization
  prompt_type: string      // 'chat', 'inline', 'edit', 'debug'
  tools_invoked: string[]  // MCP tools called (if applicable)
  files_mentioned: string[] // Files referenced in prompt
}
```

### `Session`

Groups related prompts together for correlation.

```ts
Session {
  id: UUID
  start_time: time
  end_time: time           // null if active
  repo_path: string
  total_prompts: int
  total_tokens: int

  // Session boundaries (hybrid approach)
  ended_by: string         // 'commit' | 'timeout' | 'manual'
}
```

**Session Lifecycle**:
- **Auto-start**: On first prompt in a repo
- **Auto-end**: On commit OR 30 minutes of inactivity
- **Manual override**: `prov session start/end`

This enables **session-based correlation**: all prompts in a session are assumed related to the eventual commit.

### `ChangeSet` (Derived)

Maps prompts/sessions to code changes.

```ts
ChangeSet {
  id: UUID
  prompt_id: UUID          // Can be null if session-level
  session_id: UUID
  timestamp: time

  files_changed: string[]
  diff_summary: string     // "+10 -5" style
  commit_introduced: string

  // Correlation metadata
  correlation_method: string  // 'session' | 'file_watch' | 'git_hook' | 'manual'
  confidence: float        // 0.0 - 1.0
  time_to_first_change_ms: int
}
```

**Correlation Strategy**:
1. **Session-based (primary)**: All prompts in session → commit that ends session
2. **File watch (fallback)**: Watch git status for 60s after prompt
3. **Git hook (enhancement)**: `post-commit` hook links recent prompts
4. **Manual (override)**: `prov tag <prompt-id> --commit <sha>` for explicit linking

### `RedactionRule`

User-defined patterns for privacy.

```ts
RedactionRule {
  id: UUID
  pattern: regex           // e.g., AWS key patterns
  replacement: string      // '[REDACTED]'
  scope: string            // 'prompt' | 'response' | 'both'
  enabled: bool
}
```

### `ReviewEvent` (Optional, Phase 5+)

```ts
ReviewEvent {
  commit: string
  reviewer: string
  approved: bool
  timestamp: time
  comments: string[]
}
```

---

## MVP Scope

### Included

* **Shell hook** for CLI-based AI tools (Claude Code, Aider, etc.)
* **Git state capture** (HEAD, branch, dirty files, diff summaries)
* **Local SQLite storage** with WAL mode
* **Session management** (auto-start/end with timeout and commit boundaries)
* **CLI tools**:
  * `prov init` - Initialize storage
  * `prov daemon start/stop` - Control background daemon
  * `prov list/show/search` - Query prompts
  * `prov session` - Manage sessions
  * `prov stats` - Usage analytics
* **VS Code extension** (Phase 2)
* **Neovim plugin** (Phase 3)

### Excluded (Initially)

* MCP server (optional track, not required for core functionality)
* Cloud sync / team aggregation (Phase 5+)
* Full PR/GitHub integration (Phase 6+)
* Automatic bug classification (Phase 6+)
* Enforcement / blocking (intentionally avoided - observational only)
* Windows support (designed for, but tested on Linux/macOS first)

---

## Non-Invasive Integration Patterns

### Shell Hook (CLI Tools)

* **Mechanism**: Shell integration similar to `atuin` or `direnv`
* **Activation**: Add to `.bashrc`/`.zshrc`:
  ```bash
  eval "$(prov hook bash)"  # or zsh, fish, etc.
  ```
* **Behavior**: Intercepts AI tool invocations transparently
* **Modularity**: Can fallback to explicit wrapper (`prov exec <cmd>`) if hooks prove difficult

**Benefits**:
- Zero workflow change after initial setup
- Works with any CLI AI tool
- Easy to disable (`unset` or comment out)

### Neovim

* Lua plugin installed via plugin manager
* Integrates with existing AI plugins (Copilot, Avante, etc.)
* Captures buffer context automatically
* Sends events to daemon via Unix socket

**No manual invocation needed** - works in background

### VS Code

* Lightweight extension from marketplace
* Hooks into AI provider APIs
* Status bar indicator (shows session, allows quick disable)
* Background capture with opt-out button

### Design Principle

> **If the developer notices it, it's too invasive**

The system is **observational, not interventional** - it never blocks, warns, or modifies AI behavior.

---

## Git Integration Strategy

### Passive Capture (Phase 0-1)

* Record **HEAD commit** at prompt time
* Capture **dirty state**: which files have uncommitted changes
* Capture **branch name** and remote tracking info
* Compute **diff summary** (`+X -Y lines`)

### Session-Based Correlation (Phase 1)

* **Session → Commit mapping**: When session ends via commit, link all prompts in that session to the commit
* **Confidence scoring**: Higher confidence for prompts closer to commit time
* **File overlap**: Boost confidence if files mentioned in prompt match files in commit

### Active Integration (Phase 4+)

* **Git hooks**: `post-commit` hook automatically links recent prompts
* **Annotated commits** (optional): Add `AI-Session-ID: <uuid>` to commit messages
* **Bisect helper**:
  ```bash
  prov bisect --bad abc123 --good def456
  ```
  Identifies prompts correlated with first bad commit

---

## Edge Cases to Consider

### Prompt / Code Mismatch

* Prompt spans multiple commits
* Developer edits AI output manually
* Partial application of suggestions

### Git Ambiguities

* Rebase / force push breaks hashes
* Squashed commits lose granularity

### Human Factors

* Shared machines
* Pair programming
* Prompt copy/paste outside IDE

### Performance

* Large prompt payloads
* High‑frequency agent calls
* Token cost vs logging cost

---

## Overhead & Risk

### Performance

* MCP proxy adds network hop (ms-level)
* Disk writes (batching recommended)

### Cognitive

* Risk of chilling experimentation
* Must remain **observational**, not punitive

### Privacy & Security

**Default Behavior**: **Log by default, opt-out available**
- Users who install this tool intend to use it
- Easy disable: `prov disable` or `AI_PROVENANCE_DISABLED=1`
- Per-repo configuration: `.ai-provenance/config.yaml`

**Secret Protection**:
* Built-in redaction patterns (AWS keys, API tokens, etc.)
* User-defined redaction rules via regex
* Pre-commit review option: `prov review-session`
* Redaction applied before storage (not after)

**Storage Security**:
* SQLite database in `~/.ai-provenance/` with restricted permissions (600)
* Optional encryption-at-rest (SQLite SEE or sqlcipher)
* Clear data ownership - local-first, user controls retention

**Configuration Hierarchy**:
```yaml
# ~/.ai-provenance/config.yaml (global defaults)
enabled: true
redact_secrets: true
log_response_text: true

# Session configuration
session:
  timeout_minutes: 30          # Auto-end after inactivity
  end_on_commit: true           # End session on git commit

# Storage configuration
storage:
  location: "global-index"      # "global-index" | "per-repo" | "global-only"
  db_path: "~/.ai-provenance/db.sqlite"

# Write batching (Phase 4+)
batching:
  enabled: false                # Start with immediate writes
  max_events: 10
  max_delay_seconds: 5

# Redaction rules
redaction:
  builtin_patterns: true        # AWS keys, tokens, etc.
  custom_patterns:
    - pattern: 'MY_SECRET_\w+'
      replacement: '[REDACTED]'

# /path/to/repo/.ai-provenance/config.yaml (per-repo override)
enabled: false  # Disable for sensitive repos
storage:
  location: "per-repo"          # Override to use local DB
```

---

## Improvements & Extensions

* Prompt quality scoring vs outcomes
* Diff‑level attribution heatmaps
* Team-level analytics dashboards
* Integration with CI failures
* LLM prompt linting / guardrails

---

## Why This Works

* Treats AI like a **junior engineer with a paper trail**
* Separates capture from judgment
* Leverages Git as the source of truth
* Respects existing developer workflows

---

## TL;DR

Capture **who asked what**, **at which commit**, **with what outcome**, and **at what cost** — without getting in the way. This system creates the missing observability layer for agentic software development.
