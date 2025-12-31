# Testing the Shell Hook

Quick guide to manually test the provenance tracking system.

## Prerequisites

Build the binary:

```bash
go build -o prov ./cmd/prov
```

## Test 1: Basic Wrapper

Test that the wrapper captures events:

```bash
# Start daemon
./prov daemon start

# Verify daemon is running
./prov daemon status

# Wrap a simple command
./prov wrap test-agent echo "This is a test prompt"

# Should output: This is a test prompt

# Query events
./prov list

# You should see:
# ID                   Timestamp            Agent         Prompt
# --------------------------------------------------------------------------------
# evt-xxxxx-xxxxx      2025-12-30 HH:MM:SS  test-agent    echo This is a test prompt

# View details
./prov show <event-id>

# Clean up
./prov daemon stop
```

## Test 2: Shell Hook Integration

Test the shell hook with aliases:

```bash
# Start daemon
./prov daemon start

# Generate and load hook (in current shell)
eval "$(./prov hook bash)"

# Create an alias for testing
alias test-ai='prov wrap test-agent'

# Use the alias
test-ai echo "Testing shell hook integration"

# Should output: Testing shell hook integration

# Verify it was captured
./prov list

# You should see the event

# Clean up
./prov daemon stop
```

## Test 3: Git Context Capture

Test that git information is captured:

```bash
# Initialize a git repo (if not already in one)
git init
git config user.email "test@example.com"
git config user.name "Test User"

# Create and commit a file
echo "test" > test.txt
git add test.txt
git commit -m "Initial commit"

# Make some uncommitted changes
echo "modified" >> test.txt

# Start daemon
./prov daemon start

# Capture an event
./prov wrap test-agent echo "Modified test.txt"

# View the event details
./prov show <event-id>

# You should see:
# - Git Commit: <hash>
# - Git Branch: main (or master)
# - Git Dirty: true
# - Repo: /path/to/current/repo

# Clean up
./prov daemon stop
```

## Test 4: Multiple Events and Sessions

Test session linking:

```bash
# Start daemon
./prov daemon start

# Capture multiple events
./prov wrap agent1 echo "First prompt"
./prov wrap agent2 echo "Second prompt"
./prov wrap agent1 echo "Third prompt"

# List all events
./prov list

# Search for specific agent
./prov search "agent1"

# You should see 2 events

# Stop daemon
./prov daemon stop
```

## Test 5: Exit Code Preservation

Verify that exit codes are preserved:

```bash
# Start daemon
./prov daemon start

# Wrap a failing command
./prov wrap test-agent sh -c "exit 42"
echo "Exit code: $?"

# Should output: Exit code: 42

# Wrap a succeeding command
./prov wrap test-agent echo "success"
echo "Exit code: $?"

# Should output: Exit code: 0

# Stop daemon
./prov daemon stop
```

## Troubleshooting

### Events not appearing?

1. Check daemon is running:
   ```bash
   ./prov daemon status
   ```

2. Check for errors in daemon (if detached, check logs when we add them)

3. Verify database was created:
   ```bash
   ls -la ~/.ai-provenance/
   # Should see: daemon.sock, db.sqlite, daemon.pid
   ```

4. Query database directly:
   ```bash
   sqlite3 ~/.ai-provenance/db.sqlite "SELECT COUNT(*) FROM prompt_events;"
   ```

### Daemon won't start?

1. Check if already running:
   ```bash
   ./prov daemon status
   ```

2. Check socket file:
   ```bash
   ls -la ~/.ai-provenance/daemon.sock
   ```

3. If stale, remove and retry:
   ```bash
   rm ~/.ai-provenance/daemon.sock
   rm ~/.ai-provenance/daemon.pid
   ./prov daemon start
   ```

## Testing with Real AI Tools

Once basic tests work, try with actual AI tools:

```bash
# Start daemon
./prov daemon start

# If you have aider installed
alias aider='prov wrap aider aider'
aider --message "Add a function to calculate fibonacci"

# Query the captured interaction
./prov list
./prov show <event-id>

# Stop daemon
./prov daemon stop
```

## Running Automated Tests

Run the full test suite:

```bash
# All tests
go test ./...

# Just shell hook tests
go test -v -run "Hook|Wrapper" ./cmd/prov

# With race detector
go test -race ./...
```
