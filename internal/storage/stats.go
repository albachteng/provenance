package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// RepoStats holds aggregate statistics for a repository
type RepoStats struct {
	TotalPrompts   int
	TotalTokensIn  int
	TotalTokensOut int
	FilesMentioned map[string]int
	ToolsInvoked   map[string]int
}

// GetRepoStats returns aggregate statistics for all prompts in a repository
func GetRepoStats(db *sql.DB, repoPath string) (*RepoStats, error) {
	return queryStats(db, repoPath, time.Time{})
}

// GetTimeframeStats returns aggregate statistics for prompts in a repository since a given time
func GetTimeframeStats(db *sql.DB, repoPath string, since time.Time) (*RepoStats, error) {
	return queryStats(db, repoPath, since)
}

func queryStats(db *sql.DB, repoPath string, since time.Time) (*RepoStats, error) {
	stats := &RepoStats{
		FilesMentioned: make(map[string]int),
		ToolsInvoked:   make(map[string]int),
	}

	query := `
		SELECT COUNT(*), COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0)
		FROM prompt_events
		WHERE repo_path = ?
	`
	args := []any{repoPath}

	if !since.IsZero() {
		query += " AND timestamp > ?"
		args = append(args, since.Unix())
	}

	if err := db.QueryRow(query, args...).Scan(
		&stats.TotalPrompts, &stats.TotalTokensIn, &stats.TotalTokensOut,
	); err != nil {
		return nil, fmt.Errorf("failed to query stats: %w", err)
	}

	// Aggregate files and tools from JSON columns
	detailQuery := `
		SELECT files_mentioned, tools_invoked
		FROM prompt_events
		WHERE repo_path = ?
	`
	detailArgs := []any{repoPath}
	if !since.IsZero() {
		detailQuery += " AND timestamp > ?"
		detailArgs = append(detailArgs, since.Unix())
	}

	rows, err := db.Query(detailQuery, detailArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query detail stats: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var filesJSON, toolsJSON string
		if err := rows.Scan(&filesJSON, &toolsJSON); err != nil {
			return nil, fmt.Errorf("failed to scan stats row: %w", err)
		}

		var files, tools []string
		if err := json.Unmarshal([]byte(filesJSON), &files); err == nil {
			for _, f := range files {
				stats.FilesMentioned[f]++
			}
		}
		if err := json.Unmarshal([]byte(toolsJSON), &tools); err == nil {
			for _, tool := range tools {
				stats.ToolsInvoked[tool]++
			}
		}
	}

	return stats, rows.Err()
}
