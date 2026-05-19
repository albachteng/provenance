package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/albachteng/provenance/internal/git"
)

// MigrateToV2 migrates the database from v1 (session-based) to v2 (commit windows)
// This function should be called after running the schema migration
func MigrateToV2(db *sql.DB, repoPath string) error {
	fmt.Println("Starting migration to v2 (commit windows architecture)...")

	// Step 1: Populate branch_at_capture from git_branch
	fmt.Println("Step 1/4: Populating branch_at_capture from git_branch...")
	if err := populateBranchAtCapture(db); err != nil {
		return fmt.Errorf("failed to populate branch_at_capture: %w", err)
	}

	// Step 2: Extract tool invocations from tools_invoked JSON
	fmt.Println("Step 2/4: Extracting tool invocations from tools_invoked JSON...")
	if err := extractToolInvocations(db); err != nil {
		return fmt.Errorf("failed to extract tool invocations: %w", err)
	}

	// Step 3: Mark historical branch switches (optional, best effort)
	fmt.Println("Step 3/4: Marking historical branch switches...")
	if repoPath != "" {
		if err := markHistoricalBranchSwitches(db, repoPath); err != nil {
			fmt.Printf("Warning: Failed to mark historical branch switches: %v\n", err)
			// Non-fatal - continue migration
		}
	} else {
		fmt.Println("Warning: No repo path provided, skipping branch switch detection")
	}

	// Step 4: Build commit windows cache (optional)
	fmt.Println("Step 4/4: Building commit windows cache...")
	if repoPath != "" {
		if err := buildCommitWindowsCache(db, repoPath); err != nil {
			fmt.Printf("Warning: Failed to build commit windows cache: %v\n", err)
			// Non-fatal - cache can be built on-demand
		}
	}

	fmt.Println("Migration to v2 complete!")
	return nil
}

// populateBranchAtCapture copies git_branch to branch_at_capture for all records
// This was already done in the SQL migration, but we do it here for safety
func populateBranchAtCapture(db *sql.DB) error {
	query := `
		UPDATE prompt_events
		SET branch_at_capture = git_branch
		WHERE branch_at_capture IS NULL
	`

	result, err := db.Exec(query)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	fmt.Printf("  Updated %d prompt events with branch_at_capture\n", rows)

	return nil
}

