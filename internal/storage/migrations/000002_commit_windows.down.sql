-- Rollback Migration 000002: Commit Window-Based Architecture
-- WARNING: This will lose commit window data and tool invocations
-- Recreates sessions and change_sets tables

-- Drop new tables
DROP INDEX IF EXISTS idx_commit_windows_next_commit;
DROP INDEX IF EXISTS idx_commit_windows_repo_branch;
DROP INDEX IF EXISTS idx_tool_invocations_timestamp;
DROP INDEX IF EXISTS idx_tool_invocations_prompt;
DROP INDEX IF EXISTS idx_prompt_events_repo_branch_time;
DROP INDEX IF EXISTS idx_prompt_events_branch;

DROP TABLE IF EXISTS commit_windows;
DROP TABLE IF EXISTS tool_invocations;

-- Remove new columns from prompt_events
-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
-- This is a simplified approach - actual rollback should preserve existing data

-- Create temporary table without new columns
CREATE TABLE prompt_events_backup (
    id TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    session_id TEXT NOT NULL,
    agent TEXT NOT NULL,
    model_version TEXT,
    prompt_text TEXT NOT NULL,
    response_text TEXT,
    tokens_in INTEGER,
    tokens_out INTEGER,
    latency_ms INTEGER,
    repo_path TEXT NOT NULL,
    git_commit TEXT,
    git_branch TEXT,
    git_dirty BOOLEAN,
    dirty_files TEXT,
    author TEXT NOT NULL,
    ide TEXT,
    active_file TEXT,
    workspace_files TEXT,
    prompt_type TEXT,
    tools_invoked TEXT,
    files_mentioned TEXT
);

-- Copy data (excluding new columns)
INSERT INTO prompt_events_backup SELECT
    id, timestamp, session_id, agent, model_version, prompt_text, response_text,
    tokens_in, tokens_out, latency_ms, repo_path, git_commit, git_branch,
    git_dirty, dirty_files, author, ide, active_file, workspace_files,
    prompt_type, tools_invoked, files_mentioned
FROM prompt_events;

-- Drop original table
DROP TABLE prompt_events;

-- Rename backup to original
ALTER TABLE prompt_events_backup RENAME TO prompt_events;

-- Recreate original indexes
CREATE INDEX idx_prompt_events_session ON prompt_events(session_id);
CREATE INDEX idx_prompt_events_timestamp ON prompt_events(timestamp);
CREATE INDEX idx_prompt_events_repo ON prompt_events(repo_path);
CREATE INDEX idx_prompt_events_author ON prompt_events(author);

-- Recreate sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    start_time INTEGER NOT NULL,
    end_time INTEGER,
    repo_path TEXT,
    total_prompts INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    ended_by TEXT
);

CREATE INDEX idx_sessions_repo ON sessions(repo_path);
CREATE INDEX idx_sessions_start_time ON sessions(start_time);

-- Recreate change_sets table
CREATE TABLE IF NOT EXISTS change_sets (
    id TEXT PRIMARY KEY,
    prompt_id TEXT,
    session_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    files_changed TEXT NOT NULL,
    diff_summary TEXT,
    commit_introduced TEXT,
    correlation_method TEXT,
    confidence REAL,
    time_to_first_change_ms INTEGER,
    FOREIGN KEY (prompt_id) REFERENCES prompt_events(id),
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX idx_change_sets_prompt ON change_sets(prompt_id);
CREATE INDEX idx_change_sets_session ON change_sets(session_id);
CREATE INDEX idx_change_sets_commit ON change_sets(commit_introduced);
