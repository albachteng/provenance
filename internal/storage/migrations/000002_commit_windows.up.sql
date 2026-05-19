-- Migration 000002: Commit Window-Based Architecture
-- Removes session management and confidence scoring
-- Adds branch tracking and optional commit window caching

-- Enhance prompt_events with branch tracking
ALTER TABLE prompt_events
  ADD COLUMN branch_at_capture TEXT;

ALTER TABLE prompt_events
  ADD COLUMN pre_branch_switch BOOLEAN DEFAULT FALSE;

-- Tool invocations table (lightweight, file paths only)
CREATE TABLE IF NOT EXISTS tool_invocations (
    id TEXT PRIMARY KEY,
    prompt_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    tool_args TEXT,  -- Lightweight JSON (file paths, simple params)
    timestamp INTEGER NOT NULL,
    FOREIGN KEY (prompt_id) REFERENCES prompt_events(id) ON DELETE CASCADE
);

-- Commit windows cache (optional, for performance)
CREATE TABLE IF NOT EXISTS commit_windows (
    id TEXT PRIMARY KEY,
    repo_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    prev_commit TEXT,
    next_commit TEXT NOT NULL,
    prev_commit_time INTEGER,
    next_commit_time INTEGER NOT NULL,
    prompt_count INTEGER DEFAULT 0,
    UNIQUE(repo_path, branch, next_commit)
);

-- New indexes for window queries
CREATE INDEX IF NOT EXISTS idx_prompt_events_branch ON prompt_events(git_branch);
CREATE INDEX IF NOT EXISTS idx_prompt_events_repo_branch_time
  ON prompt_events(repo_path, git_branch, timestamp);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_prompt ON tool_invocations(prompt_id);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_timestamp ON tool_invocations(timestamp);
CREATE INDEX IF NOT EXISTS idx_commit_windows_repo_branch ON commit_windows(repo_path, branch);
CREATE INDEX IF NOT EXISTS idx_commit_windows_next_commit ON commit_windows(next_commit);

-- Copy existing git_branch to branch_at_capture for all records
UPDATE prompt_events SET branch_at_capture = git_branch WHERE branch_at_capture IS NULL;

-- Remove foreign key constraint to sessions table before dropping it
-- SQLite requires recreating the table to remove FK constraints
PRAGMA foreign_keys=OFF;

CREATE TABLE prompt_events_new (
    id TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    session_id TEXT,  -- Now nullable, no FK constraint

    -- AI metadata
    agent TEXT NOT NULL,
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
    dirty_files TEXT,

    -- Developer context
    author TEXT NOT NULL,
    ide TEXT,
    active_file TEXT,
    workspace_files TEXT,

    -- Categorization
    prompt_type TEXT,
    tools_invoked TEXT,
    files_mentioned TEXT,

    -- New v2 fields
    branch_at_capture TEXT,
    pre_branch_switch BOOLEAN DEFAULT FALSE
);

INSERT INTO prompt_events_new SELECT * FROM prompt_events;
DROP TABLE prompt_events;
ALTER TABLE prompt_events_new RENAME TO prompt_events;

-- Recreate indexes for prompt_events
CREATE INDEX IF NOT EXISTS idx_prompt_events_timestamp ON prompt_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_prompt_events_repo ON prompt_events(repo_path);
CREATE INDEX IF NOT EXISTS idx_prompt_events_author ON prompt_events(author);
CREATE INDEX IF NOT EXISTS idx_prompt_events_branch ON prompt_events(git_branch);
CREATE INDEX IF NOT EXISTS idx_prompt_events_repo_branch_time
  ON prompt_events(repo_path, git_branch, timestamp);

PRAGMA foreign_keys=ON;

-- Drop old tables (no longer needed in commit window architecture)
DROP TABLE IF EXISTS change_sets;
DROP TABLE IF EXISTS sessions;
