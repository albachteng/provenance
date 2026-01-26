# Manual Testing Guide

Quick guide to manually test the AI Provenance tracking system (v2 architecture).

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

## Test 4: Multiple Events

Test capturing multiple prompts:

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

## Test 5: Commit Window Blame (V2 Core Feature)

Test the v2 commit window-based blame functionality:

```bash
# Start daemon
./prov daemon start

# Initialize a test git repository
mkdir -p /tmp/prov-test
cd /tmp/prov-test
git init
git config user.email "test@example.com"
git config user.name "Test User"

# Create initial commit
echo "# My Project" > README.md
git add README.md
git commit -m "Initial commit"

# Wait a moment, then capture some prompts
sleep 2
./prov wrap claude-code echo "Implement user authentication"
sleep 1
./prov wrap claude-code echo "Add password hashing"
sleep 1

# Make code changes and commit
echo "function authenticate() {}" > auth.js
git add auth.js
git commit -m "Add authentication"

# Now blame the commit to see which prompts led to it
./prov blame HEAD

# Expected output:
# Commit: <sha> (YYYY-MM-DD HH:MM:SS)
# Branch: main
#
# Found 2 prompt(s) in commit window:
#
# [1] Prompt ID: evt-xxxxx
#     Timestamp: YYYY-MM-DD HH:MM:SS
#     Agent: claude-code
#     Prompt: Implement user authentication
#
# [2] Prompt ID: evt-xxxxx
#     Timestamp: YYYY-MM-DD HH:MM:SS
#     Agent: claude-code
#     Prompt: Add password hashing
#
# Files changed in commit:
#   - auth.js

# Test with no prompts in window
git commit --allow-empty -m "Empty commit"
./prov blame HEAD
# Should output: No prompts found in commit window

# Test branch filtering
git checkout -b feature/test
sleep 2
./prov wrap claude-code echo "Work on feature branch"
sleep 1
echo "feature code" > feature.js
git add feature.js
git commit -m "Add feature"

# This should show the feature branch prompt
./prov blame HEAD

# Switch back to main - should not show feature branch prompts
git checkout main
./prov blame HEAD
# Should show main branch prompts only

# Clean up
cd -
./prov daemon stop
rm -rf /tmp/prov-test
```

**What this tests:**
- Prompts are associated with commits based on timestamp windows
- Only prompts on the same branch are shown
- Branch filtering works correctly
- Empty commit windows are handled gracefully

## Test 6: Exit Code Preservation

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

## Testing with Claude Code (Recommended)

The most comprehensive way to test provenance is with Claude Code hooks:

```bash
# Build and install
go build -o prov ./cmd/prov
./prov daemon start

# Install Claude Code hooks (one-time setup)
./prov install-hooks claude-code

# Verify installation
./prov hooks status
# Should show:
# claude-code:
#   - claude-prompt.py
#   - claude-tool-pre.py
#   - claude-tool-post.py

# Now use Claude Code normally
# Every prompt and tool invocation is automatically captured!

# After working with Claude Code, check captured events
./prov list

# Search for specific prompts
./prov search "implement"

# View detailed event info
./prov show <event-id>

# After committing code, blame it
git add .
git commit -m "Feature implemented"
./prov blame HEAD

# Stop daemon
./prov daemon stop
```

## Testing with Other AI Tools

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
# All tests (should pass 100%)
go test ./...

# Specific test suites
go test -v -run "TestBlame" ./cmd/prov           # V2 blame tests
go test -v -run "TestHook" ./cmd/prov            # Hook integration tests
go test -v ./internal/queries                    # V2 query layer tests
go test -v ./internal/git                        # Git utilities tests
go test -v ./internal/storage                    # Storage layer tests

# With race detector
go test -race ./...

# With coverage
go test -cover ./...
```

**Test Statistics (V2):**
- Total tests: ~50+ (includes skipped v1 tests)
- Passing: 100%
- Skipped: ~16 (manual tagging and session-based stats - deferred features)
- Test execution time: ~8-10 seconds

**Key Test Suites:**
- **Blame tests** (6): Test commit window queries, branch filtering, pre_branch_switch handling
- **Query tests** (5): Test GetPromptsForCommit core functionality
- **Hook tests** (6): Test Claude Code integration
- **Storage tests** (26): Test database operations
- **Git tests**: Test commit metadata extraction and git operations
