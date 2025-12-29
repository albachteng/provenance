# AI Provenance

Track AI-assisted code changes across your development workflow. An open-source, agent-agnostic provenance system that helps teams understand AI usage patterns, measure ROI, and maintain code quality.

> **Status**: Early development - core storage and daemon functional, capture adapters coming soon. See `ROADMAP.md` for development plans.

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

- **`ROADMAP.md`**: Development roadmap, phase breakdown, and design decisions
- **`DESIGN.md`**: Technical design document and architecture details

## Contributing

See `CONTRIBUTING.md` (coming soon) for guidelines.

## License

MIT (to be added)

## Questions & Feedback

File issues at: https://github.com/albachteng/provenance/issues
