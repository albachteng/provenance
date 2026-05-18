package git

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DetectBranchSwitch checks if current branch differs from last prompt's branch
// If switched, marks prompts since last commit as pre_branch_switch
func DetectBranchSwitch(db *sql.DB, repoPath, currentBranch string) (bool, error) {
	// Get last prompt in this repo
	lastPrompt, err := getLastPromptForRepo(db, repoPath)
	if err != nil {
		if err == sql.ErrNoRows {
			// No previous prompts, no switch possible
			return false, nil
		}
		return false, fmt.Errorf("failed to get last prompt: %w", err)
	}

	// Check if branch switched
	if lastPrompt.Branch != currentBranch && lastPrompt.Branch != "" {
		// Mark prompts since last commit as pre_branch_switch
		err := markPromptsPreSwitch(db, repoPath, lastPrompt.Branch, time.Now())
		if err != nil {
			return true, fmt.Errorf("failed to mark prompts: %w", err)
		}
		return true, nil
	}

	return false, nil
}

// LastPromptInfo contains minimal info about the last prompt
type LastPromptInfo struct {
	Branch    string
	Timestamp time.Time
}

// getLastPromptForRepo retrieves the most recent prompt for a repository
func getLastPromptForRepo(db *sql.DB, repoPath string) (*LastPromptInfo, error) {
	query := `
		SELECT git_branch, timestamp
		FROM prompt_events
		WHERE repo_path = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`

	var branch string
	var timestamp int64

	err := db.QueryRow(query, repoPath).Scan(&branch, &timestamp)
	if err != nil {
		return nil, err
	}

	return &LastPromptInfo{
		Branch:    branch,
		Timestamp: time.Unix(timestamp, 0),
	}, nil
}

// markPromptsPreSwitch flags prompts between last commit and switch time
func markPromptsPreSwitch(db *sql.DB, repoPath, oldBranch string, switchTime time.Time) error {
	// Get last commit time on old branch
	lastCommit, err := GetLastCommitOnBranch(repoPath, oldBranch)
	if err != nil {
		// If we can't get the last commit, mark all prompts on old branch
		// since the epoch as potentially pre-switch (conservative approach)
		query := `
			UPDATE prompt_events
			SET pre_branch_switch = TRUE
			WHERE repo_path = ?
			  AND git_branch = ?
			  AND timestamp <= ?
			  AND pre_branch_switch = FALSE
		`
		_, err = db.Exec(query, repoPath, oldBranch, switchTime.Unix())
		return err
	}

	lastCommitTime, err := GetCommitTime(repoPath, lastCommit)
	if err != nil {
		return fmt.Errorf("failed to get commit time: %w", err)
	}

	// Mark prompts in gap between last commit and switch
	query := `
		UPDATE prompt_events
		SET pre_branch_switch = TRUE
		WHERE repo_path = ?
		  AND git_branch = ?
		  AND timestamp > ?
		  AND timestamp <= ?
		  AND pre_branch_switch = FALSE
	`

	_, err = db.Exec(query, repoPath, oldBranch, lastCommitTime.Unix(), switchTime.Unix())
	return err
}

// GetLastCommitOnBranch returns the most recent commit SHA on a branch
func GetLastCommitOnBranch(repoPath, branch string) (string, error) {
	if !isGitRepo(repoPath) {
		return "", fmt.Errorf("not a git repository: %s", repoPath)
	}

	output, err := runGitCommand(repoPath, "rev-parse", branch)
	if err != nil {
		return "", fmt.Errorf("failed to get last commit on branch: %w", err)
	}

	return trimSpace(output), nil
}

// trimSpace is a helper to handle strings consistently
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
