package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/albachteng/provenance/internal/git"
	"github.com/albachteng/provenance/internal/storage"
)

// GetPromptsForCommit returns all prompts in the commit window for a given commit
// A commit window is defined as: (previous_commit_time, current_commit_time]
// Only prompts on the same branch that are not flagged as pre_branch_switch are returned
func GetPromptsForCommit(db *sql.DB, repoPath, commitSHA, branch string) ([]*storage.PromptEvent, error) {
	// Get the commit time for the current commit
	commitTime, err := git.GetCommitTime(repoPath, commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit time: %w", err)
	}

	// Get the previous commit on the same branch
	prevCommitSHA, err := git.GetPreviousCommit(repoPath, commitSHA, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to get previous commit: %w", err)
	}

	// Determine the start of the time window
	var prevCommitTime time.Time
	if prevCommitSHA == "" {
		// This is the initial commit - use epoch 0 as the start time
		// This means no prompts will be included (correct behavior)
		prevCommitTime = time.Unix(0, 0)
	} else {
		prevCommitTime, err = git.GetCommitTime(repoPath, prevCommitSHA)
		if err != nil {
			return nil, fmt.Errorf("failed to get previous commit time: %w", err)
		}
	}

	// Query prompts in the commit window
	// Window: (prevCommitTime, commitTime]
	// Filters: same branch, not pre_branch_switch, same repo
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
		WHERE repo_path = ?
		  AND branch_at_capture = ?
		  AND timestamp > ?
		  AND timestamp <= ?
		  AND pre_branch_switch = 0
		ORDER BY timestamp ASC
	`

	rows, err := db.Query(
		query,
		repoPath,
		branch,
		prevCommitTime.Unix(),
		commitTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query prompts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var prompts []*storage.PromptEvent
	for rows.Next() {
		var event storage.PromptEvent
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

		// Unmarshal JSON fields
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

		prompts = append(prompts, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating prompts: %w", err)
	}

	return prompts, nil
}
