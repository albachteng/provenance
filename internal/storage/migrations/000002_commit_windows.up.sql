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

-- Drop old tables (no longer needed in commit window architecture)
DROP TABLE IF EXISTS change_sets;
DROP TABLE IF EXISTS sessions;
