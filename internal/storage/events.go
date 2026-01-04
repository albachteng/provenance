package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a requested record does not exist
var ErrNotFound = errors.New("record not found")

// PromptEvent represents a single AI interaction captured for provenance
type PromptEvent struct {
	ID        string
	Timestamp time.Time
	SessionID string

	// AI metadata
	Agent        string
	ModelVersion string
	PromptText   string
	ResponseText string

	// Usage metrics
	TokensIn  int
	TokensOut int
	LatencyMs int

	// Git context
	RepoPath   string
	GitCommit  string
	GitBranch  string
	GitDirty   bool
	DirtyFiles []string // JSON array in DB

	// Developer context
	Author         string
	IDE            string
	ActiveFile     string
	WorkspaceFiles []string // JSON array in DB

	// Categorization
	PromptType     string
	ToolsInvoked   []string // JSON array in DB
	FilesMentioned []string // JSON array in DB
}

// Session represents a group of related prompts
type Session struct {
	ID           string
	StartTime    time.Time
	EndTime      *time.Time // nil if active
	RepoPath     string
	TotalPrompts int
	TotalTokens  int
	EndedBy      string // 'commit' | 'timeout' | 'manual'
}

// RepoStats contains aggregate statistics for a repository
type RepoStats struct {
	TotalPrompts   int
	TotalTokensIn  int
	TotalTokensOut int
	SessionCount   int
	FilesMentioned map[string]int // file path -> mention count
	ToolsInvoked   map[string]int // tool name -> invocation count
}

// SessionStats contains statistics for a specific session
type SessionStats struct {
	SessionID      string
	TotalPrompts   int
	TotalTokensIn  int
	TotalTokensOut int
	StartTime      time.Time
	EndTime        *time.Time
	FilesMentioned map[string]int // file path -> mention count
	ToolsInvoked   map[string]int // tool name -> invocation count
}

// StorePromptEvent stores a prompt event in the database
func StorePromptEvent(db *sql.DB, event *PromptEvent) error {
	dirtyFilesJSON, err := json.Marshal(event.DirtyFiles)
	if err != nil {
		return fmt.Errorf("failed to marshal dirty_files: %w", err)
	}

	workspaceFilesJSON, err := json.Marshal(event.WorkspaceFiles)
	if err != nil {
		return fmt.Errorf("failed to marshal workspace_files: %w", err)
	}

	toolsInvokedJSON, err := json.Marshal(event.ToolsInvoked)
	if err != nil {
		return fmt.Errorf("failed to marshal tools_invoked: %w", err)
	}

	filesMentionedJSON, err := json.Marshal(event.FilesMentioned)
	if err != nil {
		return fmt.Errorf("failed to marshal files_mentioned: %w", err)
	}

	query := `
		INSERT INTO prompt_events (
			id, timestamp, session_id,
			agent, model_version, prompt_text, response_text,
			tokens_in, tokens_out, latency_ms,
			repo_path, git_commit, git_branch, git_dirty, dirty_files,
			author, ide, active_file, workspace_files,
			prompt_type, tools_invoked, files_mentioned
		) VALUES (
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?
		)
	`

	_, err = db.Exec(
		query,
		event.ID, event.Timestamp.Unix(), event.SessionID,
		event.Agent, event.ModelVersion, event.PromptText, event.ResponseText,
		event.TokensIn, event.TokensOut, event.LatencyMs,
		event.RepoPath, event.GitCommit, event.GitBranch, event.GitDirty, string(dirtyFilesJSON),
		event.Author, event.IDE, event.ActiveFile, string(workspaceFilesJSON),
		event.PromptType, string(toolsInvokedJSON), string(filesMentionedJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to insert prompt event: %w", err)
	}

	return nil
}

// GetPromptEvent retrieves a prompt event by ID
func GetPromptEvent(db *sql.DB, id string) (*PromptEvent, error) {
	query := `
		SELECT
			id, timestamp, session_id,
			agent, model_version, prompt_text, response_text,
			tokens_in, tokens_out, latency_ms,
			repo_path, git_commit, git_branch, git_dirty, dirty_files,
			author, ide, active_file, workspace_files,
			prompt_type, tools_invoked, files_mentioned
		FROM prompt_events
		WHERE id = ?
	`

	var event PromptEvent
	var timestampUnix int64
	var dirtyFilesJSON, workspaceFilesJSON, toolsInvokedJSON, filesMentionedJSON string

	err := db.QueryRow(query, id).Scan(
		&event.ID, &timestampUnix, &event.SessionID,
		&event.Agent, &event.ModelVersion, &event.PromptText, &event.ResponseText,
		&event.TokensIn, &event.TokensOut, &event.LatencyMs,
		&event.RepoPath, &event.GitCommit, &event.GitBranch, &event.GitDirty, &dirtyFilesJSON,
		&event.Author, &event.IDE, &event.ActiveFile, &workspaceFilesJSON,
		&event.PromptType, &toolsInvokedJSON, &filesMentionedJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query prompt event: %w", err)
	}

	event.Timestamp = time.Unix(timestampUnix, 0)

	if err := json.Unmarshal([]byte(dirtyFilesJSON), &event.DirtyFiles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal dirty_files: %w", err)
	}

	if err := json.Unmarshal([]byte(workspaceFilesJSON), &event.WorkspaceFiles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workspace_files: %w", err)
	}

	if err := json.Unmarshal([]byte(toolsInvokedJSON), &event.ToolsInvoked); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tools_invoked: %w", err)
	}

	if err := json.Unmarshal([]byte(filesMentionedJSON), &event.FilesMentioned); err != nil {
		return nil, fmt.Errorf("failed to unmarshal files_mentioned: %w", err)
	}

	return &event, nil
}

