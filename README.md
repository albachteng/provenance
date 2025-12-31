# AI Provenance

Track AI-assisted code changes across your development workflow. An open-source, agent-agnostic provenance system that helps teams understand AI usage patterns, measure ROI, and maintain code quality.

> **Status**: Phase 1 complete - daemon, CLI, and shell hooks functional. You can now capture and query AI interactions! See `ROADMAP.md` for development plans.

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

**Option 1: Shell Hook (Recommended)**

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

**Option 2: Direct Wrapper**

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

### Environment Configuration

Override the default storage location:

```bash
export AI_PROVENANCE_HOME=/custom/path
./prov daemon start
```

## Try It Out

Here's a complete example:

```bash
# Build and start daemon
go build -o prov ./cmd/prov
./prov daemon start

# Wrap a command to track it
./prov wrap claude-code echo "Implement user authentication"

# View captured events
./prov list

# Output:
# ID                   Timestamp            Agent           Prompt
# ----------------------------------------------------------------------------------------------------
# evt-1735567890-12345 2025-12-30 10:30:00 claude-code     echo Implement user authentication
#
# Showing 1 event(s)

# See detailed event info
./prov show evt-1735567890-12345

# Search for specific prompts
./prov search "authentication"
```

The captured event includes:
- Prompt text and agent used
- Git context (branch, commit, dirty state)
- Timestamp and author
- Repository path and session ID

## How It Works

A background daemon stores AI interaction events in a local SQLite database:

```
Capture Adapters → Unix Socket → Storage Daemon → SQLite Database
(coming soon)         ↓              ↓               ↓
                   Commands      Event Store    ~/.ai-provenance/
```

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
│   ├── daemon/            # Unix socket server
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

## Contributing

See `CONTRIBUTING.md` (coming soon) for guidelines.

## License

MIT (to be added)

## Questions & Feedback

File issues at: https://github.com/albachteng/provenance/issues
