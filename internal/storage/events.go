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

	// V2: Commit window tracking
	BranchAtCapture string // Branch at prompt submission (immutable)
	PreBranchSwitch bool   // Flag for prompts before branch switch
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
			id, timestamp,
			agent, model_version, prompt_text, response_text,
			tokens_in, tokens_out, latency_ms,
			repo_path, git_commit, git_branch, git_dirty, dirty_files,
			author, ide, active_file, workspace_files,
			prompt_type, tools_invoked, files_mentioned,
			branch_at_capture, pre_branch_switch
		) VALUES (
			?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?
		)
	`

	_, err = db.Exec(
		query,
		event.ID, event.Timestamp.Unix(),
		event.Agent, event.ModelVersion, event.PromptText, event.ResponseText,
		event.TokensIn, event.TokensOut, event.LatencyMs,
		event.RepoPath, event.GitCommit, event.GitBranch, event.GitDirty, string(dirtyFilesJSON),
		event.Author, event.IDE, event.ActiveFile, string(workspaceFilesJSON),
		event.PromptType, string(toolsInvokedJSON), string(filesMentionedJSON),
		event.BranchAtCapture, event.PreBranchSwitch,
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
			id, timestamp,
			agent, model_version, prompt_text, response_text,
			tokens_in, tokens_out, latency_ms,
			repo_path, git_commit, git_branch, git_dirty, dirty_files,
			author, ide, active_file, workspace_files,
			prompt_type, tools_invoked, files_mentioned,
			branch_at_capture, pre_branch_switch
		FROM prompt_events
		WHERE id = ?
	`

	var event PromptEvent
	var timestampUnix int64
	var dirtyFilesJSON, workspaceFilesJSON, toolsInvokedJSON, filesMentionedJSON string

	err := db.QueryRow(query, id).Scan(
		&event.ID, &timestampUnix,
		&event.Agent, &event.ModelVersion, &event.PromptText, &event.ResponseText,
		&event.TokensIn, &event.TokensOut, &event.LatencyMs,
		&event.RepoPath, &event.GitCommit, &event.GitBranch, &event.GitDirty, &dirtyFilesJSON,
		&event.Author, &event.IDE, &event.ActiveFile, &workspaceFilesJSON,
		&event.PromptType, &toolsInvokedJSON, &filesMentionedJSON,
		&event.BranchAtCapture, &event.PreBranchSwitch,
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
// NOTE: This function is deprecated in v2 architecture (sessions removed)
// Use GetPromptsForCommitWindow or similar query functions instead
func ListPromptEvents(db *sql.DB) ([]*PromptEvent, error) {
	query := `
		SELECT
			id, timestamp,
			agent, model_version, prompt_text, response_text,
			tokens_in, tokens_out, latency_ms,
			repo_path, git_commit, git_branch, git_dirty, dirty_files,
			author, ide, active_file, workspace_files,
			prompt_type, tools_invoked, files_mentioned,
			branch_at_capture, pre_branch_switch
		FROM prompt_events
		ORDER BY timestamp ASC
	`

	rows, err := db.Query(query)
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
			&event.ID, &timestampUnix,
			&event.Agent, &event.ModelVersion, &event.PromptText, &event.ResponseText,
			&event.TokensIn, &event.TokensOut, &event.LatencyMs,
			&event.RepoPath, &event.GitCommit, &event.GitBranch, &event.GitDirty, &dirtyFilesJSON,
			&event.Author, &event.IDE, &event.ActiveFile, &workspaceFilesJSON,
			&event.PromptType, &toolsInvokedJSON, &filesMentionedJSON,
			&event.BranchAtCapture, &event.PreBranchSwitch,
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