// ListPromptEvents retrieves all prompt events for a session in chronological order
func ListPromptEvents(db *sql.DB, sessionID string) ([]*PromptEvent, error) {
	query := `
		SELECT
			id, timestamp, session_id,
			agent, model_version, prompt_text, response_text,
			tokens_in, tokens_out, latency_ms,
			repo_path, git_commit, git_branch, git_dirty, dirty_files,
			author, ide, active_file, workspace_files,
			prompt_type, tools_invoked, files_mentioned
		FROM prompt_events
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`

	rows, err := db.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query prompt events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var events []*PromptEvent
	for rows.Next() {
		var event PromptEvent
		var timestampUnix int64
		var dirtyFilesJSON, workspaceFilesJSON, toolsInvokedJSON, filesMentionedJSON string

		err := rows.Scan(
			&event.ID, &timestampUnix, &event.SessionID,
			&event.Agent, &event.ModelVersion, &event.PromptText, &event.ResponseText,
			&event.TokensIn, &event.TokensOut, &event.LatencyMs,
			&event.RepoPath, &event.GitCommit, &event.GitBranch, &event.GitDirty, &dirtyFilesJSON,
			&event.Author, &event.IDE, &event.ActiveFile, &workspaceFilesJSON,
			&event.PromptType, &toolsInvokedJSON, &filesMentionedJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan prompt event: %w", err)
		}

		event.Timestamp = time.Unix(timestampUnix, 0)

		if err := json.Unmarshal([]byte(dirtyFilesJSON), &event.DirtyFiles); err != nil {
			return nil, fmt.Errorf("failed to unmarshal dirty_files: %w", err)
		}

		if err := json.Unmarshal([]byte(workspaceFilesJSON), &event.WorkspaceFiles); err != nil {
			return nil, fmt.Errorf("failed to unmarshal workspace_files: %w", err)
		}

		if err := json.Unmarshal([]byte(toolsInvokedJSON), &event.ToolsInvoked); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tools_invoked: %w", err)
		}

		if err := json.Unmarshal([]byte(filesMentionedJSON), &event.FilesMentioned); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files_mentioned: %w", err)
		}

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating prompt events: %w", err)
	}

	return events, nil
}

