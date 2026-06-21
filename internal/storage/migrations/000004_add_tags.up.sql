CREATE TABLE IF NOT EXISTS prompt_tags (
    id TEXT PRIMARY KEY,
    prompt_id TEXT NOT NULL REFERENCES prompt_events(id) ON DELETE CASCADE,
    commit_sha TEXT NOT NULL,
    note TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_prompt_tags_prompt_id ON prompt_tags(prompt_id);
CREATE INDEX IF NOT EXISTS idx_prompt_tags_commit_sha ON prompt_tags(commit_sha);
CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_tags_unique ON prompt_tags(prompt_id, commit_sha);
