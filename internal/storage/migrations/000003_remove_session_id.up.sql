-- Migration 000003: Remove session_id Column
-- V2 architecture doesn't use sessions, so remove the legacy column entirely

PRAGMA foreign_keys=OFF;

CREATE TABLE prompt_events_new (
    id TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,

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

    -- V2 fields
    branch_at_capture TEXT,
    pre_branch_switch BOOLEAN DEFAULT FALSE
);

-- Copy all data except session_id column
INSERT INTO prompt_events_new (
    id, timestamp, agent, model_version, prompt_text, response_text,
    tokens_in, tokens_out, latency_ms, repo_path, git_commit, git_branch,
    git_dirty, dirty_files, author, ide, active_file, workspace_files,
    prompt_type, tools_invoked, files_mentioned, branch_at_capture, pre_branch_switch
)
SELECT
    id, timestamp, agent, model_version, prompt_text, response_text,
    tokens_in, tokens_out, latency_ms, repo_path, git_commit, git_branch,
    git_dirty, dirty_files, author, ide, active_file, workspace_files,
    prompt_type, tools_invoked, files_mentioned, branch_at_capture, pre_branch_switch
FROM prompt_events;

DROP TABLE prompt_events;
ALTER TABLE prompt_events_new RENAME TO prompt_events;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_prompt_events_timestamp ON prompt_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_prompt_events_repo ON prompt_events(repo_path);
CREATE INDEX IF NOT EXISTS idx_prompt_events_author ON prompt_events(author);
CREATE INDEX IF NOT EXISTS idx_prompt_events_branch ON prompt_events(git_branch);
CREATE INDEX IF NOT EXISTS idx_prompt_events_repo_branch_time
  ON prompt_events(repo_path, git_branch, timestamp);

PRAGMA foreign_keys=ON;
