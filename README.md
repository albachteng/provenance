# AI Provenance

Track AI-assisted code changes across your development workflow. An open-source, agent-agnostic provenance system that helps teams understand AI usage patterns, measure ROI, and maintain code quality.

## Status

🚧 **Phase 0 - Foundation** (In Progress)

Core storage, daemon, and CLI basics are functional. Shell hook integration and IDE extensions coming in later phases.

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

# Stop the daemon
./prov daemon stop
```

The daemon:
- Listens on a Unix socket at `~/.ai-provenance/daemon.sock`
- Stores events in `~/.ai-provenance/db.sqlite`
- Runs in the background until stopped

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

### Environment Configuration

Override the default storage location:

```bash
export AI_PROVENANCE_HOME=/custom/path
./prov daemon start
```

## Architecture

```
┌────────────────────────────────────────────────────────┐
│                    Capture Adapters                    │
│  (Phase 1+: Shell hooks, VS Code, Neovim, MCP)        │
└────────────────────────┬───────────────────────────────┘
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
            └────────────────────────┘
```

### What Gets Tracked

Each AI interaction captures:

- **AI Metadata**: Agent (claude-code, cursor, etc.), model version, prompt text, response
- **Git Context**: Repository, branch, commit, dirty files
- **Developer Context**: Author, IDE, workspace files
- **Metrics**: Tokens, latency, session grouping

See `ROADMAP.md` for the complete data schema.

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
│   ├── daemon/            # Unix socket server
│   ├── storage/           # SQLite database layer
│   └── git/               # Git integration
├── ROADMAP.md             # Development roadmap and design decisions
└── DESIGN.md              # Technical design document
```

## Roadmap

### Phase 0: Foundation ✅ (Current)
- ✅ SQLite storage with WAL mode
- ✅ Unix socket daemon
- ✅ Git integration (branch, dirty files, commits)
- ✅ CLI basics (daemon control, list/show/search)

### Phase 1: Shell Hook Integration
- Shell hooks for transparent AI tool capture
- Session-based file tracking
- Auto-linking prompts to commits

### Phase 2+
- VS Code extension
- Neovim plugin
- Enhanced analytics (blame, stats, cost tracking)
- Team aggregation (optional)

See `ROADMAP.md` for detailed phase breakdown.

## Design Principles

1. **Agent-agnostic**: Works with Claude Code, Cursor, Copilot, Aider, etc.
2. **Non-invasive**: Minimal developer friction
3. **Local-first**: Your data stays on your machine
4. **Privacy-respecting**: Built-in redaction, easy opt-out
5. **Test-driven**: TDD approach with comprehensive test coverage

## Contributing

See `CONTRIBUTING.md` (coming soon) for guidelines.

## License

MIT (to be added)

## Questions & Feedback

File issues at: https://github.com/albachteng/provenance/issues

---

*Phase 0 Status: Foundation complete - daemon, storage, git integration, and CLI basics working.*
