# Agentic AI Prompt Provenance – MVP Design

## Problem Statement

Agentic AI tools (e.g. Claude, Copilot, Cursor) introduce productivity gains but also new risks:

* Hallucinated or misunderstood changes
* Subtle regressions introduced across refactors
* Loss of attribution: *which prompt caused which change?*
* Difficulty quantifying ROI or understanding which prompts/tasks work well

The goal is to **capture high‑fidelity provenance** for AI‑assisted code changes with minimal disruption to developer workflows.

---

## Core Idea

Introduce a **local MCP-compatible interception layer** that:

* Records AI prompts and responses
* Correlates them with **Git state**, **actors**, and **review events**
* Produces an auditable trail usable for debugging, analytics, and governance

The system should be:

* Language‑agnostic
* IDE‑non‑invasive
* Useful even if partially adopted

---

## High-Level Architecture

```
IDE / Agent (Claude, etc)
        │
        ▼
Local MCP Proxy (Python)
        │
        ▼
Provenance Store (SQLite → Postgres)
        │
        ▼
Analytics / CLI / Git Integrations (Go)
```

### Language Split

| Component              | Language | Rationale                           |
| ---------------------- | -------- | ----------------------------------- |
| MCP proxy / intercept  | Python   | Fast iteration, SDK compatibility   |
| Storage, indexing, CLI | Go       | Concurrency, binaries, Git plumbing |
| IDE integration        | Lua / TS | Native to Neovim / VS Code          |

---

## Data Model (MVP)

### `PromptEvent`

Represents a single AI interaction.

```ts
PromptEvent {
  id: UUID
  timestamp: time
  agent: string            // claude-3.5, gpt-4.1, etc
  model_version: string
  prompt_text: string
  response_text: string
  tokens_in: int
  tokens_out: int
  latency_ms: int
  git_commit: string       // HEAD at prompt time
  git_dirty: bool
  repo_path: string
  author: string           // OS user / git user
  ide: string              // nvim / vscode
  session_id: UUID
}
```

### `ChangeSet` (Derived)

Maps prompts to code changes.

```ts
ChangeSet {
  id: UUID
  prompt_id: UUID
  files_changed: string[]
  diff_hash: string
  commit_introduced: string
}
```

### `ReviewEvent` (Optional, Phase 2)

```ts
ReviewEvent {
  commit: string
  reviewer: string
  approved: bool
  timestamp: time
}
```

---

## MVP Scope

### Included

* MCP proxy that logs prompt/response pairs
* Git snapshot capture (HEAD, dirty state)
* Local SQLite storage
* CLI to:

  * list prompts by commit
  * search prompts by text
  * show prompt → diff mappings
* Neovim + VS Code minimal integration

### Excluded (Initially)

* Cloud sync
* Full PR/GitHub integration
* Automatic bug classification
* Enforcement / blocking

---

## Non-Invasive IDE Flows

### Neovim

* Lua plugin
* Environment variable points agent to MCP proxy
* On save / agent invocation:

  * capture git state
  * send metadata via MCP

**No required workflow change**

### VS Code

* Lightweight extension
* Wrap existing AI provider config
* Background provenance capture

### Design Principle

> **If the developer notices it, it’s too invasive**

---

## Git Integration Ideas

### Passive

* Record HEAD at prompt time
* Detect dirty vs clean

### Active (Later)

* Annotated commits: `ai-prompt-id`
* `git bisect` helper:

  ```bash
  ai-bisect --bad abc123 --good def456
  ```
* Identify prompts correlated with first bad commit

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

### Security

* Prompts may include secrets
* Encryption-at-rest and redaction hooks needed

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