// CreateSession creates a new session in the database
func CreateSession(db *sql.DB, session *Session) error {
	var endTimeUnix *int64
	if session.EndTime != nil {
		unix := session.EndTime.Unix()
		endTimeUnix = &unix
	}

	query := `
		INSERT INTO sessions (
			id, start_time, end_time, repo_path,
			total_prompts, total_tokens, ended_by
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(
		query,
		session.ID, session.StartTime.Unix(), endTimeUnix, session.RepoPath,
		session.TotalPrompts, session.TotalTokens, session.EndedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	return nil
}

// GetActiveSession retrieves the active session for a repository
func GetActiveSession(db *sql.DB, repoPath string) (*Session, error) {
	query := `
		SELECT
			id, start_time, end_time, repo_path,
			total_prompts, total_tokens, ended_by
		FROM sessions
		WHERE repo_path = ? AND end_time IS NULL
		ORDER BY start_time DESC
		LIMIT 1
	`

	var session Session
	var startTimeUnix int64
	var endTimeUnix *int64

	err := db.QueryRow(query, repoPath).Scan(
		&session.ID, &startTimeUnix, &endTimeUnix, &session.RepoPath,
		&session.TotalPrompts, &session.TotalTokens, &session.EndedBy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query active session: %w", err)
	}

	session.StartTime = time.Unix(startTimeUnix, 0)
	if endTimeUnix != nil {
		endTime := time.Unix(*endTimeUnix, 0)
		session.EndTime = &endTime
	}

	return &session, nil
}

// EndSession ends a session with the given end time and reason
func EndSession(db *sql.DB, sessionID string, endTime time.Time, reason string) error {
	query := `
		UPDATE sessions
		SET end_time = ?, ended_by = ?
		WHERE id = ?
	`

	_, err := db.Exec(query, endTime.Unix(), reason, sessionID)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID
func GetSession(db *sql.DB, sessionID string) (*Session, error) {
	query := `
		SELECT
			id, start_time, end_time, repo_path,
			total_prompts, total_tokens, ended_by
		FROM sessions
		WHERE id = ?
	`

	var session Session
	var startTimeUnix int64
	var endTimeUnix *int64

	err := db.QueryRow(query, sessionID).Scan(
		&session.ID, &startTimeUnix, &endTimeUnix, &session.RepoPath,
		&session.TotalPrompts, &session.TotalTokens, &session.EndedBy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	session.StartTime = time.Unix(startTimeUnix, 0)
	if endTimeUnix != nil {
		endTime := time.Unix(*endTimeUnix, 0)
		session.EndTime = &endTime
	}

	return &session, nil
}

// UpdateSessionMetrics increments the session's total prompts and tokens
func UpdateSessionMetrics(db *sql.DB, sessionID string, promptsDelta, tokensDelta int) error {
	query := `
		UPDATE sessions
		SET total_prompts = total_prompts + ?,
		    total_tokens = total_tokens + ?
		WHERE id = ?
	`

	_, err := db.Exec(query, promptsDelta, tokensDelta, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session metrics: %w", err)
	}

	return nil
}

// ListSessions returns all sessions, optionally filtered to only active ones
func ListSessions(db *sql.DB, activeOnly bool) ([]*Session, error) {
	query := `
		SELECT id, start_time, end_time, repo_path, total_prompts, total_tokens, ended_by
		FROM sessions
	`

	if activeOnly {
		query += " WHERE end_time IS NULL"
	}

	query += " ORDER BY start_time DESC"

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var sessions []*Session
	for rows.Next() {
		var s Session
		var startTimeUnix int64
		var endTimeUnix sql.NullInt64
		var endedBy sql.NullString

		err := rows.Scan(&s.ID, &startTimeUnix, &endTimeUnix, &s.RepoPath, &s.TotalPrompts, &s.TotalTokens, &endedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		s.StartTime = time.Unix(startTimeUnix, 0)

		if endTimeUnix.Valid {
			endTime := time.Unix(endTimeUnix.Int64, 0)
			s.EndTime = &endTime
		}
		if endedBy.Valid {
			s.EndedBy = endedBy.String
		}

		sessions = append(sessions, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// GetRepoStats returns aggregate statistics for a repository
func GetRepoStats(db *sql.DB, repoPath string) (*RepoStats, error) {
	stats := &RepoStats{
		FilesMentioned: make(map[string]int),
		ToolsInvoked:   make(map[string]int),
	}

	// Get aggregate counts and session count
	query := `
		SELECT
			COUNT(DISTINCT pe.id) as total_prompts,
			COALESCE(SUM(pe.tokens_in), 0) as total_tokens_in,
			COALESCE(SUM(pe.tokens_out), 0) as total_tokens_out,
			COUNT(DISTINCT pe.session_id) as session_count
		FROM prompt_events pe
		WHERE pe.repo_path = ?
	`

	err := db.QueryRow(query, repoPath).Scan(
		&stats.TotalPrompts,
		&stats.TotalTokensIn,
		&stats.TotalTokensOut,
		&stats.SessionCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregate stats: %w", err)
	}

	// Get all events to aggregate files and tools
	eventsQuery := `
		SELECT files_mentioned, tools_invoked
		FROM prompt_events
		WHERE repo_path = ?
	`

	rows, err := db.Query(eventsQuery, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var filesMentionedJSON, toolsInvokedJSON string

		if err := rows.Scan(&filesMentionedJSON, &toolsInvokedJSON); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		// Parse and aggregate files mentioned
		var filesMentioned []string
		if err := json.Unmarshal([]byte(filesMentionedJSON), &filesMentioned); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files_mentioned: %w", err)
		}
		for _, file := range filesMentioned {
			stats.FilesMentioned[file]++
		}

		// Parse and aggregate tools invoked
		var toolsInvoked []string
		if err := json.Unmarshal([]byte(toolsInvokedJSON), &toolsInvoked); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tools_invoked: %w", err)
		}
		for _, tool := range toolsInvoked {
			stats.ToolsInvoked[tool]++
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return stats, nil
}

// GetSessionStats returns statistics for a specific session
func GetSessionStats(db *sql.DB, sessionID string) (*SessionStats, error) {
	// First get session metadata
	session, err := GetSession(db, sessionID)
	if err != nil {
		return nil, err
	}

	stats := &SessionStats{
		SessionID:      sessionID,
		StartTime:      session.StartTime,
		EndTime:        session.EndTime,
		FilesMentioned: make(map[string]int),
		ToolsInvoked:   make(map[string]int),
	}

	// Get aggregate counts for this session
	query := `
		SELECT
			COUNT(id) as total_prompts,
			COALESCE(SUM(tokens_in), 0) as total_tokens_in,
			COALESCE(SUM(tokens_out), 0) as total_tokens_out
		FROM prompt_events
		WHERE session_id = ?
	`

	err = db.QueryRow(query, sessionID).Scan(
		&stats.TotalPrompts,
		&stats.TotalTokensIn,
		&stats.TotalTokensOut,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregate stats: %w", err)
	}

	// Get all events to aggregate files and tools
	eventsQuery := `
		SELECT files_mentioned, tools_invoked
		FROM prompt_events
		WHERE session_id = ?
	`

	rows, err := db.Query(eventsQuery, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var filesMentionedJSON, toolsInvokedJSON string

		if err := rows.Scan(&filesMentionedJSON, &toolsInvokedJSON); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		// Parse and aggregate files mentioned
		var filesMentioned []string
		if err := json.Unmarshal([]byte(filesMentionedJSON), &filesMentioned); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files_mentioned: %w", err)
		}
		for _, file := range filesMentioned {
			stats.FilesMentioned[file]++
		}

		// Parse and aggregate tools invoked
		var toolsInvoked []string
		if err := json.Unmarshal([]byte(toolsInvokedJSON), &toolsInvoked); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tools_invoked: %w", err)
		}
		for _, tool := range toolsInvoked {
			stats.ToolsInvoked[tool]++
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return stats, nil
}

// GetTimeframeStats returns statistics for a repository within a time window
func GetTimeframeStats(db *sql.DB, repoPath string, since time.Time) (*RepoStats, error) {
	stats := &RepoStats{
		FilesMentioned: make(map[string]int),
		ToolsInvoked:   make(map[string]int),
	}

	sinceUnix := since.Unix()

	// Get aggregate counts and session count
	query := `
		SELECT
			COUNT(DISTINCT pe.id) as total_prompts,
			COALESCE(SUM(pe.tokens_in), 0) as total_tokens_in,
			COALESCE(SUM(pe.tokens_out), 0) as total_tokens_out,
			COUNT(DISTINCT pe.session_id) as session_count
		FROM prompt_events pe
		WHERE pe.repo_path = ? AND pe.timestamp >= ?
	`

	err := db.QueryRow(query, repoPath, sinceUnix).Scan(
		&stats.TotalPrompts,
		&stats.TotalTokensIn,
		&stats.TotalTokensOut,
		&stats.SessionCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregate stats: %w", err)
	}

	// Get all events to aggregate files and tools
	eventsQuery := `
		SELECT files_mentioned, tools_invoked
		FROM prompt_events
		WHERE repo_path = ? AND timestamp >= ?
	`

	rows, err := db.Query(eventsQuery, repoPath, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var filesMentionedJSON, toolsInvokedJSON string

		if err := rows.Scan(&filesMentionedJSON, &toolsInvokedJSON); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		// Parse and aggregate files mentioned
		var filesMentioned []string
		if err := json.Unmarshal([]byte(filesMentionedJSON), &filesMentioned); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files_mentioned: %w", err)
		}
		for _, file := range filesMentioned {
			stats.FilesMentioned[file]++
		}

		// Parse and aggregate tools invoked
		var toolsInvoked []string
		if err := json.Unmarshal([]byte(toolsInvokedJSON), &toolsInvoked); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tools_invoked: %w", err)
		}
		for _, tool := range toolsInvoked {
			stats.ToolsInvoked[tool]++
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return stats, nil
}
