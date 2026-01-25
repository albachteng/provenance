package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// CommitWindow represents a cached commit window for performance
type CommitWindow struct {
	ID             string
	RepoPath       string
	Branch         string
	PrevCommit     string // May be empty for initial commit
	NextCommit     string
	PrevCommitTime time.Time // Zero value if PrevCommit is empty
	NextCommitTime time.Time
	PromptCount    int
}

// CreateCommitWindow inserts a new commit window record
func CreateCommitWindow(db *sql.DB, cw *CommitWindow) error {
	var prevCommitTimeUnix *int64
	if !cw.PrevCommitTime.IsZero() {
		t := cw.PrevCommitTime.Unix()
		prevCommitTimeUnix = &t
	}

	var prevCommit *string
	if cw.PrevCommit != "" {
		prevCommit = &cw.PrevCommit
	}

	query := `
		INSERT INTO commit_windows (
			id, repo_path, branch, prev_commit, next_commit,
			prev_commit_time, next_commit_time, prompt_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
		cw.ID,
		cw.RepoPath,
		cw.Branch,
		prevCommit,
		cw.NextCommit,
		prevCommitTimeUnix,
		cw.NextCommitTime.Unix(),
		cw.PromptCount,
	)

	if err != nil {
		return fmt.Errorf("failed to create commit window: %w", err)
	}

	return nil
}

// GetCommitWindow retrieves a commit window by ID
func GetCommitWindow(db *sql.DB, id string) (*CommitWindow, error) {
	query := `
		SELECT id, repo_path, branch, prev_commit, next_commit,
		       prev_commit_time, next_commit_time, prompt_count
		FROM commit_windows
		WHERE id = ?
	`

	var cw CommitWindow
	var prevCommit sql.NullString
	var prevCommitTime sql.NullInt64
	var nextCommitTime int64

	err := db.QueryRow(query, id).Scan(
		&cw.ID,
		&cw.RepoPath,
		&cw.Branch,
		&prevCommit,
		&cw.NextCommit,
		&prevCommitTime,
		&nextCommitTime,
		&cw.PromptCount,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get commit window: %w", err)
	}

	if prevCommit.Valid {
		cw.PrevCommit = prevCommit.String
	}

	if prevCommitTime.Valid {
		cw.PrevCommitTime = time.Unix(prevCommitTime.Int64, 0)
	}

	cw.NextCommitTime = time.Unix(nextCommitTime, 0)

	return &cw, nil
}

// GetCommitWindowForCommit retrieves the commit window for a specific commit
func GetCommitWindowForCommit(db *sql.DB, repoPath, branch, commitSHA string) (*CommitWindow, error) {
	query := `
		SELECT id, repo_path, branch, prev_commit, next_commit,
		       prev_commit_time, next_commit_time, prompt_count
		FROM commit_windows
		WHERE repo_path = ?
		  AND branch = ?
		  AND next_commit = ?
	`

	var cw CommitWindow
	var prevCommit sql.NullString
	var prevCommitTime sql.NullInt64
	var nextCommitTime int64

	err := db.QueryRow(query, repoPath, branch, commitSHA).Scan(
		&cw.ID,
		&cw.RepoPath,
		&cw.Branch,
		&prevCommit,
		&cw.NextCommit,
		&prevCommitTime,
		&nextCommitTime,
		&cw.PromptCount,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get commit window for commit: %w", err)
	}

	if prevCommit.Valid {
		cw.PrevCommit = prevCommit.String
	}

	if prevCommitTime.Valid {
		cw.PrevCommitTime = time.Unix(prevCommitTime.Int64, 0)
	}

	cw.NextCommitTime = time.Unix(nextCommitTime, 0)

	return &cw, nil
}

// ListCommitWindows retrieves all commit windows for a repository/branch
func ListCommitWindows(db *sql.DB, repoPath, branch string) ([]*CommitWindow, error) {
	query := `
		SELECT id, repo_path, branch, prev_commit, next_commit,
		       prev_commit_time, next_commit_time, prompt_count
		FROM commit_windows
		WHERE repo_path = ?
		  AND branch = ?
		ORDER BY next_commit_time DESC
	`

	rows, err := db.Query(query, repoPath, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to query commit windows: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	windows := []*CommitWindow{}

	for rows.Next() {
		var cw CommitWindow
		var prevCommit sql.NullString
		var prevCommitTime sql.NullInt64
		var nextCommitTime int64

		err := rows.Scan(
			&cw.ID,
			&cw.RepoPath,
			&cw.Branch,
			&prevCommit,
			&cw.NextCommit,
			&prevCommitTime,
			&nextCommitTime,
			&cw.PromptCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan commit window: %w", err)
		}

		if prevCommit.Valid {
			cw.PrevCommit = prevCommit.String
		}

		if prevCommitTime.Valid {
			cw.PrevCommitTime = time.Unix(prevCommitTime.Int64, 0)
		}

		cw.NextCommitTime = time.Unix(nextCommitTime, 0)
		windows = append(windows, &cw)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating commit windows: %w", err)
	}

	return windows, nil
}

// UpdateCommitWindowPromptCount updates the prompt count for a commit window
func UpdateCommitWindowPromptCount(db *sql.DB, id string, promptCount int) error {
	query := `
		UPDATE commit_windows
		SET prompt_count = ?
		WHERE id = ?
	`

	result, err := db.Exec(query, promptCount, id)
	if err != nil {
		return fmt.Errorf("failed to update commit window: %w", err)
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

// DeleteCommitWindow deletes a commit window by ID
func DeleteCommitWindow(db *sql.DB, id string) error {
	query := `DELETE FROM commit_windows WHERE id = ?`

	result, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete commit window: %w", err)
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

// DeleteCommitWindowsForBranch deletes all commit windows for a branch
func DeleteCommitWindowsForBranch(db *sql.DB, repoPath, branch string) error {
	query := `
		DELETE FROM commit_windows
		WHERE repo_path = ? AND branch = ?
	`

	_, err := db.Exec(query, repoPath, branch)
	if err != nil {
		return fmt.Errorf("failed to delete commit windows for branch: %w", err)
	}

	return nil
}
