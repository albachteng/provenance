-- Initial database schema for AI provenance tracking

-- Sessions table: groups related prompts
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    start_time INTEGER NOT NULL,
    end_time INTEGER,
    repo_path TEXT,
    total_prompts INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    ended_by TEXT  -- 'commit' | 'timeout' | 'manual'
);

-- Prompt events table: individual AI interactions
CREATE TABLE IF NOT EXISTS prompt_events (
    id TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    session_id TEXT NOT NULL,

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
    dirty_files TEXT,  -- JSON array

    -- Developer context
    author TEXT NOT NULL,
    ide TEXT,
    active_file TEXT,
    workspace_files TEXT,  -- JSON array

    -- Categorization
    prompt_type TEXT,
    tools_invoked TEXT,  -- JSON array
    files_mentioned TEXT,  -- JSON array

    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

-- Change sets table: derived prompt-to-code-change correlations
CREATE TABLE IF NOT EXISTS change_sets (
    id TEXT PRIMARY KEY,
    prompt_id TEXT,
    session_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,

    files_changed TEXT NOT NULL,  -- JSON array
    diff_summary TEXT,
    commit_introduced TEXT,

    -- Correlation metadata
    correlation_method TEXT,  -- 'session' | 'file_watch' | 'git_hook' | 'manual'
    confidence REAL,
    time_to_first_change_ms INTEGER,

    FOREIGN KEY (prompt_id) REFERENCES prompt_events(id),
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

-- Redaction rules table: privacy patterns
CREATE TABLE IF NOT EXISTS redaction_rules (
    id TEXT PRIMARY KEY,
    pattern TEXT NOT NULL,
    replacement TEXT DEFAULT '[REDACTED]',
    scope TEXT DEFAULT 'both',  -- 'prompt' | 'response' | 'both'
    enabled BOOLEAN DEFAULT TRUE
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_prompt_events_session ON prompt_events(session_id);
CREATE INDEX IF NOT EXISTS idx_prompt_events_timestamp ON prompt_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_prompt_events_repo ON prompt_events(repo_path);
CREATE INDEX IF NOT EXISTS idx_prompt_events_author ON prompt_events(author);
CREATE INDEX IF NOT EXISTS idx_sessions_repo ON sessions(repo_path);
CREATE INDEX IF NOT EXISTS idx_sessions_start_time ON sessions(start_time);
CREATE INDEX IF NOT EXISTS idx_change_sets_prompt ON change_sets(prompt_id);
CREATE INDEX IF NOT EXISTS idx_change_sets_session ON change_sets(session_id);
CREATE INDEX IF NOT EXISTS idx_change_sets_commit ON change_sets(commit_introduced);