// extractToolInvocations extracts tool invocations from tools_invoked JSON field
func extractToolInvocations(db *sql.DB) error {
	// Query all prompts that have tools_invoked data
	query := `
		SELECT id, tools_invoked, timestamp
		FROM prompt_events
		WHERE tools_invoked IS NOT NULL AND tools_invoked != '[]' AND tools_invoked != ''
	`

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close() //nolint:errcheck

	totalInvocations := 0

	for rows.Next() {
		var promptID string
		var toolsJSON string
		var timestamp int64

		if err := rows.Scan(&promptID, &toolsJSON, &timestamp); err != nil {
			return err
		}

		// Parse tools_invoked JSON array
		var tools []string
		if err := json.Unmarshal([]byte(toolsJSON), &tools); err != nil {
			// Skip invalid JSON
			continue
		}

		// Create tool_invocations records
		for i, toolName := range tools {
			invocation := &ToolInvocation{
				ID:        fmt.Sprintf("ti-%s-%d", promptID, i),
				PromptID:  promptID,
				ToolName:  toolName,
				ToolArgs:  "", // No detailed args available from v1
				Timestamp: time.Unix(timestamp, 0),
			}

			if err := CreateToolInvocation(db, invocation); err != nil {
				// Skip duplicates, continue
				continue
			}

			totalInvocations++
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	fmt.Printf("  Created %d tool invocation records\n", totalInvocations)
	return nil
}

// markHistoricalBranchSwitches attempts to mark prompts that occurred before branch switches
// This is a best-effort heuristic based on git history
func markHistoricalBranchSwitches(db *sql.DB, repoPath string) error {
	// Get all unique branches from prompt history
	branchQuery := `
		SELECT DISTINCT git_branch
		FROM prompt_events
		WHERE git_branch IS NOT NULL AND git_branch != ''
		ORDER BY git_branch
	`

	rows, err := db.Query(branchQuery)
	if err != nil {
		return err
	}
	defer rows.Close() //nolint:errcheck

	branches := []string{}
	for rows.Next() {
		var branch string
		if err := rows.Scan(&branch); err != nil {
			continue
		}
		branches = append(branches, branch)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	// For each branch, check if we can detect any switches
	// This is heuristic: if there are prompts on branch A followed by prompts on branch B
	// and no commit on branch A between them, mark the A prompts as pre_branch_switch

	marked := 0
	for _, branch := range branches {
		count, err := markSwitchesForBranch(db, repoPath, branch)
		if err != nil {
			// Log but continue
			fmt.Printf("  Warning: Failed to mark switches for branch %s: %v\n", branch, err)
			continue
		}
		marked += count
	}

	fmt.Printf("  Marked %d prompts as pre_branch_switch\n", marked)
	return nil
}

// markSwitchesForBranch marks prompts on a specific branch that may have occurred before a switch
func markSwitchesForBranch(db *sql.DB, repoPath, branch string) (int, error) {
	// Get last commit on this branch
	lastCommit, err := git.GetLastCommitOnBranch(repoPath, branch)
	if err != nil {
		// Branch may not exist anymore
		return 0, nil
	}

	lastCommitTime, err := git.GetCommitTime(repoPath, lastCommit)
	if err != nil {
		return 0, nil
	}

	// Mark prompts on this branch that occurred after the last commit
	// (they were likely abandoned when switching branches)
	query := `
		UPDATE prompt_events
		SET pre_branch_switch = TRUE
		WHERE git_branch = ?
		  AND timestamp > ?
		  AND pre_branch_switch = FALSE
	`

	result, err := db.Exec(query, branch, lastCommitTime.Unix())
	if err != nil {
		return 0, err
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// buildCommitWindowsCache pre-computes commit windows for all branches in the repository
func buildCommitWindowsCache(db *sql.DB, repoPath string) error {
	// Get all unique branches that have prompts
	branchQuery := `
		SELECT DISTINCT git_branch
		FROM prompt_events
		WHERE repo_path = ?
		  AND git_branch IS NOT NULL
		  AND git_branch != ''
	`

	rows, err := db.Query(branchQuery, repoPath)
	if err != nil {
		return err
	}
	defer rows.Close() //nolint:errcheck

	branches := []string{}
	for rows.Next() {
		var branch string
		if err := rows.Scan(&branch); err != nil {
			continue
		}
		branches = append(branches, branch)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	totalWindows := 0

	for _, branch := range branches {
		count, err := buildWindowsForBranch(db, repoPath, branch)
		if err != nil {
			fmt.Printf("  Warning: Failed to build windows for branch %s: %v\n", branch, err)
			continue
		}
		totalWindows += count
	}

	fmt.Printf("  Created %d commit window cache entries\n", totalWindows)
	return nil
}

// buildWindowsForBranch creates commit window cache entries for a specific branch
func buildWindowsForBranch(db *sql.DB, repoPath, branch string) (int, error) {
	// Get all commits on this branch
	commits, err := git.GetCommitsInBranch(repoPath, branch)
	if err != nil {
		return 0, err
	}

	created := 0

	// Create a window for each commit
	for i, commit := range commits {
		var prevCommit string
		var prevTime time.Time

		if i < len(commits)-1 {
			// Previous commit is next in list (commits are ordered newest first)
			prevCommit = commits[i+1].SHA
			prevTime = commits[i+1].Timestamp
		}

		// Count prompts in this window
		promptCount, err := countPromptsInWindow(db, repoPath, branch, prevTime, commit.Timestamp)
		if err != nil {
			continue
		}

		// Create window entry
		window := &CommitWindow{
			ID:             fmt.Sprintf("cw-%s-%d", commit.SHA[:8], time.Now().UnixNano()),
			RepoPath:       repoPath,
			Branch:         branch,
			PrevCommit:     prevCommit,
			NextCommit:     commit.SHA,
			PrevCommitTime: prevTime,
			NextCommitTime: commit.Timestamp,
			PromptCount:    promptCount,
		}

		if err := CreateCommitWindow(db, window); err != nil {
			// Skip duplicates
			continue
		}

		created++
	}

	return created, nil
}

// countPromptsInWindow counts prompts between two timestamps on a branch
func countPromptsInWindow(db *sql.DB, repoPath, branch string, startTime, endTime time.Time) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM prompt_events
		WHERE repo_path = ?
		  AND git_branch = ?
		  AND timestamp >= ?
		  AND timestamp <= ?
		  AND pre_branch_switch = FALSE
	`

	var count int
	err := db.QueryRow(query, repoPath, branch, startTime.Unix(), endTime.Unix()).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
