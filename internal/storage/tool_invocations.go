package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// ToolInvocation represents a single tool use by an AI agent
type ToolInvocation struct {
	ID        string
	PromptID  string
	ToolName  string
	ToolArgs  string // Lightweight JSON (file paths, simple params)
	Timestamp time.Time
}

// CreateToolInvocation inserts a new tool invocation record
func CreateToolInvocation(db *sql.DB, ti *ToolInvocation) error {
	query := `
		INSERT INTO tool_invocations (id, prompt_id, tool_name, tool_args, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query, ti.ID, ti.PromptID, ti.ToolName, ti.ToolArgs, ti.Timestamp.Unix())
	if err != nil {
		return fmt.Errorf("failed to create tool invocation: %w", err)
	}

	return nil
}

// GetToolInvocation retrieves a tool invocation by ID
func GetToolInvocation(db *sql.DB, id string) (*ToolInvocation, error) {
	query := `
		SELECT id, prompt_id, tool_name, tool_args, timestamp
		FROM tool_invocations
		WHERE id = ?
	`

	var ti ToolInvocation
	var timestamp int64

	err := db.QueryRow(query, id).Scan(
		&ti.ID,
		&ti.PromptID,
		&ti.ToolName,
		&ti.ToolArgs,
		&timestamp,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tool invocation: %w", err)
	}

	ti.Timestamp = time.Unix(timestamp, 0)
	return &ti, nil
}

// GetToolInvocationsForPrompt retrieves all tool invocations for a prompt
func GetToolInvocationsForPrompt(db *sql.DB, promptID string) ([]*ToolInvocation, error) {
	query := `
		SELECT id, prompt_id, tool_name, tool_args, timestamp
		FROM tool_invocations
		WHERE prompt_id = ?
		ORDER BY timestamp ASC
	`

	rows, err := db.Query(query, promptID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tool invocations: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	invocations := []*ToolInvocation{}
	for rows.Next() {
		var ti ToolInvocation
		var timestamp int64

		err := rows.Scan(
			&ti.ID,
			&ti.PromptID,
			&ti.ToolName,
			&ti.ToolArgs,
			&timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool invocation: %w", err)
		}

		ti.Timestamp = time.Unix(timestamp, 0)
		invocations = append(invocations, &ti)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tool invocations: %w", err)
	}

	return invocations, nil
}

// DeleteToolInvocation deletes a tool invocation by ID
func DeleteToolInvocation(db *sql.DB, id string) error {
	query := `DELETE FROM tool_invocations WHERE id = ?`

	result, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tool invocation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteToolInvocationsForPrompt deletes all tool invocations for a prompt
// This is typically called when a prompt is deleted (cascade delete)
func DeleteToolInvocationsForPrompt(db *sql.DB, promptID string) error {
	query := `DELETE FROM tool_invocations WHERE prompt_id = ?`

	_, err := db.Exec(query, promptID)
	if err != nil {
		return fmt.Errorf("failed to delete tool invocations for prompt: %w", err)
	}

	return nil
}
