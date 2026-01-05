-- Rollback initial schema

DROP INDEX IF EXISTS idx_change_sets_commit;
DROP INDEX IF EXISTS idx_change_sets_session;
DROP INDEX IF EXISTS idx_change_sets_prompt;
DROP INDEX IF EXISTS idx_sessions_start_time;
DROP INDEX IF EXISTS idx_sessions_repo;
DROP INDEX IF EXISTS idx_prompt_events_author;
DROP INDEX IF EXISTS idx_prompt_events_repo;
DROP INDEX IF EXISTS idx_prompt_events_timestamp;
DROP INDEX IF EXISTS idx_prompt_events_session;

DROP TABLE IF EXISTS redaction_rules;
DROP TABLE IF EXISTS change_sets;
DROP TABLE IF EXISTS prompt_events;
DROP TABLE IF EXISTS sessions;
